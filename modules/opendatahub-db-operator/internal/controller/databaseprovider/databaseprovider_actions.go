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
	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/assets"
	dbcontroller "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/pkg/controller"
	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/pkg/postgres"
	dbmaps "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/pkg/utils/maps"
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

	if err := postgres.Ping(ctx, cfg); err != nil {
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

func (m *Controller) reconcileEmbeddedAction(
	ctx context.Context,
	rr *odhtypes.ReconciliationRequest,
) error {
	obj, ok := rr.Instance.(*infraApi.DatabaseProvider)
	if !ok {
		return fmt.Errorf("instance is not a DatabaseProvider")
	}
	if obj.Spec.Type != infraApi.ProviderTypeEmbedded {
		return nil
	}

	tlsState, err := computeEmbeddedAdminSecret(ctx, rr.Client, obj, m.cfg)
	obj.Status.TLS = embeddedTLSStatus(obj, m.cfg, tlsState.Ready)
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
				conditions.WithMessage("TLS is not enabled for this embedded provider"))
		}

		return fmt.Errorf("ensuring embedded admin Secret: %w", err)
	}

	if err := rr.AddResources(tlsState.AdminSecret); err != nil {
		return fmt.Errorf("adding embedded admin Secret to resources: %w", err)
	}

	if key := tlsState.AdminSecret.Annotations[dbcontroller.EmbeddedAdminSecretKeyAnnotation]; len(key) != 0 {
		rr.Extensions = dbmaps.Set(rr.Extensions, extKeyAdminSecretKey, any(key))
	}
	if key := tlsState.TLSSecretHash; len(key) != 0 {
		rr.Extensions = dbmaps.Set(rr.Extensions, extKeyTLSSecretHash, any(key))
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
			conditions.WithMessage("TLS is not enabled for this embedded provider"))
	case tlsState.Ready:
		rr.Conditions.Mark(ConditionTLSConfiguration, metav1.ConditionTrue,
			conditions.WithReason(reasonTLSConfigured),
			conditions.WithMessage("Embedded provider TLS configuration resolved"))
	default:
		rr.Conditions.Mark(ConditionTLSConfiguration, metav1.ConditionFalse,
			conditions.WithReason(reasonTLSProvisioning),
			conditions.WithMessage("Embedded provider TLS configuration is pending"))
	}

	rr.Templates = []odhtypes.TemplateInfo{
		{FS: assets.Manifests, Path: "manifests/embedded/pvc.yaml.tmpl"},
		{FS: assets.Manifests, Path: "manifests/embedded/service.yaml.tmpl"},
		{FS: assets.Manifests, Path: "manifests/embedded/initdb-configmap.yaml.tmpl"},
		{FS: assets.Manifests, Path: "manifests/embedded/issuer.yaml.tmpl"},
		{FS: assets.Manifests, Path: "manifests/embedded/certificate.yaml.tmpl"},
		{FS: assets.Manifests, Path: "manifests/embedded/statefulset.yaml.tmpl"},
		{FS: assets.Manifests, Path: "manifests/embedded/networkpolicy.yaml.tmpl"},
	}

	return nil
}

func (m *Controller) embeddedReadinessAction(
	ctx context.Context,
	rr *odhtypes.ReconciliationRequest,
) error {
	obj, ok := rr.Instance.(*infraApi.DatabaseProvider)
	if !ok {
		return fmt.Errorf("instance is not a DatabaseProvider")
	}
	if obj.Spec.Type != infraApi.ProviderTypeEmbedded {
		return nil
	}

	sts := &appsv1.StatefulSet{}
	sts.Namespace = dbcontroller.EmbeddedNamespace(obj, m.cfg.OperatorNamespace)
	sts.Name = dbcontroller.EmbeddedServiceName(obj.Name)

	if err := rr.Client.Get(ctx, client.ObjectKeyFromObject(sts), sts); err != nil {
		if apierrors.IsNotFound(err) {
			rr.Conditions.Mark(ConditionReachable, metav1.ConditionFalse,
				conditions.WithReason(reasonProvisioning),
				conditions.WithMessage("Waiting for embedded PostgreSQL resources"))
			return nil
		}
		return fmt.Errorf("reading embedded StatefulSet: %w", err)
	}

	if sts.Status.ReadyReplicas != 1 {
		rr.Conditions.Mark(ConditionReachable, metav1.ConditionFalse,
			conditions.WithReason(reasonProvisioning),
			conditions.WithMessage("Waiting for embedded PostgreSQL StatefulSet to become ready"))
		return nil
	}

	rr.Conditions.Mark(ConditionReachable, metav1.ConditionTrue,
		conditions.WithReason(reasonInstanceRunning),
		conditions.WithMessage("Embedded PostgreSQL instance is ready"))

	return nil
}
