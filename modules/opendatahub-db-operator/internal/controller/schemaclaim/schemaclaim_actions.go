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
// Helper functions (dbcontroller.OpenPool, roleNameFor, etc.) live in schemaclaim_support.go.
package schemaclaim

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

// provisionAction is the main reconciliation action. It:
//  1. Resolves the schema name (default or explicit).
//  2. Resolves the DatabaseProvider.
//  3. Opens a connection to the provider's backend.
//  4. Idempotently creates the schema.
//  5. Idempotently provisions role + credentials:
//     - If schema, role, and credentials Secret already exist → no-op.
//     - Otherwise generate a new password, (re)create the role and Secret.
//  6. Writes the credentials Secret via rr.Resources → deploy.NewAction (SSA).
//  7. Updates status.
func (m *Controller) provisionAction(ctx context.Context, rr *odhtypes.ReconciliationRequest) error {
	obj, ok := rr.Instance.(*infraApi.SchemaClaim)
	if !ok {
		return fmt.Errorf("instance is not a SchemaClaim")
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
			return odherrors.NewStopErrorWithRequeueAfterW(m.cfg.SchemaClaim.RetryInterval, err)
		}
		return fmt.Errorf("resolving provider: %w", err)
	}
	if obj.Spec.Provider.Selector != nil {
		obj.Status.Provider = provider.Name
	}

	// 2. Open connection to provider.
	cfg, pool, err := dbcontroller.OpenPool(ctx, rr.Client, provider, m.cfg)
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
	defer pool.Close()

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
	provisioner := SchemaProvisioner{
		Client: rr.Client,
		Claim:  obj,
		Pool:   pool,
		Config: cfg,
	}

	secret, err := provisioner.Ensure(ctx)
	if err != nil {
		return err
	}

	// 4. Add the credentials Secret to rr.Resources so deploy.NewAction amends it.
	if err := rr.AddResources(secret); err != nil {
		return fmt.Errorf("adding credentials Secret to resources: %w", err)
	}

	// 5. Update status.
	obj.Status.Schema = provisioner.Schema()
	obj.Status.Connection = provisioner.ConnectionStatus(obj.Status.Schema)

	rr.Conditions.Mark(ConditionProvisioned, metav1.ConditionTrue,
		conditions.WithReason("SchemaReady"),
		conditions.WithMessage("Schema %q provisioned on provider %q", obj.Status.Schema, provider.Name))

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
	role := dbcontroller.RoleNameFor(obj.Namespace, obj.Name)

	return m.withGrace(ctx, obj,
		func(ctx context.Context) error {
			provider, err := dbcontroller.ResolveForCurrent(ctx, rr.Client, obj.Spec.Provider, obj.Status.Provider)
			if err != nil {
				var notFound dbcontroller.ErrNotFound
				if errors.As(err, &notFound) {
					// Provider permanently gone -- signal CleanupWithGrace to allow
					// removal by returning nil (not an error). This is not transient.
					return nil
				}
				return err
			}

			_, pool, err := dbcontroller.OpenPool(ctx, rr.Client, provider, m.cfg)
			if err != nil {
				return fmt.Errorf("opening pool for provider %q: %w", provider.Name, err)
			}
			defer pool.Close()

			roleExists, err := postgres.RoleExists(ctx, pool, role)
			if err != nil {
				return fmt.Errorf("checking role %q existence: %w", role, err)
			}
			if !roleExists {
				return nil
			}

			if err := postgres.RevokeSchemaPrivileges(ctx, pool, schema, role); err != nil {
				return fmt.Errorf("revoking schema privileges for role %q: %w", role, err)
			}

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
