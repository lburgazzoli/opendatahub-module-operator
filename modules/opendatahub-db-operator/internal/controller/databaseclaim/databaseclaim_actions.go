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

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	infraApi "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/api/infrastructure/v1alpha1"
	dbcontroller "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/pkg/controller"
	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/pkg/postgres"
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
	provider, err := dbcontroller.ResolveForCurrent(ctx, rr.Client, obj.Spec.Provider, obj.Status.Provider)
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
	cfg, pool, err := openPool(ctx, rr.Client, provider, m.cfg)
	if err != nil {
		rr.Conditions.Mark(ConditionProvisioned, metav1.ConditionFalse,
			conditions.WithError(err))
		if retryErr := dbcontroller.StopWithQuickRetryIfConnectionRefused(err); retryErr != nil {
			return retryErr
		}
		return odherrors.NewStopErrorW(err)
	}
	defer pool.Close()

	// 3. Ensure claim credentials and connection details.
	provisioner := DatabaseProvisioner{
		Client: rr.Client,
		Claim:  obj,
		Pool:   pool,
		Config: cfg,
	}
	secret, err := provisioner.Ensure(ctx)
	if notFound, ok := errors.AsType[ErrDatabaseNotFound](err); ok {
		rr.Conditions.Mark(ConditionProvisioned, metav1.ConditionFalse,
			conditions.WithReason("DatabaseNotFound"),
			conditions.WithMessage("database %q does not exist on provider %q", notFound.Database, provider.Name))
		return odherrors.NewStopError("%s", notFound.Error())
	}
	if err != nil {
		return err
	}

	// 4. Add credentials Secret to rr.Resources so deploy.NewAction amends it.
	if err := rr.AddResources(secret); err != nil {
		return fmt.Errorf("adding credentials Secret to resources: %w", err)
	}

	// 5. Update status -- status.database echoes spec.database exactly, no defaulting.
	connection := provisioner.ConnectionStatus()
	obj.Status.Database = connection.Database
	obj.Status.Connection = connection
	rr.Conditions.Mark(ConditionProvisioned, metav1.ConditionTrue,
		conditions.WithReason("UserProvisioned"),
		conditions.WithMessage("User provisioned on database %q (provider %q)", connection.Database, provider.Name))

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
			provider, err := dbcontroller.ResolveForCurrent(ctx, rr.Client, obj.Spec.Provider, obj.Status.Provider)
			if err != nil {
				return nil // provider permanently gone -- allow removal
			}

			_, pool, err := openPool(ctx, rr.Client, provider, m.cfg)
			if err != nil {
				return fmt.Errorf("opening pool for provider %q: %w", provider.Name, err)
			}
			defer pool.Close()

			if err := postgres.RevokeDatabasePrivileges(ctx, pool, obj.Spec.Database, role); err != nil {
				return fmt.Errorf("revoking privileges from role %q on database %q: %w", role, obj.Spec.Database, err)
			}

			// Drop only the role. NEVER issue DROP DATABASE.
			if err := postgres.DropRole(ctx, pool, role); err != nil {
				return fmt.Errorf("dropping role %q: %w", role, err)
			}
			return nil
		})
}
