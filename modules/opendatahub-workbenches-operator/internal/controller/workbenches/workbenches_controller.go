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

package workbenches

import (
	"context"

	imagev1 "github.com/openshift/api/image/v1"
	admissionv1 "k8s.io/api/admissionregistration/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	extv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	ctrl "sigs.k8s.io/controller-runtime"

	componentApi "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-workbenches-operator/api/components/v1alpha1"
	moduleconfig "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-workbenches-operator/pkg/config"
	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-workbenches-operator/pkg/controller/handlers"
	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-workbenches-operator/pkg/controller/predicates"
	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-workbenches-operator/pkg/resources/gvk"
	"github.com/opendatahub-io/odh-platform-utilities/framework/controller/actions/deploy"
	"github.com/opendatahub-io/odh-platform-utilities/framework/controller/actions/gc"
	"github.com/opendatahub-io/odh-platform-utilities/framework/controller/actions/render/kustomize"
	"github.com/opendatahub-io/odh-platform-utilities/framework/controller/actions/status/deployments"
	fwimagestreams "github.com/opendatahub-io/odh-platform-utilities/framework/controller/actions/status/imagestreams"
	fwreleases "github.com/opendatahub-io/odh-platform-utilities/framework/controller/actions/status/releases"
	"github.com/opendatahub-io/odh-platform-utilities/framework/controller/reconciler"
	mk "github.com/opendatahub-io/odh-platform-utilities/framework/render/kustomize"
)

const appLabelPrefix = "app.opendatahub.io"

// Module operator's own CRD
// +kubebuilder:rbac:groups=components.platform.opendatahub.io,resources=workbenches,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=components.platform.opendatahub.io,resources=workbenches/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=components.platform.opendatahub.io,resources=workbenches/finalizers,verbs=update

// MLflowOperator is read uncached to determine mlflow-enabled state.
// +kubebuilder:rbac:groups=components.platform.opendatahub.io,resources=mlflowoperators,verbs=get;list;watch

// OpenShift cluster Ingress is read to compute the gateway domain fallback.
// +kubebuilder:rbac:groups=config.openshift.io,resources=ingresses,verbs=get;list;watch

// Baseline CRD and protected-metrics RBAC required by every module operator
// +kubebuilder:rbac:groups=apiextensions.k8s.io,resources=customresourcedefinitions,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=authentication.k8s.io,resources=tokenreviews,verbs=create
// +kubebuilder:rbac:groups=authorization.k8s.io,resources=subjectaccessreviews,verbs=create
// +kubebuilder:rbac:urls=/metrics,verbs=get

// Resources deployed by the kustomize manifests
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=services;serviceaccounts;configmaps;secrets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=namespaces,verbs=get;list;watch;create;update;patch
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=clusterroles;clusterrolebindings;roles;rolebindings,verbs=get;list;watch;create;update;patch;delete;bind
// +kubebuilder:rbac:groups=admissionregistration.k8s.io,resources=mutatingwebhookconfigurations;validatingwebhookconfigurations,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=image.openshift.io,resources=imagestreams,verbs=get;list;watch;create;update;patch;delete

// Permissions required by the deployed odh-notebook-controller ClusterRole
// +kubebuilder:rbac:groups=kubeflow.org,resources=notebooks,verbs=get;list;watch;create;delete;patch;update
// +kubebuilder:rbac:groups=kubeflow.org,resources=notebooks/finalizers,verbs=get;update;patch;delete
// +kubebuilder:rbac:groups=kubeflow.org,resources=notebooks/status,verbs=get;list;watch;create;delete;patch;update
// +kubebuilder:rbac:groups=config.openshift.io,resources=proxies,verbs=get;list;watch
// +kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=gateways,verbs=get;list;watch
// +kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=httproutes;referencegrants,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=networking.k8s.io,resources=networkpolicies;networkpolicies/finalizers,verbs=get;list;watch;create;update;patch
// +kubebuilder:rbac:groups=oauth.openshift.io,resources=oauthclients,verbs=get;list;watch;update;patch;delete
// +kubebuilder:rbac:groups=route.openshift.io,resources=routes,verbs=get;list;watch
// +kubebuilder:rbac:groups=datasciencepipelinesapplications.opendatahub.io,resources=datasciencepipelinesapplications,verbs=get;list;watch;patch;update;delete
// +kubebuilder:rbac:groups=datasciencepipelinesapplications.opendatahub.io,resources=datasciencepipelinesapplications/api,verbs=get;create;delete;patch;update

// Permissions required by the deployed kf-notebook-controller ClusterRole
// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch;delete
// +kubebuilder:rbac:groups="",resources=events,verbs=get;list;watch;create;patch
// +kubebuilder:rbac:groups=apps,resources=statefulsets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=networking.istio.io,resources=virtualservices,verbs=get;list;watch;create;update;patch;delete

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

	m.apiReader = mgr.GetAPIReader()

	_, err = reconciler.ReconcilerFor(mgr, &componentApi.Workbenches{}).
		Owns(&corev1.ConfigMap{}).
		Owns(&corev1.Secret{}).
		Owns(&rbacv1.ClusterRoleBinding{}).
		Owns(&rbacv1.ClusterRole{}).
		Owns(&rbacv1.Role{}).
		Owns(&rbacv1.RoleBinding{}).
		Owns(&corev1.ServiceAccount{}).
		Owns(&corev1.Service{}).
		Owns(&appsv1.Deployment{}, reconciler.WithPredicates(predicates.DefaultDeploymentPredicate)).
		Owns(&admissionv1.MutatingWebhookConfiguration{}).
		Owns(&admissionv1.ValidatingWebhookConfiguration{}).
		Owns(&imagev1.ImageStream{}).
		// Watch CRDs labelled for workbenches and also the MLflowOperator CRD so that
		// when mlflow is installed or removed the controller re-evaluates mlflow-enabled.
		Watches(
			&extv1.CustomResourceDefinition{},
			reconciler.WithEventHandler(
				handlers.ToNamed(componentApi.WorkbenchesInstanceName)),
			reconciler.WithPredicates(predicates.Or(
				predicates.ForComponentLabel(appLabelPrefix+"/"+LegacyComponentName, "true"),
				predicates.CreatedOrDeletedNamed(mlflowOperatorCRDName),
			)),
		).
		Watches(&corev1.Namespace{}).
		WatchesGVK(
			gvk.MLflowOperator,
			reconciler.Dynamic(reconciler.CrdExists(gvk.MLflowOperator)),
			reconciler.WithEventHandler(handlers.ToNamed(componentApi.WorkbenchesInstanceName)),
		).
		Watches(
			&imagev1.ImageStream{},
			reconciler.WithEventHandler(handlers.ToNamed(componentApi.WorkbenchesInstanceName)),
			reconciler.WithPredicates(
				predicates.ForComponentLabel(appLabelPrefix+"/"+LegacyComponentName, "true"),
			),
		).
		WithReconcilerOpts(
			reconciler.WithRelease(m.release),
		).
		WithAction(m.initialize).
		WithAction(m.upgradeIfNeeded).
		WithAction(m.customizeManifests).
		WithAction(fwreleases.NewAction(
			fwreleases.WithMetadataFilePath(metadataFilePath),
		)).
		WithAction(m.configureDependencies).
		WithAction(kustomize.NewAction(
			kustomize.WithManifestsOptions(mk.WithEngineFS(m.renderFS)),
			kustomize.WithNamespaceFn(moduleconfig.ApplicationsNamespaceGetter(cfg)),
		)).
		WithAction(deploy.NewAction(
			deploy.WithCache(),
			deploy.WithApplyOrder(),
			deploy.WithLabel(appLabelPrefix+"/"+LegacyComponentName, "true"),
			deploy.WithPartOfLabelDefault(LegacyComponentName),
		)).
		WithAction(deployments.NewAction(
			deployments.InNamespaceFn(moduleconfig.ApplicationsNamespaceGetter(cfg)),
		)).
		WithAction(fwimagestreams.NewAction(
			fwimagestreams.InNamespace(cfg.ApplicationsNamespace),
		)).
		WithAction(m.reportStatus).
		WithAction(gc.NewAction(moduleconfig.ApplicationsNamespaceGetter(cfg))).
		WithConditions(conditionTypes...).
		Build(ctx)

	if err != nil {
		return err
	}

	if cfg.Controller.Webhook.Enabled {
		if err := m.RegisterWebhooks(mgr); err != nil {
			return err
		}
	}

	return nil
}
