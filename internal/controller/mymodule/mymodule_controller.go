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

package mymodule

import (
	"context"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	ctrl "sigs.k8s.io/controller-runtime"

	componentApi "github.com/lburgazzoli/opendatahub-module-operator/api/components/v1alpha1"
	moduleconfig "github.com/lburgazzoli/opendatahub-module-operator/pkg/config"
	"github.com/opendatahub-io/opendatahub-operator/v2/api/common"
	"github.com/opendatahub-io/opendatahub-operator/v2/pkg/controller/actions/deploy"
	"github.com/opendatahub-io/opendatahub-operator/v2/pkg/controller/actions/gc"
	"github.com/opendatahub-io/opendatahub-operator/v2/pkg/controller/actions/render/kustomize"
	"github.com/opendatahub-io/opendatahub-operator/v2/pkg/controller/actions/status/deployments"
	"github.com/opendatahub-io/opendatahub-operator/v2/pkg/controller/actions/status/releases"
	"github.com/opendatahub-io/opendatahub-operator/v2/pkg/controller/handlers"
	"github.com/opendatahub-io/opendatahub-operator/v2/pkg/controller/precondition"
	"github.com/opendatahub-io/opendatahub-operator/v2/pkg/controller/predicates"
	"github.com/opendatahub-io/opendatahub-operator/v2/pkg/controller/predicates/resources"
	"github.com/opendatahub-io/opendatahub-operator/v2/pkg/controller/reconciler"
	"github.com/opendatahub-io/opendatahub-operator/v2/pkg/controller/status"
	"github.com/opendatahub-io/opendatahub-operator/v2/pkg/metadata/labels"
)

// +kubebuilder:rbac:groups=components.platform.opendatahub.io,resources=mymodules,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=components.platform.opendatahub.io,resources=mymodules/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=components.platform.opendatahub.io,resources=mymodules/finalizers,verbs=update
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=services;serviceaccounts;configmaps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=networking.k8s.io,resources=ingresses,verbs=get;list;watch;patch;update
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=clusterroles;clusterrolebindings,verbs=get;list;watch;create;update;patch;delete

// NewReconciler sets up the controller with the Manager using the ReconcilerFor
// builder and the standard action pipeline.
//
// This is an example implementation that demonstrates the framework's
// capabilities: preconditions, lifecycle actions on a Module struct,
// watches on external resources, webhooks, and condition management.
// Developers are free to implement controllers in whatever way suits
// their module — the framework imposes no particular structure beyond
// the actions.Fn signature.
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

	r, err := reconciler.ReconcilerFor(mgr, &componentApi.MyModule{}).
		// Owned resources: the controller watches these and reconciles
		// when they change.
		Owns(&appsv1.Deployment{}, reconciler.WithPredicates(predicates.DefaultDeploymentPredicate)).
		Owns(&corev1.Service{}).
		Owns(&corev1.ServiceAccount{}).
		Owns(&corev1.ConfigMap{}).
		Owns(&rbacv1.ClusterRole{}).
		Owns(&rbacv1.ClusterRoleBinding{}).
		// Watch the required Ingress in the application namespace.
		// When it is created, updated, or deleted the singleton MyModule
		// CR is re-enqueued so the precondition re-evaluates.
		Watches(&networkingv1.Ingress{},
			reconciler.WithPredicates(resources.CreatedOrUpdatedOrDeletedNamed(IngressName)),
			reconciler.WithEventHandler(handlers.ToNamed(componentApi.MyModuleInstanceName)),
		).
		// Preconditions: halt the pipeline if the required Ingress is
		// missing. Nothing is deployed until it exists.
		WithPreCondition(precondition.NewPreCondition(
			m.checkIngress,
			precondition.WithConditionType(ConditionIngressAvailable),
			precondition.WithStopReconciliation(),
		)).
		// Action pipeline:
		//   initialize -> upgrade ->
		//   kustomize -> deploy ->
		//   deployments -> releases -> reportStatus ->
		//   gc
		WithAction(m.initialize).
		WithAction(m.upgradeIfNeeded).
		WithAction(kustomize.NewAction(
			kustomize.WithLabel(labels.PlatformPartOf, componentName),
		)).
		WithAction(deploy.NewAction(
			deploy.WithCache(),
		)).
		WithAction(deployments.NewAction()).
		WithAction(releases.NewAction()).
		// reportStatus runs after deployments and releases have populated
		// their conditions and release info, so it can override or enrich
		// any previously set status fields.
		WithAction(m.reportStatus).
		WithAction(gc.NewAction(
			gc.InNamespace(cfg.ApplicationsNamespace),
		)).
		WithConditions(
			status.ConditionDeploymentsAvailable,
			ConditionIngressAvailable,
		).
		Build(ctx)

	if err != nil {
		return err
	}

	// The reconciler framework reads Release from cluster.GetRelease(),
	// which is only populated by cluster.Init() (called by the main ODH
	// operator). A standalone module operator must set it explicitly so
	// the deploy action stamps the correct platform.opendatahub.io/type
	// and platform.opendatahub.io/version annotations on managed resources.
	r.Release = rel

	if cfg.WebhooksEnabled {
		if err := m.RegisterWebhooks(mgr); err != nil {
			return err
		}
	}

	return nil
}
