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

package trustyai

import (
	"context"

	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	extv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	componentApi "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-trustyai-operator/api/components/v1alpha1"
	moduleconfig "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-trustyai-operator/pkg/config"
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
	pkgresources "github.com/opendatahub-io/opendatahub-operator/v2/pkg/resources"
)

const (
	// InferenceServicesCRDName is the CRD name TrustyAI depends on.
	InferenceServicesCRDName = "inferenceservices.serving.kserve.io"
)

// isInferenceServicesCRD checks if obj is the InferenceServices CRD managed by KServe.
// Mirrors the monolith's isInferenceServicesCRD function.
func isInferenceServicesCRD(obj client.Object) bool {
	if obj.GetName() != InferenceServicesCRDName {
		return false
	}
	return pkgresources.HasLabel(obj, labels.ODH.Component("kserve"), labels.True)
}

// +kubebuilder:rbac:groups=components.platform.opendatahub.io,resources=trustyais,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=components.platform.opendatahub.io,resources=trustyais/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=components.platform.opendatahub.io,resources=trustyais/finalizers,verbs=update
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=services;serviceaccounts;configmaps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=clusterroles;clusterrolebindings;roles;rolebindings,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apiextensions.k8s.io,resources=customresourcedefinitions,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps,resources=deployments/finalizers;deployments/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch;create;delete;patch
// +kubebuilder:rbac:groups=monitoring.coreos.com,resources=servicemonitors,verbs=get;list;watch;create;update
// +kubebuilder:rbac:groups=coordination.k8s.io,resources=leases,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=persistentvolumeclaims;persistentvolumes;namespaces,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch;update
// +kubebuilder:rbac:groups=authentication.k8s.io,resources=tokenreviews,verbs=create
// +kubebuilder:rbac:groups=authorization.k8s.io,resources=subjectaccessreviews,verbs=create
// +kubebuilder:rbac:groups=serving.kserve.io,resources=inferenceservices;servingruntimes,verbs=get;list;watch
// +kubebuilder:rbac:groups=trustyai.opendatahub.io,resources=trustyaiservices;trustyaiservices/status;trustyaiservices/finalizers,verbs=get;list;watch;create;delete;patch;update
// +kubebuilder:rbac:groups=trustyai.opendatahub.io,resources=lmevaljobs;lmevaljobs/status;lmevaljobs/finalizers,verbs=get;list;watch;create;delete;patch;update
// +kubebuilder:rbac:groups=trustyai.opendatahub.io,resources=evalhubs;evalhubs/status;evalhubs/finalizers;evalhubs/proxy,verbs=get;list;watch;create;delete;patch;update
// +kubebuilder:rbac:groups=trustyai.opendatahub.io,resources=guardrailsorchestrators;guardrailsorchestrators/status;guardrailsorchestrators/finalizers,verbs=get;list;watch;create;delete;patch;update
// +kubebuilder:rbac:groups=trustyai.opendatahub.io,resources=nemoguardrails;nemoguardrails/status;nemoguardrails/finalizers,verbs=get;list;watch;create;delete;patch;update
// +kubebuilder:rbac:groups=trustyai.opendatahub.io,resources=collections;providers;status-events,verbs=get;list;create
// +kubebuilder:rbac:groups=networking.istio.io,resources=destinationrules;virtualservices,verbs=get;list;watch;create;delete;patch;update
// +kubebuilder:rbac:groups=kueue.x-k8s.io,resources=workloads,verbs=get;list;watch;patch
// +kubebuilder:rbac:groups=mlflow.kubeflow.org,resources=experiments,verbs=get;list;create;update;delete
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

	r, err := reconciler.ReconcilerFor(mgr, &componentApi.TrustyAI{}).
		Owns(&corev1.ConfigMap{}).
		Owns(&corev1.ServiceAccount{}).
		Owns(&rbacv1.ClusterRoleBinding{}).
		Owns(&rbacv1.ClusterRole{}).
		Owns(&rbacv1.Role{}).
		Owns(&rbacv1.RoleBinding{}).
		Owns(&corev1.Service{}).
		Owns(&monitoringv1.ServiceMonitor{}).
		Owns(&appsv1.Deployment{}, reconciler.WithPredicates(predicates.DefaultDeploymentPredicate)).
		Watches(
			&extv1.CustomResourceDefinition{},
			reconciler.WithEventHandler(
				handlers.ToNamed(componentApi.TrustyAIInstanceName)),
			reconciler.WithPredicates(predicate.Or(
				// Re-enqueue when TrustyAI-labelled CRDs change.
				component.ForLabel(labels.ODH.Component(LegacyComponentName), labels.True),
				// Also re-enqueue when InferenceServices CRD appears or disappears
				// (dependency on KServe). Mirrors the monolith's custom predicate.
				predicate.Funcs{
					CreateFunc:  func(e event.CreateEvent) bool { return isInferenceServicesCRD(e.Object) },
					UpdateFunc:  func(e event.UpdateEvent) bool { return false },
					DeleteFunc:  func(e event.DeleteEvent) bool { return isInferenceServicesCRD(e.Object) },
					GenericFunc: func(e event.GenericEvent) bool { return false },
				},
			)),
		).
		WithAction(m.checkPreConditions).
		WithAction(m.initialize).
		WithAction(m.upgradeIfNeeded).
		WithAction(m.createConfigMap).
		WithAction(releases.NewAction()).
		WithAction(kustomize.NewAction(
			kustomize.WithLabel(labels.ODH.Component(LegacyComponentName), labels.True),
			kustomize.WithLabel(labels.K8SCommon.PartOf, LegacyComponentName),
		)).
		WithAction(deploy.NewAction(
			deploy.WithCache(),
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
