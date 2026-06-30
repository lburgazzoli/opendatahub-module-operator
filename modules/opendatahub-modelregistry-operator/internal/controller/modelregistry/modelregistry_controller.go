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

package modelregistry

import (
	"context"

	admissionv1 "k8s.io/api/admissionregistration/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	extv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	ctrl "sigs.k8s.io/controller-runtime"

	componentApi "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-modelregistry-operator/api/components/v1alpha1"
	moduleconfig "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-modelregistry-operator/pkg/config"
	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-modelregistry-operator/pkg/resources/gvk"
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

// Module operator's own CRD
// +kubebuilder:rbac:groups=components.platform.opendatahub.io,resources=modelregistries,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=components.platform.opendatahub.io,resources=modelregistries/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=components.platform.opendatahub.io,resources=modelregistries/finalizers,verbs=update

// Baseline RBAC required for all module operators
// +kubebuilder:rbac:groups=apiextensions.k8s.io,resources=customresourcedefinitions,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=authentication.k8s.io,resources=tokenreviews,verbs=create
// +kubebuilder:rbac:groups=authorization.k8s.io,resources=subjectaccessreviews,verbs=create
// +kubebuilder:rbac:urls=/metrics,verbs=get

// Core resources owned by the controller (from kustomize build output)
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=configmaps;secrets;services;serviceaccounts;namespaces,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=persistentvolumeclaims,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=endpoints;pods;pods/log,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch
// +kubebuilder:rbac:groups=events.k8s.io,resources=events,verbs=create;patch
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=clusterroles;clusterrolebindings;roles;rolebindings,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=admissionregistration.k8s.io,resources=mutatingwebhookconfigurations;validatingwebhookconfigurations,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=coordination.k8s.io,resources=leases,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=template.openshift.io,resources=templates,verbs=get;list;watch;create;update;patch;delete

// Permissions required by the deployed model-registry-operator ClusterRole
// +kubebuilder:rbac:groups=config.openshift.io,resources=ingresses,verbs=get;list;watch
// +kubebuilder:rbac:groups=config.openshift.io,resources=apiservers,verbs=get;list;watch
// +kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=httproutes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=referencegrants,verbs=get;list;watch;create
// +kubebuilder:rbac:groups=migration.k8s.io,resources=storageversionmigrations,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=modelregistry.opendatahub.io,resources=modelregistries,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=modelregistry.opendatahub.io,resources=modelregistries/finalizers,verbs=update
// +kubebuilder:rbac:groups=modelregistry.opendatahub.io,resources=modelregistries/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=networking.k8s.io,resources=networkpolicies,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=route.openshift.io,resources=routes;routes/custom-host,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=services.platform.opendatahub.io,resources=auths,verbs=get;list;watch
// +kubebuilder:rbac:groups=user.openshift.io,resources=groups,verbs=get;list;watch;create;update;patch;delete

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

	_, err = reconciler.ReconcilerFor(mgr, &componentApi.ModelRegistry{}).
		Owns(&corev1.ConfigMap{}).
		Owns(&corev1.Secret{}).
		Owns(&rbacv1.Role{}).
		Owns(&rbacv1.RoleBinding{}).
		Owns(&rbacv1.ClusterRole{}).
		Owns(&rbacv1.ClusterRoleBinding{}).
		Owns(&corev1.Service{}).
		Owns(&corev1.ServiceAccount{}).
		Owns(&appsv1.Deployment{}, reconciler.WithPredicates(predicates.DefaultDeploymentPredicate)).
		Owns(&admissionv1.MutatingWebhookConfiguration{}).
		Owns(&admissionv1.ValidatingWebhookConfiguration{}).
		OwnsGVK(gvk.OpenshiftTemplate).
		Watches(
			&corev1.Namespace{},
		).
		Watches(
			&extv1.CustomResourceDefinition{},
			reconciler.WithEventHandler(
				handlers.ToNamed(componentApi.ModelRegistryInstanceName)),
			reconciler.WithPredicates(
				labelpred.ForLabel(appLabelPrefix+"/"+LegacyComponentName, "true")),
		).
		WithReconcilerOpts(
			reconciler.WithRelease(m.platformRelease),
		).
		WithAction(m.stageManifests).
		WithAction(m.upgradeIfNeeded).
		WithAction(m.customizeManifests).
		WithAction(kustomize.NewAction(
			kustomize.WithManifestsOptions(mk.WithEngineFS(m.kustomizeFS)),
			kustomize.WithNamespaceFn(moduleconfig.ApplicationsNamespaceGetter(cfg)),
		)).
		WithAction(fwtemplate.NewAction(
			fwtemplate.WithNamespaceFn(moduleconfig.ApplicationsNamespaceGetter(cfg)),
		)).
		WithAction(m.configureDependencies).
		WithAction(deploy.NewAction(
			deploy.WithCache(),
			deploy.WithApplyOrder(),
			deploy.WithLabel(appLabelPrefix+"/"+LegacyComponentName, "true"),
			deploy.WithPartOfLabelDefault(LegacyComponentName),
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
