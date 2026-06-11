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

package platformoperator

import (
	"context"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	configApi "github.com/lburgazzoli/opendatahub-module-operator/orchestrator/api/config/v1alpha1"
	"github.com/lburgazzoli/opendatahub-module-operator/orchestrator/pkg/config"
	"github.com/lburgazzoli/opendatahub-module-operator/orchestrator/pkg/controller/handlers"
	"github.com/lburgazzoli/opendatahub-module-operator/orchestrator/pkg/module"
	"github.com/opendatahub-io/operator-actions-framework/controller/actions/deploy"
	"github.com/opendatahub-io/operator-actions-framework/controller/actions/status/deployments"
	"github.com/opendatahub-io/operator-actions-framework/controller/reconciler"
)

// NewReconciler creates a single controller that handles all
// PlatformOperator CRs. It watches the Platform CR for runlevel/version
// changes and enqueues only eligible modules.
func NewReconciler(
	ctx context.Context,
	mgr ctrl.Manager,
	registry *module.Registry,
	cfg *config.Config,
) error {
	r := &PlatformOperatorReconciler{
		registry: registry,
		cfg:      cfg,
		client:   mgr.GetClient(),
		recorder: mgr.GetEventRecorder("platformoperator-controller"),
		contexts: make(map[string]*moduleContext),
	}

	for _, mod := range registry.Modules() {
		mc, err := newModuleContext(mod, cfg)
		if err != nil {
			return err
		}
		r.contexts[mod.Name] = mc
	}

	b := reconciler.ReconcilerFor(mgr, &configApi.PlatformOperator{}).
		WithDynamicOwnership().
		Watches(
			&configApi.Platform{},
			reconciler.WithEventHandler(handler.EnqueueRequestsFromMapFunc(r.eligibleModuleRequests)),
			reconciler.WithPredicates(predicate.Or(
				predicate.GenerationChangedPredicate{},
				r.platformChangePredicate(),
			)),
		)

	for _, mod := range registry.Modules() {
		m := mod
		b = b.WatchesGVK(
			m.GVK,
			reconciler.WithEventHandler(handlers.ToNamed(m.Name)),
			reconciler.WithPredicates(predicate.ResourceVersionChangedPredicate{}),
			reconciler.Dynamic(reconciler.CrdExists(m.GVK)),
		)
	}

	_, err := b.
		WithAction(r.checkRunlevel).
		WithAction(r.ensureNamespace).
		WithAction(r.renderChart).
		WithAction(deploy.NewAction(
			deploy.WithCache(),
			deploy.WithApplyOrder(),
		)).
		WithAction(deployments.NewAction(
			deployments.InNamespaceFn(r.moduleNamespace),
		)).
		WithAction(r.pruneOrphans).
		WithAction(r.reportStatus).
		Build(ctx)

	return err
}
