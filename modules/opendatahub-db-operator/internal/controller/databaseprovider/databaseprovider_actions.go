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

// Package databaseprovider reconciles databaseprovider objects.
// Task-specific action implementations live here.
package databaseprovider

import (
	"context"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	infraApi "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/api/infrastructure/v1alpha1"
	dbcontroller "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/pkg/controller"
	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/pkg/postgres"
	pginstance "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/pkg/postgres/instance"
	api "github.com/opendatahub-io/odh-platform-utilities/framework/api"
	"github.com/opendatahub-io/odh-platform-utilities/framework/controller/conditions"
	odhtypes "github.com/opendatahub-io/odh-platform-utilities/framework/controller/types"
)

func (m *Controller) reconcileExternalAction(
	ctx context.Context,
	rr *odhtypes.ReconciliationRequest,
) error {
	obj, ok := rr.Instance.(*infraApi.DatabaseProvider)
	if !ok {
		return fmt.Errorf("instance is not a DatabaseProvider")
	}
	if obj.Spec.Type != infraApi.ProviderTypeExternal {
		return nil
	}

	cfg, err := loadExternalConfig(ctx, rr.Client, obj)
	if err != nil {
		obj.Status.Connection = infraApi.ProviderConnectionStatus{}
		obj.Status.TLS = nil
		rr.Conditions.Mark(ConditionReachable, metav1.ConditionFalse,
			conditions.WithReason(externalFailureReason(err)),
			conditions.WithMessage("%s", err.Error()))
		rr.Conditions.Mark(ConditionTLSConfiguration, metav1.ConditionUnknown,
			conditions.WithReason(externalFailureReason(err)),
			conditions.WithMessage("%s", err.Error()))
		return nil
	}

	obj.Status.Connection = providerConnectionStatus(cfg)
	obj.Status.TLS = &infraApi.ProviderTLSStatus{
		Enabled: cfg.TLSEnabled(),
		Ready:   cfg.TLSReady(),
	}
	switch {
	case !cfg.TLSEnabled():
		rr.Conditions.Mark(ConditionTLSConfiguration, metav1.ConditionFalse,
			conditions.WithSeverity(api.ConditionSeverityInfo),
			conditions.WithReason(reasonTLSNotEnabled),
			conditions.WithMessage("TLS is not enabled for this external provider"))
	case cfg.TLSReady():
		rr.Conditions.Mark(ConditionTLSConfiguration, metav1.ConditionTrue,
			conditions.WithReason(reasonTLSConfigured),
			conditions.WithMessage("External provider TLS configuration resolved"))
	default:
		rr.Conditions.Mark(ConditionTLSConfiguration, metav1.ConditionFalse,
			conditions.WithReason(reasonTLSProvisioning),
			conditions.WithMessage("External provider TLS configuration is pending"))
	}

	factory := m.PostgresClientFactory
	if factory == nil {
		factory = postgres.DefaultClientFactory
	}

	pgClient, err := factory(ctx, cfg)
	if err != nil {
		rr.Conditions.Mark(ConditionReachable, metav1.ConditionFalse,
			conditions.WithReason(externalFailureReason(err)),
			conditions.WithMessage("%s", err.Error()))
		if retryErr := dbcontroller.StopWithQuickRetryIfConnectionRefused(err); retryErr != nil {
			return retryErr
		}
		return nil
	}
	defer pgClient.Close()

	if err := pgClient.Ping(ctx); err != nil {
		rr.Conditions.Mark(ConditionReachable, metav1.ConditionFalse,
			conditions.WithReason(externalFailureReason(err)),
			conditions.WithMessage("%s", err.Error()))
		if retryErr := dbcontroller.StopWithQuickRetryIfConnectionRefused(err); retryErr != nil {
			return retryErr
		}
		return nil
	}

	rr.Conditions.Mark(ConditionReachable, metav1.ConditionTrue,
		conditions.WithReason("ConnectionVerified"),
		conditions.WithMessage("Connection verified"))

	return nil
}

func (m *Controller) reconcileInternalAction(
	ctx context.Context,
	rr *odhtypes.ReconciliationRequest,
) error {
	obj, ok := rr.Instance.(*infraApi.DatabaseProvider)
	if !ok {
		return fmt.Errorf("instance is not a DatabaseProvider")
	}
	if obj.Spec.Type != infraApi.ProviderTypeInternal {
		return nil
	}

	tlsState, err := computeInternalAdminSecret(ctx, rr.Client, obj, m.cfg)
	obj.Status.TLS = internalTLSStatus(obj, m.cfg, tlsState.Ready)
	if err != nil {
		obj.Status.Connection = infraApi.ProviderConnectionStatus{}

		rr.Conditions.Mark(ConditionReachable, metav1.ConditionFalse,
			conditions.WithReason(reasonAdminSecretUnavailable),
			conditions.WithMessage("%s", err.Error()))

		if tlsState.Enabled {
			rr.Conditions.Mark(ConditionTLSConfiguration, metav1.ConditionFalse,
				conditions.WithReason(reasonTLSProvisioning),
				conditions.WithMessage("%s", err.Error()))
		} else {
			rr.Conditions.Mark(ConditionTLSConfiguration, metav1.ConditionFalse,
				conditions.WithSeverity(api.ConditionSeverityInfo),
				conditions.WithReason(reasonTLSNotEnabled),
				conditions.WithMessage("TLS is not enabled for this internal provider"))
		}

		return fmt.Errorf("ensuring internal admin Secret: %w", err)
	}

	if err := rr.AddResources(tlsState.AdminSecret); err != nil {
		return fmt.Errorf("adding internal admin Secret to resources: %w", err)
	}

	// Build connection status from the in-memory secret so it is available on
	// first reconcile before the deploy action has persisted the Secret.
	obj.Status.Connection = infraApi.ProviderConnectionStatus{
		Host:     string(tlsState.AdminSecret.Data[postgres.SecretKeyHost]),
		Port:     postgres.DefaultPort,
		Database: string(tlsState.AdminSecret.Data[postgres.SecretKeyDatabase]),
	}

	switch {
	case !tlsState.Enabled:
		rr.Conditions.Mark(ConditionTLSConfiguration, metav1.ConditionFalse,
			conditions.WithSeverity(api.ConditionSeverityInfo),
			conditions.WithReason(reasonTLSNotEnabled),
			conditions.WithMessage("TLS is not enabled for this internal provider"))
	case tlsState.Ready:
		rr.Conditions.Mark(ConditionTLSConfiguration, metav1.ConditionTrue,
			conditions.WithReason(reasonTLSConfigured),
			conditions.WithMessage("Internal provider TLS configuration resolved"))
	default:
		rr.Conditions.Mark(ConditionTLSConfiguration, metav1.ConditionFalse,
			conditions.WithReason(reasonTLSProvisioning),
			conditions.WithMessage("Internal provider TLS configuration is pending"))
	}

	data, err := resolveInternalData(ctx, rr.Client, obj, m.cfg, tlsState)
	if err != nil {
		return fmt.Errorf("resolving internal resource data: %w", err)
	}

	pgres, err := pginstance.Resources(ctx, data)
	if err != nil {
		return fmt.Errorf("rendering internal resources: %w", err)
	}

	for i := range pgres {
		if err := rr.AddResources(&pgres[i]); err != nil {
			return fmt.Errorf("adding internal resources: %w", err)
		}
	}

	return nil
}

func (m *Controller) internalReadinessAction(
	ctx context.Context,
	rr *odhtypes.ReconciliationRequest,
) error {
	obj, ok := rr.Instance.(*infraApi.DatabaseProvider)
	if !ok {
		return fmt.Errorf("instance is not a DatabaseProvider")
	}
	if obj.Spec.Type != infraApi.ProviderTypeInternal {
		return nil
	}

	sts := &appsv1.StatefulSet{}
	sts.Namespace = dbcontroller.InternalNamespace(obj, m.cfg.OperatorNamespace)
	sts.Name = dbcontroller.InternalServiceName(obj.Name)

	if err := rr.Client.Get(ctx, client.ObjectKeyFromObject(sts), sts); err != nil {
		if apierrors.IsNotFound(err) {
			rr.Conditions.Mark(ConditionReachable, metav1.ConditionFalse,
				conditions.WithReason(reasonProvisioning),
				conditions.WithMessage("Waiting for internal PostgreSQL resources"))
			return nil
		}
		return fmt.Errorf("reading internal StatefulSet: %w", err)
	}

	if sts.Status.ReadyReplicas != 1 {
		rr.Conditions.Mark(ConditionReachable, metav1.ConditionFalse,
			conditions.WithReason(reasonProvisioning),
			conditions.WithMessage("Waiting for internal PostgreSQL StatefulSet to become ready"))
		return nil
	}

	rr.Conditions.Mark(ConditionReachable, metav1.ConditionTrue,
		conditions.WithReason(reasonInstanceRunning),
		conditions.WithMessage("Internal PostgreSQL instance is ready"))

	return nil
}
