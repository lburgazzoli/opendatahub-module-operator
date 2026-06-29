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

package sparkoperator

import (
	"context"

	promv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	admissionv1 "k8s.io/api/admissionregistration/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	extv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	ctrl "sigs.k8s.io/controller-runtime"

	componentApi "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-spark-operator/api/components/v1alpha1"
	moduleconfig "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-spark-operator/pkg/config"
	"github.com/opendatahub-io/odh-platform-utilities/framework/controller/actions/deploy"
	"github.com/opendatahub-io/odh-platform-utilities/framework/controller/actions/gc"
	"github.com/opendatahub-io/odh-platform-utilities/framework/controller/actions/render/kustomize"
	fwtemplate "github.com/opendatahub-io/odh-platform-utilities/framework/controller/actions/render/template"
	"github.com/opendatahub-io/odh-platform-utilities/framework/controller/actions/status/deployments"
	"github.com/opendatahub-io/odh-platform-utilities/framework/controller/handlers"
	"github.com/opendatahub-io/odh-platform-utilities/framework/controller/predicates"
	labelpred "github.com/opendatahub-io/odh-platform-utilities/framework/controller/predicates/label"
	"github.com/opendatahub-io/odh-platform-utilities/framework/controller/reconciler"
	mk "github.com/opendatahub-io/odh-platform-utilities/framework/render/kustomize"
)

const appLabelPrefix = "app.opendatahub.io"

// +kubebuilder:rbac:groups=components.platform.opendatahub.io,resources=sparkoperators,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=components.platform.opendatahub.io,resources=sparkoperators/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=components.platform.opendatahub.io,resources=sparkoperators/finalizers,verbs=update
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=services;serviceaccounts;configmaps,verbs=get;list;watch;create;update;patch;delete;deletecollection
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=clusterroles;clusterrolebindings;roles;rolebindings,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=admissionregistration.k8s.io,resources=mutatingwebhookconfigurations;validatingwebhookconfigurations,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=monitoring.coreos.com,resources=podmonitors,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apiextensions.k8s.io,resources=customresourcedefinitions,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=networking.k8s.io,resources=networkpolicies;ingresses,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=extensions,resources=ingresses,verbs=get;list;watch;create;delete;update
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update;patch
// +kubebuilder:rbac:groups="",resources=pods;persistentvolumeclaims,verbs=get;list;watch;create;update;patch;delete;deletecollection
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch;update
// +kubebuilder:rbac:groups=coordination.k8s.io,resources=leases,verbs=get;list;watch;create;update;patch
// +kubebuilder:rbac:groups=config.openshift.io,resources=apiservers,verbs=get;list;watch
// +kubebuilder:rbac:groups=sparkoperator.k8s.io,resources=sparkapplications;sparkapplications/status;sparkapplications/finalizers;scheduledsparkapplications;scheduledsparkapplications/status;scheduledsparkapplications/finalizers;sparkconnects;sparkconnects/status;sparkconnects/finalizers,verbs=get;list;watch;create;delete;patch;update

func NewReconciler(
	ctx context.Context,
	mgr ctrl.Manager,
	cfg *moduleconfig.Config,
) error {
	m, err := NewModule(cfg)
	if err != nil {
		return err
	}

	if err := m.Init(); err != nil {
		return err
	}

	_, err = reconciler.ReconcilerFor(mgr, &componentApi.SparkOperator{}).
		Owns(&corev1.ConfigMap{}).
		Owns(&rbacv1.RoleBinding{}).
		Owns(&rbacv1.Role{}).
		Owns(&rbacv1.ClusterRoleBinding{}).
		Owns(&rbacv1.ClusterRole{}).
		Owns(&corev1.ServiceAccount{}).
		Owns(&corev1.Service{}).
		Owns(&promv1.PodMonitor{}).
		Owns(&admissionv1.MutatingWebhookConfiguration{}).
		Owns(&admissionv1.ValidatingWebhookConfiguration{}).
		Owns(&networkingv1.NetworkPolicy{}).
		Owns(&appsv1.Deployment{}, reconciler.WithPredicates(predicates.DefaultDeploymentPredicate)).
		Watches(
			&extv1.CustomResourceDefinition{},
			reconciler.WithEventHandler(
				handlers.ToNamed(componentApi.SparkOperatorInstanceName)),
			reconciler.WithPredicates(
				labelpred.ForLabel(appLabelPrefix+"/"+componentName, "true")),
		).
		WithReconcilerOpts(
			reconciler.WithRelease(m.platformRelease),
		).
		WithAction(m.initialize).
		WithAction(m.upgradeIfNeeded).
		WithAction(kustomize.NewAction(
			kustomize.WithManifestsOptions(mk.WithEngineFS(m.kustomizeFS)),
			kustomize.WithNamespaceFn(moduleconfig.ApplicationsNamespaceGetter(cfg)),
		)).
		WithAction(fwtemplate.NewAction(
			fwtemplate.WithNamespaceFn(moduleconfig.ApplicationsNamespaceGetter(cfg)),
		)).
		WithAction(deploy.NewAction(
			deploy.WithCache(),
			deploy.WithApplyOrder(),
			deploy.WithLabel(appLabelPrefix+"/"+componentName, "true"),
			deploy.WithPartOfLabelDefault(componentName),
		)).
		WithAction(deployments.NewAction(
			deployments.InNamespaceFn(moduleconfig.ApplicationsNamespaceGetter(cfg)),
		)).
		WithAction(m.reportStatus).
		WithAction(gc.NewAction(moduleconfig.ApplicationsNamespaceGetter(cfg))).
		WithConditions(
			deployments.DefaultConditionType,
		).
		Build(ctx)

	if err != nil {
		return err
	}

	return nil
}
