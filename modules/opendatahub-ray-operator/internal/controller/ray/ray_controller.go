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

package ray

import (
	"context"

	admissionv1 "k8s.io/api/admissionregistration/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	extv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	ctrl "sigs.k8s.io/controller-runtime"

	componentApi "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-ray-operator/api/components/v1alpha1"
	moduleconfig "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-ray-operator/pkg/config"
	module "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-ray-operator/pkg/module"
	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-ray-operator/pkg/resources/gvk"
	fwapi "github.com/opendatahub-io/odh-platform-utilities/framework/api"
	"github.com/opendatahub-io/odh-platform-utilities/framework/controller/actions/deploy"
	"github.com/opendatahub-io/odh-platform-utilities/framework/controller/actions/gc"
	"github.com/opendatahub-io/odh-platform-utilities/framework/controller/actions/render/kustomize"
	"github.com/opendatahub-io/odh-platform-utilities/framework/controller/actions/sanitycheck"
	"github.com/opendatahub-io/odh-platform-utilities/framework/controller/actions/status/deployments"
	fwreleases "github.com/opendatahub-io/odh-platform-utilities/framework/controller/actions/status/releases"
	"github.com/opendatahub-io/odh-platform-utilities/framework/controller/handlers"
	"github.com/opendatahub-io/odh-platform-utilities/framework/controller/predicates"
	labelpred "github.com/opendatahub-io/odh-platform-utilities/framework/controller/predicates/label"
	"github.com/opendatahub-io/odh-platform-utilities/framework/controller/reconciler"
)

const appLabelPrefix = "app.opendatahub.io"

// Module operator's own CRD
// +kubebuilder:rbac:groups=components.platform.opendatahub.io,resources=rays,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=components.platform.opendatahub.io,resources=rays/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=components.platform.opendatahub.io,resources=rays/finalizers,verbs=update

// Resources deployed by the kustomize manifests
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=services;serviceaccounts;configmaps;secrets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=clusterroles;clusterrolebindings;roles;rolebindings,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=security.openshift.io,resources=securitycontextconstraints,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apiextensions.k8s.io,resources=customresourcedefinitions,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=admissionregistration.k8s.io,resources=mutatingwebhookconfigurations,verbs=get;list;watch;create;update;patch;delete

// Permissions required by the deployed kuberay-operator ClusterRoles
// (the module operator SA must hold these to create ClusterRoles that grant them)
// +kubebuilder:rbac:groups=ray.io,resources=rayclusters;rayjobs;rayservices,verbs=get;list;watch;create;delete;deletecollection;patch;update
// +kubebuilder:rbac:groups=ray.io,resources=rayclusters/finalizers;rayjobs/finalizers;rayservices/finalizers,verbs=get;update
// +kubebuilder:rbac:groups=ray.io,resources=rayclusters/status;rayjobs/status;rayservices/status,verbs=get;patch;update
// +kubebuilder:rbac:groups="",resources=pods;pods/status,verbs=get;list;watch;create;delete;deletecollection;patch;update
// +kubebuilder:rbac:groups="",resources=pods/proxy;services/proxy;services/status,verbs=get;create;patch;update
// +kubebuilder:rbac:groups="",resources=endpoints,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=events,verbs=get;list;watch;create;delete;patch;update
// +kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch;create;delete;patch;update
// +kubebuilder:rbac:groups=networking.k8s.io,resources=ingresses;networkpolicies,verbs=get;list;watch;create;delete;patch;update
// +kubebuilder:rbac:groups=networking.k8s.io,resources=ingressclasses,verbs=get;list;watch
// +kubebuilder:rbac:groups=extensions,resources=ingresses,verbs=get;list;watch;create;delete;patch;update
// +kubebuilder:rbac:groups=coordination.k8s.io,resources=leases,verbs=get;list;create;update
// +kubebuilder:rbac:groups=cert-manager.io,resources=certificates;issuers,verbs=get;list;watch;create;delete;patch;update
// +kubebuilder:rbac:groups=cert-manager.io,resources=certificates/status,verbs=get;patch;update
// +kubebuilder:rbac:groups=authentication.k8s.io,resources=tokenreviews,verbs=create
// +kubebuilder:rbac:groups=authorization.k8s.io,resources=subjectaccessreviews,verbs=create
// +kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=gateways,verbs=get;list;watch
// +kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=httproutes;referencegrants,verbs=get;list;watch;create;delete;patch;update
// +kubebuilder:rbac:groups=config.openshift.io,resources=authentications;oauths,verbs=get;list;watch
// +kubebuilder:rbac:groups=config.openshift.io,resources=authentications/status;oauths/status,verbs=get;list;watch
// +kubebuilder:rbac:groups=operator.openshift.io,resources=kubeapiservers,verbs=get;list;watch
// +kubebuilder:rbac:groups=operator.openshift.io,resources=kubeapiservers/status,verbs=get;list;watch
// +kubebuilder:rbac:groups=route.openshift.io,resources=routes,verbs=get;list;watch;create;delete;patch;update

func NewReconciler(
	ctx context.Context,
	mgr ctrl.Manager,
	cfg *moduleconfig.Config,
	rel componentApi.Release,
) error {
	m, err := NewModule(cfg)
	if err != nil {
		return err
	}

	r, err := reconciler.ReconcilerFor(mgr, &componentApi.Ray{}).
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
		OwnsGVK(gvk.SecurityContextConstraints).
		Watches(
			&extv1.CustomResourceDefinition{},
			reconciler.WithEventHandler(
				handlers.ToNamed(componentApi.RayInstanceName)),
			reconciler.WithPredicates(
				labelpred.ForLabel(appLabelPrefix+"/"+LegacyComponentName, "true")),
		).
		WatchesGVK(gvk.CodeFlare, reconciler.Dynamic(reconciler.CrdExists(gvk.CodeFlare))).
		WithReconcilerOpts(
			reconciler.WithRelease(fwapi.Release{
				Name:    fwapi.Platform(rel.Name),
				Version: rel.Version.Version,
			}),
		).
		WithAction(sanitycheck.NewAction(
			sanitycheck.WithUnwantedResource(gvk.CodeFlare, module.CodeFlarePresentMessage),
		)).
		WithAction(m.initialize).
		WithAction(m.upgradeIfNeeded).
		WithAction(fwreleases.NewAction()).
		WithAction(kustomize.NewAction()).
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

	r.Release = fwapi.Release{
		Name:    fwapi.Platform(rel.Name),
		Version: rel.Version.Version,
	}

	return nil
}
