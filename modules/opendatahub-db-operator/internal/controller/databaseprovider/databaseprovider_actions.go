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
	odherrors "github.com/opendatahub-io/odh-platform-utilities/framework/controller/actions/errors"
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
		rr.Conditions.Mark(ConditionReachable, metav1.ConditionFalse,
			conditions.WithReason(externalFailureReason(err)),
			conditions.WithMessage("%s", err.Error()))
		return nil
	}

	obj.Status.Connection = providerConnectionStatus(cfg)

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

	if _, err := resolveEmbeddedImage(obj, m.cfg); err != nil {
		rr.Conditions.Mark(ConditionReachable, metav1.ConditionFalse,
			conditions.WithReason(reasonImageUnmapped),
			conditions.WithMessage("%s", err.Error()))
		return odherrors.NewStopErrorW(err)
	}

	sts := &appsv1.StatefulSet{}
	stsKey := embeddedStatefulSetKey(obj, m.cfg)
	err := rr.Client.Get(ctx, stsKey, sts)
	switch {
	case err == nil && embeddedImageChanged(sts, obj, m.cfg):
		err := fmt.Errorf("embedded extensions changed for an existing instance; recreate the provider to apply them")
		rr.Conditions.Mark(ConditionReachable, metav1.ConditionFalse,
			conditions.WithReason(reasonExtensionChangeRequiresRecreate),
			conditions.WithMessage("%s", err.Error()))
		return odherrors.NewStopErrorW(err)
	case client.IgnoreNotFound(err) != nil:
		return fmt.Errorf("reading embedded StatefulSet: %w", err)
	}

	if _, err := ensureEmbeddedAdminSecret(ctx, rr.Client, obj, m.cfg); err != nil {
		obj.Status.Connection = infraApi.ProviderConnectionStatus{}
		rr.Conditions.Mark(ConditionReachable, metav1.ConditionFalse,
			conditions.WithReason(reasonAdminSecretUnavailable),
			conditions.WithMessage("%s", err.Error()))
		return fmt.Errorf("ensuring embedded admin Secret: %w", err)
	}

	cfg, err := dbcontroller.LoadProviderConfig(ctx, rr.Client, obj, m.cfg.OperatorNamespace)
	if err != nil {
		obj.Status.Connection = infraApi.ProviderConnectionStatus{}
		return fmt.Errorf("loading embedded provider connection status: %w", err)
	}
	obj.Status.Connection = providerConnectionStatus(cfg)

	rr.Templates = []odhtypes.TemplateInfo{
		{FS: assets.Manifests, Path: "manifests/embedded/pvc.yaml.tmpl"},
		{FS: assets.Manifests, Path: "manifests/embedded/service.yaml.tmpl"},
		{FS: assets.Manifests, Path: "manifests/embedded/initdb-configmap.yaml.tmpl"},
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
	if err := rr.Client.Get(ctx, embeddedStatefulSetKey(obj, m.cfg), sts); err != nil {
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

func (m *Controller) embeddedIdleCleanupAction(
	ctx context.Context,
	rr *odhtypes.ReconciliationRequest,
) error {
	obj, ok := rr.Instance.(*infraApi.DatabaseProvider)
	if !ok {
		return fmt.Errorf("instance is not a DatabaseProvider")
	}
	if obj.Spec.Type != infraApi.ProviderTypeEmbedded || !wantsIdleDeletion(obj) {
		return nil
	}

	sts := &appsv1.StatefulSet{}
	if err := rr.Client.Get(ctx, embeddedStatefulSetKey(obj, m.cfg), sts); err != nil {
		return client.IgnoreNotFound(err)
	}

	namespaces, err := referencedClaimNamespaces(ctx, rr.Client, obj)
	if err != nil {
		return fmt.Errorf("listing referenced claim namespaces: %w", err)
	}
	if len(namespaces) != 0 {
		return nil
	}

	rr.Conditions.Mark(ConditionReachable, metav1.ConditionFalse,
		conditions.WithReason(reasonIdle),
		conditions.WithMessage("No claims currently reference this embedded provider"))
	if !shouldTearDownIdleInstance(obj, m.cfg.GracePeriod) {
		return nil
	}

	if err := deleteEmbeddedChildResources(ctx, rr.Client, obj, m.cfg); err != nil {
		return fmt.Errorf("deleting idle embedded resources: %w", err)
	}

	return nil
}
