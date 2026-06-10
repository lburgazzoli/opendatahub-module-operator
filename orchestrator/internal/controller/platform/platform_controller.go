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

package platform

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	configApi "github.com/lburgazzoli/opendatahub-module-operator/orchestrator/api/config/v1alpha1"
	"github.com/lburgazzoli/opendatahub-module-operator/orchestrator/pkg/config"
	"github.com/lburgazzoli/opendatahub-module-operator/orchestrator/pkg/controller/handlers"
	"github.com/lburgazzoli/opendatahub-module-operator/orchestrator/pkg/controller/predicates"
	"github.com/lburgazzoli/opendatahub-module-operator/orchestrator/pkg/module"
	"github.com/opendatahub-io/operator-actions-framework/api"
	"github.com/opendatahub-io/operator-actions-framework/controller/actions/deploy"
	"github.com/opendatahub-io/operator-actions-framework/controller/reconciler"
)

// +kubebuilder:rbac:groups=config.opendatahub.io,resources=platforms,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=config.opendatahub.io,resources=platforms/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=config.opendatahub.io,resources=platforms/finalizers,verbs=update
// +kubebuilder:rbac:groups=config.opendatahub.io,resources=platformoperators,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=config.opendatahub.io,resources=platformoperators/status,verbs=get;update;patch
// +kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch

func NewReconciler(
	ctx context.Context,
	mgr ctrl.Manager,
	registry *module.Registry,
	cfg *config.Config,
) error {
	r := &PlatformReconciler{
		registry: registry,
		cfg:      cfg,
		recorder: mgr.GetEventRecorder("platform-controller"),
	}

	_, err := reconciler.ReconcilerFor(mgr, &configApi.Platform{}).
		Owns(&configApi.PlatformOperator{},
			reconciler.WithPredicates(predicate.ResourceVersionChangedPredicate{}),
		).
		Watches(
			&configApi.PlatformOperator{},
			reconciler.WithEventHandler(handlers.ToNamed(configApi.PlatformInstanceName)),
			reconciler.WithPredicates(predicate.ResourceVersionChangedPredicate{}),
		).
		Watches(
			&corev1.ConfigMap{},
			reconciler.WithEventHandler(handlers.ToNamed(configApi.PlatformInstanceName)),
			reconciler.WithPredicates(
				predicates.Named(types.NamespacedName{
					Namespace: cfg.Namespace(),
					Name:      config.AdminAcksConfigMapName,
				}),
			),
		).
		WithReconcilerOpts(
			reconciler.WithRelease(api.Release{
				Name:    cfg.Release().Name,
				Version: cfg.Release().Version.Version,
			}),
		).
		WithAction(r.initialize).
		WithAction(r.checkAdminAcks).
		WithAction(r.ensureModules).
		WithAction(deploy.NewAction(
			deploy.WithCache(),
			deploy.WithApplyOrder(),
		)).
		WithAction(r.pruneModules).
		WithAction(r.advanceRunlevel).
		WithAction(r.aggregateStatus).
		WithConditions(
			configApi.ConditionUpToDate,
		).
		Build(ctx)

	return err
}
