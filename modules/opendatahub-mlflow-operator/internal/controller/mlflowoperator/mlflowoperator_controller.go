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

package mlflowoperator

import (
	"context"

	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	extv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"

	componentApi "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-mlflow-operator/api/components/v1alpha1"
	moduleconfig "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-mlflow-operator/pkg/config"
	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-mlflow-operator/pkg/resources/gvk"
	"github.com/opendatahub-io/opendatahub-operator/v2/api/common"
	"github.com/opendatahub-io/opendatahub-operator/v2/pkg/controller/actions/deploy"
	"github.com/opendatahub-io/opendatahub-operator/v2/pkg/controller/actions/gc"
	"github.com/opendatahub-io/opendatahub-operator/v2/pkg/controller/actions/render/kustomize"
	"github.com/opendatahub-io/opendatahub-operator/v2/pkg/controller/actions/status/deployments"
	"github.com/opendatahub-io/opendatahub-operator/v2/pkg/controller/actions/status/releases"
	"github.com/opendatahub-io/opendatahub-operator/v2/pkg/controller/handlers"
	"github.com/opendatahub-io/opendatahub-operator/v2/pkg/controller/predicates"
	"github.com/opendatahub-io/opendatahub-operator/v2/pkg/controller/predicates/component"
	"github.com/opendatahub-io/opendatahub-operator/v2/pkg/controller/reconciler"
	"github.com/opendatahub-io/opendatahub-operator/v2/pkg/controller/status"
	"github.com/opendatahub-io/opendatahub-operator/v2/pkg/metadata/labels"
)

// +kubebuilder:rbac:groups=components.platform.opendatahub.io,resources=mlflowoperators,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=components.platform.opendatahub.io,resources=mlflowoperators/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=components.platform.opendatahub.io,resources=mlflowoperators/finalizers,verbs=update
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=services;serviceaccounts;configmaps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=clusterroles;clusterrolebindings;roles;rolebindings,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=console.openshift.io,resources=consolelinks,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=monitoring.coreos.com,resources=servicemonitors,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apiextensions.k8s.io,resources=customresourcedefinitions,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=httproutes,verbs=get;list;watch;create;delete;patch;update
// +kubebuilder:rbac:groups=batch,resources=jobs;cronjobs,verbs=get;list;watch;create;delete;patch;update
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;delete;patch;update
// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=namespaces,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=persistentvolumeclaims,verbs=get;list;watch;create;delete;patch;update
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch
// +kubebuilder:rbac:groups=networking.k8s.io,resources=networkpolicies,verbs=get;list;watch;create;delete;patch;update
// +kubebuilder:rbac:groups=coordination.k8s.io,resources=leases,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=authentication.k8s.io,resources=tokenreviews,verbs=create
// +kubebuilder:rbac:groups=authorization.k8s.io,resources=subjectaccessreviews,verbs=create
// +kubebuilder:rbac:groups=mlflow.opendatahub.io,resources=mlflows;mlflows/status;mlflows/finalizers,verbs=get;list;watch;create;delete;deletecollection;patch;update
// +kubebuilder:rbac:groups=mlflow.kubeflow.org,resources=mlflowconfigs;datasets;experiments;registeredmodels;gatewayendpoints;gatewaymodeldefinitions;gatewaysecrets,verbs=get;list;watch;create;delete;deletecollection;patch;update
// +kubebuilder:rbac:groups=mlflow.kubeflow.org,resources=gatewayendpoints/use;gatewaymodeldefinitions/use;gatewaysecrets/use,verbs=create
// +kubebuilder:rbac:urls=/metrics,verbs=get

func NewReconciler(
	ctx context.Context,
	mgr ctrl.Manager,
	cfg *moduleconfig.Config,
	rel common.Release,
) error {
	m, err := NewModule(cfg)
	if err != nil {
		return err
	}

	r, err := reconciler.ReconcilerFor(mgr, &componentApi.MLflowOperator{}).
		Owns(&corev1.ConfigMap{}).
		Owns(&rbacv1.ClusterRole{}).
		Owns(&rbacv1.ClusterRoleBinding{}).
		Owns(&rbacv1.Role{}).
		Owns(&rbacv1.RoleBinding{}).
		Owns(&corev1.Service{}).
		Owns(&corev1.ServiceAccount{}).
		OwnsGVK(gvk.ConsoleLink).
		Owns(&monitoringv1.ServiceMonitor{}).
		Owns(&appsv1.Deployment{}, reconciler.WithPredicates(predicates.DefaultDeploymentPredicate)).
		OwnsGVK(gvk.MLflow, reconciler.Dynamic()).
		Watches(
			&extv1.CustomResourceDefinition{},
			reconciler.WithEventHandler(
				handlers.ToNamed(componentApi.MLflowOperatorInstanceName)),
			reconciler.WithPredicates(
				component.ForLabel(labels.ODH.Component(componentName), labels.True)),
		).
		Watches(&gwapiv1.HTTPRoute{}).
		// Re-reconcile when GatewayConfig changes so mlflow-url stays current.
		WatchesGVK(
			gvk.GatewayConfig,
			reconciler.Dynamic(reconciler.CrdExists(gvk.GatewayConfig)),
			reconciler.WithEventHandler(handlers.ToNamed(componentApi.MLflowOperatorInstanceName)),
		).
		WithAction(m.initialize).
		WithAction(m.upgradeIfNeeded).
		WithAction(m.setKustomizedParams).
		WithAction(releases.NewAction()).
		WithAction(kustomize.NewAction()).
		WithAction(m.fixDeploymentNamespace).
		WithAction(deploy.NewAction(
			deploy.WithCache(),
			deploy.WithApplyOrder(),
			deploy.WithLabel(labels.ODH.Component(componentName), labels.True),
		)).
		WithAction(deployments.NewAction()).
		WithAction(m.reportStatus).
		WithAction(gc.NewAction(
			gc.InNamespace(cfg.ApplicationsNamespace),
		)).
		WithConditions(
			status.ConditionDeploymentsAvailable,
		).
		Build(ctx)

	if err != nil {
		return err
	}

	r.Release = rel

	return nil
}
