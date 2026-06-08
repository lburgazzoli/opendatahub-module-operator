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
	orchestratorconfig "github.com/lburgazzoli/opendatahub-module-operator/orchestrator/pkg/config"
	"github.com/lburgazzoli/opendatahub-module-operator/orchestrator/pkg/controller/handlers"
	controllerpredicates "github.com/lburgazzoli/opendatahub-module-operator/orchestrator/pkg/controller/predicates"
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
	cfg *orchestratorconfig.Config,
) error {
	r := &ModuleReconciler{
		registry:  registry,
		cfg:       cfg,
		client:    mgr.GetClient(),
		apiReader: mgr.GetAPIReader(),
		contexts:  make(map[string]*moduleContext),
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
			reconciler.WithPredicates(controllerpredicates.CreateOrResourceVersionChanged()),
			reconciler.Dynamic(reconciler.CrdExists(m.GVK)),
		)
	}

	_, err := b.
		WithEventFilter(controllerpredicates.LogAllEvents("PO event")).
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

// platformChangePredicate triggers only when the Platform CR's runlevel
// or distribution version changes.
func (r *ModuleReconciler) platformChangePredicate() predicate.Predicate {
	extractRunlevel := func(obj client.Object) int64 {
		if u, ok := obj.(*unstructured.Unstructured); ok {
			v, _, _ := unstructured.NestedInt64(u.Object, "status", "runlevel")
			return v
		}
		return 0
	}

	extractVersion := func(obj client.Object) string {
		if u, ok := obj.(*unstructured.Unstructured); ok {
			v, _, _ := unstructured.NestedString(u.Object, "status", "distribution", "version")
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
			if extractVersion(e.ObjectOld) != extractVersion(e.ObjectNew) {
				return true
			}
			return false
		},
	}
}

// eligibleModuleRequests maps a Platform CR event to reconcile requests for
// PlatformOperator CRs whose modules are at an eligible runlevel and haven't
// yet reported the expected distribution version.
func (r *ModuleReconciler) eligibleModuleRequests(
	ctx context.Context,
	obj client.Object,
) []ctrl.Request {
	targetDistribution := configApi.DistributionInfo{
		Name:    r.cfg.Distribution.Name,
		Version: r.cfg.Distribution.Version,
	}

	p := &configApi.Platform{}
	if err := r.client.Get(ctx, client.ObjectKey{Name: configApi.PlatformInstanceName}, p); err != nil {
		return nil
	}

	var list configApi.PlatformOperatorList
	if err := r.client.List(ctx, &list); err != nil {
		return nil
	}

	var requests []ctrl.Request
	for i := range list.Items {
		po := &list.Items[i]
		m := r.registry.ModuleByName(po.Name)

		switch {
		case m == nil:
			continue
		case m.Runlevel > p.Status.Runlevel:
			continue
		case po.Status.Distribution == targetDistribution:
			continue
		default:
			requests = append(requests, ctrl.Request{
				NamespacedName: client.ObjectKeyFromObject(po),
			})
		}
	}

	return requests
}
