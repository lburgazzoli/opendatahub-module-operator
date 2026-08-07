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
	api "github.com/opendatahub-io/odh-platform-utilities/framework/api"
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
			rr.Conditions.Mark(ConditionTLSConfiguration, metav1.ConditionUnknown,
				conditions.WithReason("ProviderNotFound"),
				conditions.WithMessage("%s", notFound.Error()))
			return odherrors.NewStopErrorW(err).WithRequeueAfter(m.cfg.DatabaseClaim.RetryInterval)
		}
		return fmt.Errorf("resolving provider: %w", err)
	}
	if obj.Spec.Provider.Selector != nil {
		obj.Status.Provider = provider.Name
	}

	// 2. Open connection to provider.
	pgClient, resolvedCfg, err := dbcontroller.NewClient(
		ctx,
		rr.Client,
		provider,
		m.cfg,
		m.PostgresClientFactory,
	)
	if err != nil {
		rr.Conditions.Mark(ConditionProvisioned, metav1.ConditionFalse,
			conditions.WithError(err))
		rr.Conditions.Mark(ConditionTLSConfiguration, metav1.ConditionUnknown,
			conditions.WithReason(dbcontroller.ReasonTLSProvisioning),
			conditions.WithMessage("%s", err.Error()))
		if retryErr := dbcontroller.StopWithQuickRetryIfConnectionRefused(err); retryErr != nil {
			return retryErr
		}
		return odherrors.NewStopErrorW(err)
	}
	defer pgClient.Close()

	cfg := pgClient.Config()
	switch {
	case !cfg.TLSEnabled():
		rr.Conditions.Mark(ConditionTLSConfiguration, metav1.ConditionFalse,
			conditions.WithSeverity(api.ConditionSeverityInfo),
			conditions.WithReason(dbcontroller.ReasonTLSNotEnabled),
			conditions.WithMessage("TLS is not enabled"))
	case cfg.TLSReady():
		rr.Conditions.Mark(ConditionTLSConfiguration, metav1.ConditionTrue,
			conditions.WithReason(dbcontroller.ReasonTLSConfigured),
			conditions.WithMessage("TLS configuration is ready"))
	default:
		rr.Conditions.Mark(ConditionTLSConfiguration, metav1.ConditionFalse,
			conditions.WithReason(dbcontroller.ReasonTLSProvisioning),
			conditions.WithMessage("TLS configuration is pending"))
	}

	// 3. Ensure claim credentials and connection details.
	provisioner := DatabaseProvisioner{
		Client:         rr.Client,
		Claim:          obj,
		Postgres:       pgClient,
		ProviderConfig: resolvedCfg,
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
	switch {
	case !cfg.TLSEnabled():
		rr.Conditions.Mark(ConditionTLSConfiguration, metav1.ConditionFalse,
			conditions.WithSeverity(api.ConditionSeverityInfo),
			conditions.WithReason(dbcontroller.ReasonTLSNotEnabled),
			conditions.WithMessage("TLS is not enabled"))
	case cfg.TLSReady():
		rr.Conditions.Mark(ConditionTLSConfiguration, metav1.ConditionTrue,
			conditions.WithReason(dbcontroller.ReasonTLSConfigured),
			conditions.WithMessage("TLS configuration is ready"))
	default:
		rr.Conditions.Mark(ConditionTLSConfiguration, metav1.ConditionFalse,
			conditions.WithReason(dbcontroller.ReasonTLSProvisioning),
			conditions.WithMessage("TLS configuration is pending"))
	}

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

	role := dbcontroller.RoleNameFor(obj.Namespace, obj.Name)

	return m.withGrace(ctx, obj,
		func(ctx context.Context) error {
			provider, err := dbcontroller.ResolveForCurrent(ctx, rr.Client, obj.Spec.Provider, obj.Status.Provider)
			if err != nil {
				var notFound dbcontroller.ErrNotFound
				if errors.As(err, &notFound) {
					return nil // provider permanently gone -- allow removal
				}
				return err
			}

			pgClient, _, err := dbcontroller.NewClient(
				ctx,
				rr.Client,
				provider,
				m.cfg,
				m.PostgresClientFactory,
			)
			if err != nil {
				return fmt.Errorf("opening postgres client for provider %q: %w", provider.Name, err)
			}
			defer pgClient.Close()

			roleExists, err := postgres.RoleExists(ctx, pgClient, role)
			if err != nil {
				return fmt.Errorf("checking role %q existence: %w", role, err)
			}
			if !roleExists {
				return nil
			}

			if err := postgres.RevokeDatabasePrivileges(ctx, pgClient, obj.Spec.Database, role); err != nil {
				return fmt.Errorf("revoking privileges from role %q on database %q: %w", role, obj.Spec.Database, err)
			}

			// Drop only the role. NEVER issue DROP DATABASE.
			if err := postgres.DropRole(ctx, pgClient, role); err != nil {
				return fmt.Errorf("dropping role %q: %w", role, err)
			}
			return nil
		})
}
