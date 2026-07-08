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

// Package schemaclaim reconciles schemaclaim objects.
// Action implementations (provisionAction, cleanupAction) live here.
// Helper functions (openPool, roleNameFor, etc.) live in schemaclaim_support.go.
package schemaclaim

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

// provisionAction is the main reconciliation action. It:
//  1. Resolves the schema name (default or explicit).
//  2. Resolves the DatabaseProvider.
//  3. Opens a connection to the provider's backend.
//  4. Idempotently creates the schema.
//  5. Idempotently provisions role + credentials:
//     - If both the role and credentials Secret already exist → no-op.
//     - Otherwise generate a new password, (re)create the role and Secret.
//  6. Writes the credentials Secret via rr.Resources → deploy.NewAction (SSA).
//  7. Updates status.
func (m *Controller) provisionAction(ctx context.Context, rr *odhtypes.ReconciliationRequest) error {
	obj, ok := rr.Instance.(*infraApi.SchemaClaim)
	if !ok {
		return fmt.Errorf("instance is not a SchemaClaim")
	}

	// 1. Resolve schema name.
	schema := resolveSchema(obj.Namespace, obj.Name, obj.Spec.Schema)

	// 2. Resolve provider.
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

	// 3. Open connection to provider.
	cfg, pool, err := openPool(ctx, rr.Client, provider)
	if err != nil {
		rr.Conditions.Mark(ConditionProvisioned, metav1.ConditionFalse,
			conditions.WithError(err))
		if retryErr := dbcontroller.StopWithQuickRetryIfConnectionRefused(err); retryErr != nil {
			return retryErr
		}
		return odherrors.NewStopErrorW(err)
	}
	defer pool.Close()

	// 4. Idempotent schema creation.
	if err := postgres.CreateSchema(ctx, pool, schema); err != nil {
		err = fmt.Errorf("creating schema: %w", err)
		if retryErr := dbcontroller.StopWithQuickRetryIfConnectionRefused(err); retryErr != nil {
			return retryErr
		}
		return err
	}

	// 5. Role + credentials:
	//    - role exists AND Secret exists → both are in sync, nothing to do.
	//    - role missing OR Secret missing → generate a new password, (re)create
	//      the role, and (re)create the Secret. This covers first-time provisioning
	//      and accidental Secret deletion (the only recovery from a missing password
	//      per docs/plan.md §2 is to re-provision with a fresh one).
	role := roleNameFor(obj)
	roleExists, err := postgres.RoleExists(ctx, pool, role)
	if err != nil {
		err = fmt.Errorf("checking role existence: %w", err)
		if retryErr := dbcontroller.StopWithQuickRetryIfConnectionRefused(err); retryErr != nil {
			return retryErr
		}
		return err
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
			err = fmt.Errorf("creating role: %w", err)
			if retryErr := dbcontroller.StopWithQuickRetryIfConnectionRefused(err); retryErr != nil {
				return retryErr
			}
			return err
		}
		readOnly := obj.Spec.Access == infraApi.AccessModeReadOnly
		if err := postgres.GrantSchemaPrivileges(ctx, pool, schema, role, readOnly); err != nil {
			err = fmt.Errorf("granting schema privileges: %w", err)
			if retryErr := dbcontroller.StopWithQuickRetryIfConnectionRefused(err); retryErr != nil {
				return retryErr
			}
			return err
		}

		// 6. Add the credentials Secret to rr.Resources so deploy.NewAction applies
		// it via SSA. opendatahub.io/managed=false prevents the gc action from treating
		// it as an orphan -- lifecycle is governed by the SchemaClaim owner reference.
		secret := buildCredentialsSecret(obj, claimConfig(cfg, role, pw, schema))
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

	// 7. Update status.
	obj.Status.Schema = schema
	obj.Status.Connection = infraApi.SchemaConnectionStatus{
		ConnectionStatus: infraApi.ConnectionStatus{
			SecretRef: corev1.LocalObjectReference{Name: obj.Name},
			Host:      cfg.Host,
			Port:      int32(cfg.Port),
		},
		Database: cfg.DBName,
		Schema:   schema,
	}
	rr.Conditions.Mark(ConditionProvisioned, metav1.ConditionTrue,
		conditions.WithReason("SchemaReady"),
		conditions.WithMessage("Schema %q provisioned on provider %q", schema, provider.Name))

	return nil
}

// cleanupAction is registered as the WithFinalizer action and runs on deletion.
// The entire body is wrapped in controller.CleanupWithGrace so that transient
// errors (provider temporarily down, DDL failure) are retried within
// cfg.GracePeriod; after the grace window a Warning event is emitted and the
// finalizer is removed without DDL cleanup.
func (m *Controller) cleanupAction(ctx context.Context, rr *odhtypes.ReconciliationRequest) error {
	obj, ok := rr.Instance.(*infraApi.SchemaClaim)
	if !ok {
		return fmt.Errorf("instance is not a SchemaClaim")
	}

	schema := resolveSchema(obj.Namespace, obj.Name, obj.Spec.Schema)
	role := roleNameFor(obj)

	return m.withGrace(ctx, obj,
		func(ctx context.Context) error {
			provider, err := dbcontroller.Resolve(ctx, rr.Client, obj.Spec.Provider)
			if err != nil {
				// Provider permanently gone -- signal CleanupWithGrace to allow removal
				// by returning nil (not an error). This is not transient.
				return nil
			}

			_, pool, err := openPool(ctx, rr.Client, provider)
			if err != nil {
				return fmt.Errorf("opening pool for provider %q: %w", provider.Name, err)
			}
			defer pool.Close()

			switch obj.Spec.DeletionPolicy {
			case infraApi.DeletionPolicyDelete:
				if err := postgres.DropSchemaCascade(ctx, pool, schema); err != nil {
					return fmt.Errorf("dropping schema %q: %w", schema, err)
				}
				if err := postgres.DropRole(ctx, pool, role); err != nil {
					return fmt.Errorf("dropping role %q: %w", role, err)
				}
			default: // Retain
				if err := postgres.DropRole(ctx, pool, role); err != nil {
					return fmt.Errorf("dropping role %q: %w", role, err)
				}
			}
			return nil
		})
}
