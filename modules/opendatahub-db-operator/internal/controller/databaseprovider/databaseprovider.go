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

package databaseprovider

import (
	"context"

	infraApi "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/api/infrastructure/v1alpha1"
	"github.com/opendatahub-io/odh-platform-utilities/framework/controller/actions/deploy"
	"github.com/opendatahub-io/odh-platform-utilities/framework/controller/handlers"
	fwpredicates "github.com/opendatahub-io/odh-platform-utilities/framework/controller/predicates"
	dependentpred "github.com/opendatahub-io/odh-platform-utilities/framework/controller/predicates/dependent"
	resourcespredicates "github.com/opendatahub-io/odh-platform-utilities/framework/controller/predicates/resources"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	moduleconfig "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/pkg/config"
	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/pkg/resources/gvk"
	"github.com/opendatahub-io/odh-platform-utilities/framework/controller/reconciler"
)

// Module holds process-lifetime state for this controller.
// It embeds Options so that task-specific dependencies can be added via
// With* constructors without changing the Module type. cfg and platformRelease
// live in Options but are not user-configurable (docs/framework.md).
type Controller struct {
	Options
}

func NewController(cfg *moduleconfig.Config, opts ...Option) *Controller {
	r := &Controller{
		Options: Options{
			cfg:             cfg,
			platformRelease: cfg.PlatformRelease(),
		},
	}
	for _, opt := range opts {
		opt.applyOption(&r.Options)
	}
	return r
}

// DatabaseProvider CRD
// +kubebuilder:rbac:groups=infrastructure.opendatahub.io,resources=databaseproviders,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=infrastructure.opendatahub.io,resources=databaseproviders/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=infrastructure.opendatahub.io,resources=databaseproviders/finalizers,verbs=update

// Internal provider owns these (task-08)
// +kubebuilder:rbac:groups=apps,resources=statefulsets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=persistentvolumeclaims,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=networking.k8s.io,resources=networkpolicies,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=cert-manager.io,resources=issuers;certificates,verbs=get;list;watch;create;update;patch;delete

func NewReconciler(
	ctx context.Context,
	mgr ctrl.Manager,
	cfg *moduleconfig.Config,
	fns ...Option,
) error {
	m := NewController(cfg, fns...)

	_, err := reconciler.ReconcilerFor(mgr, &infraApi.DatabaseProvider{}).
		Owns(&appsv1.StatefulSet{}, reconciler.WithPredicates(predicate.Or(
			fwpredicates.DefaultPredicate,
			resourcespredicates.StatusChanged(),
		))).
		Owns(&corev1.PersistentVolumeClaim{}).
		Owns(&corev1.Service{}).
		Owns(&corev1.ConfigMap{}).
		Owns(&corev1.Secret{}).
		Watches(
			&corev1.Secret{},
			reconciler.WithEventHandler(handlers.LabelToName("db.infrastructure.opendatahub.io/provider")),
		).
		Owns(&networkingv1.NetworkPolicy{}).
		WatchesGVK(
			gvk.CertManagerIssuer,
			reconciler.Dynamic(reconciler.CrdExists(gvk.CertManagerIssuer)),
			reconciler.WithPredicates(dependentpred.New()),
		).
		WatchesGVK(
			gvk.CertManagerCertificate,
			reconciler.Dynamic(reconciler.CrdExists(gvk.CertManagerCertificate)),
			reconciler.WithPredicates(dependentpred.New(dependentpred.WithWatchStatus(true))),
		).
		WithReconcilerOpts(
			reconciler.WithRelease(m.platformRelease),
			reconciler.WithDefaultRequeueAfter(cfg.DatabaseProvider.RetryInterval),
		).
		WithAction(m.reconcileExternalAction).
		WithAction(m.reconcileInternalAction).
		WithAction(deploy.NewAction(
			deploy.WithCache(),
			deploy.WithApplyOrder(),
		)).
		WithAction(m.internalReadinessAction).
		WithConditions(ConditionReachable, ConditionTLSConfiguration).
		Build(ctx)

	return err
}
