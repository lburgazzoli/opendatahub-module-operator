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

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
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

// NewModuleReconciler creates a single controller that handles all
// PlatformOperator CRs. It watches the Platform CR for runlevel/version
// changes and enqueues only eligible modules.
func NewModuleReconciler(
	ctx context.Context,
	mgr ctrl.Manager,
	registry *module.ModuleRegistry,
	cfg *config.Config,
) error {
	r := &ModuleReconciler{
		registry: registry,
		cfg:      cfg,
		recorder: mgr.GetEventRecorder("platformoperator-controller"),
		contexts: make(map[string]*moduleContext),
	}

	b := reconciler.ReconcilerFor(mgr, &configApi.PlatformOperator{}).
		WithDynamicOwnership().
		Watches(
			&configApi.Platform{},
			reconciler.WithEventHandler(handler.EnqueueRequestsFromMapFunc(r.eligibleModuleRequests)),
			reconciler.WithPredicates(r.platformChangePredicate()),
		)

	for _, mod := range registry.Modules() {
		m := mod
		b = b.WatchesGVK(
			m.GVK,
			reconciler.WithEventHandler(handlers.ToNamed(m.EffectiveName())),
			reconciler.WithPredicates(predicate.ResourceVersionChangedPredicate{}),
			reconciler.Dynamic(reconciler.CrdExists(m.GVK)),
		)
	}

	_, err := b.
		WithAction(r.resolveModule).
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

// platformChangePredicate triggers only when the Platform CR's runlevel or
// current/target distribution version changes.
func (r *ModuleReconciler) platformChangePredicate() predicate.Predicate {
	extractRunlevel := func(obj client.Object) int64 {
		if u, ok := obj.(*unstructured.Unstructured); ok {
			v, _, _ := unstructured.NestedInt64(u.Object, "status", "runlevel")
			return v
		}
		return 0
	}

	extractCurrentVersion := func(obj client.Object) string {
		if u, ok := obj.(*unstructured.Unstructured); ok {
			v, _, _ := unstructured.NestedString(u.Object, "status", "distribution", "current", "version")
			return v
		}
		return ""
	}

	extractTargetVersion := func(obj client.Object) string {
		if u, ok := obj.(*unstructured.Unstructured); ok {
			v, _, _ := unstructured.NestedString(u.Object, "status", "distribution", "target", "version")
			return v
		}
		return ""
	}

	return predicate.Funcs{
		CreateFunc:  func(_ event.CreateEvent) bool { return true },
		DeleteFunc:  func(_ event.DeleteEvent) bool { return false },
		GenericFunc: func(_ event.GenericEvent) bool { return false },
		UpdateFunc: func(e event.UpdateEvent) bool {
			if extractRunlevel(e.ObjectOld) != extractRunlevel(e.ObjectNew) {
				return true
			}
			if extractCurrentVersion(e.ObjectOld) != extractCurrentVersion(e.ObjectNew) {
				return true
			}
			if extractTargetVersion(e.ObjectOld) != extractTargetVersion(e.ObjectNew) {
				return true
			}
			return false
		},
	}
}

// eligibleModuleRequests maps a Platform CR event to reconcile requests for
// all registered modules. The reconcile logic (checkRunlevel) decides whether
// each module should proceed, pause, or no-op.
func (r *ModuleReconciler) eligibleModuleRequests(
	_ context.Context,
	_ client.Object,
) []ctrl.Request {
	modules := r.registry.Modules()
	requests := make([]ctrl.Request, 0, len(modules))
	for _, m := range modules {
		requests = append(requests, ctrl.Request{
			NamespacedName: client.ObjectKey{Name: m.EffectiveName()},
		})
	}

	return requests
}
