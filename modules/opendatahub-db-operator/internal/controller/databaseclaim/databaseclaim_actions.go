/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Package databaseclaim reconciles databaseclaim objects.
// Action implementations live here; helpers live in databaseclaim_support.go.
package databaseclaim

import (
	"context"
	"errors"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"

	infraApi "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/api/infrastructure/v1alpha1"
	dbcontroller "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/pkg/controller"
	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/pkg/postgres"
	"github.com/opendatahub-io/odh-platform-utilities/framework/controller/actions/deploy"
	odherrors "github.com/opendatahub-io/odh-platform-utilities/framework/controller/actions/errors"
	"github.com/opendatahub-io/odh-platform-utilities/framework/controller/conditions"
	odhtypes "github.com/opendatahub-io/odh-platform-utilities/framework/controller/types"
)

// provisionAction reconciles the DatabaseClaim:
//  1. Resolves the DatabaseProvider.
//  2. Opens a connection to the provider's backend.
//  3. Verifies spec.database exists (never creates it).
//  4. Idempotently provisions a dedicated user with database-level privileges.
//  5. Writes the credentials Secret via rr.Resources → deploy.NewAction (SSA).
//  6. Updates status.
func (m *Controller) provisionAction(ctx context.Context, rr *odhtypes.ReconciliationRequest) error {
	obj, ok := rr.Instance.(*infraApi.DatabaseClaim)
	if !ok {
		return fmt.Errorf("instance is not a DatabaseClaim")
	}

	// 1. Resolve provider.
	provider, err := dbcontroller.Resolve(ctx, rr.Client, obj.Spec.Provider)
	if err != nil {
		if notFound, ok := errors.AsType[dbcontroller.ErrNotFound](err); ok {
			rr.Conditions.Mark(ConditionProvisioned, metav1.ConditionFalse,
				conditions.WithError(notFound),
				conditions.WithReason("ProviderNotFound"))
			return odherrors.NewStopErrorW(err)
		}
		return fmt.Errorf("resolving provider: %w", err)
	}
	if obj.Spec.Provider.Selector != nil {
		obj.Status.Provider = provider.Name
	}

	// 2. Open connection to provider.
	cfg, pool, err := openPool(ctx, rr.Client, provider)
	if err != nil {
		rr.Conditions.Mark(ConditionProvisioned, metav1.ConditionFalse,
			conditions.WithError(err))
		return odherrors.NewStopErrorW(err)
	}
	defer pool.Close()

	// 3. Verify database exists -- DatabaseClaim never creates a database.
	dbExists, err := postgres.DatabaseExists(ctx, pool, obj.Spec.Database)
	if err != nil {
		return fmt.Errorf("checking database existence: %w", err)
	}
	if !dbExists {
		rr.Conditions.Mark(ConditionProvisioned, metav1.ConditionFalse,
			conditions.WithReason("DatabaseNotFound"),
			conditions.WithMessage("database %q does not exist on provider %q", obj.Spec.Database, provider.Name))
		return odherrors.NewStopError("database %q not found", obj.Spec.Database)
	}

	// 4. Role + credentials -- same idempotency pattern as SchemaClaim:
	//    if both role and Secret exist, nothing to do.
	role := roleNameFor(obj)
	roleExists, err := postgres.RoleExists(ctx, pool, role)
	if err != nil {
		return fmt.Errorf("checking role existence: %w", err)
	}
	secretExists, err := credentialsSecretExists(ctx, rr.Client, obj)
	if err != nil {
		return fmt.Errorf("checking credentials Secret existence: %w", err)
	}

	if !roleExists || !secretExists {
		pw, err := postgres.GeneratePassword(24)
		if err != nil {
			return fmt.Errorf("generating password: %w", err)
		}
		if err := postgres.CreateRole(ctx, pool, role, pw); err != nil {
			return fmt.Errorf("creating role: %w", err)
		}
		if err := postgres.GrantDatabasePrivileges(ctx, pool, obj.Spec.Database, role); err != nil {
			return fmt.Errorf("granting database privileges: %w", err)
		}

		// 5. Add credentials Secret to rr.Resources for deploy.NewAction (SSA).
		// opendatahub.io/managed=false so gc action doesn't delete it as an orphan.
		secret := buildCredentialsSecret(obj, claimConfig(cfg, role, pw))
		if err := ctrl.SetControllerReference(obj, secret, rr.Client.Scheme()); err != nil {
			return fmt.Errorf("setting owner reference: %w", err)
		}
		if secret.Annotations == nil {
			secret.Annotations = make(map[string]string)
		}
		secret.Annotations[deploy.DefaultManagedByAnnotation] = "false"
		if err := rr.AddResources(secret); err != nil {
			return fmt.Errorf("adding credentials Secret to resources: %w", err)
		}
	}

	// 6. Update status -- status.database echoes spec.database exactly, no defaulting.
	obj.Status.Database = obj.Spec.Database
	obj.Status.Connection = infraApi.DatabaseConnectionStatus{
		SecretRef: corev1.LocalObjectReference{Name: obj.Name},
		Host:      cfg.Host,
		Port:      int32(cfg.Port),
		Database:  obj.Spec.Database,
	}
	rr.Conditions.Mark(ConditionProvisioned, metav1.ConditionTrue,
		conditions.WithReason("UserProvisioned"),
		conditions.WithMessage("User provisioned on database %q (provider %q)", obj.Spec.Database, provider.Name))

	return nil
}

// cleanupAction is registered via WithFinalizer and runs on deletion.
// DatabaseClaim always uses Retain semantics: only the provisioned user is
// dropped, never the database itself. The entire body is wrapped in
// controller.CleanupWithGrace so transient failures retry within cfg.GracePeriod.
func (m *Controller) cleanupAction(ctx context.Context, rr *odhtypes.ReconciliationRequest) error {
	obj, ok := rr.Instance.(*infraApi.DatabaseClaim)
	if !ok {
		return fmt.Errorf("instance is not a DatabaseClaim")
	}

	role := roleNameFor(obj)

	return m.withGrace(ctx, obj,
		func(ctx context.Context) error {
			provider, err := dbcontroller.Resolve(ctx, rr.Client, obj.Spec.Provider)
			if err != nil {
				return nil // provider permanently gone -- allow removal
			}

			_, pool, err := openPool(ctx, rr.Client, provider)
			if err != nil {
				return fmt.Errorf("opening pool for provider %q: %w", provider.Name, err)
			}
			defer pool.Close()

			// Drop only the role. NEVER issue DROP DATABASE.
			if err := postgres.DropRole(ctx, pool, role); err != nil {
				return fmt.Errorf("dropping role %q: %w", role, err)
			}
			return nil
		})
}
