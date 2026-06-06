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
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/source"

	configApi "github.com/lburgazzoli/opendatahub-module-operator/orchestrator/api/config/v1alpha1"
	orchestratorconfig "github.com/lburgazzoli/opendatahub-module-operator/orchestrator/pkg/config"
	"github.com/lburgazzoli/opendatahub-module-operator/orchestrator/pkg/module"
	"github.com/opendatahub-io/operator-actions-framework/controller/actions/deploy"
	"github.com/opendatahub-io/operator-actions-framework/controller/actions/status/deployments"
	"github.com/opendatahub-io/operator-actions-framework/controller/reconciler"
)

// NewModuleReconciler creates a single controller that handles all
// PlatformOperator CRs. It looks up the module by CR name, lazily creates
// the Helm engine, and gates reconciliation via the Orchestration state.
func NewModuleReconciler(
	ctx context.Context,
	mgr ctrl.Manager,
	o module.Orchestration,
	cfg *orchestratorconfig.Config,
) error {
	r := &ModuleReconciler{
		o:        o,
		cfg:      cfg,
		client:   mgr.GetClient(),
		contexts: make(map[string]*moduleContext),
	}

	_, err := reconciler.ReconcilerFor(mgr, &configApi.PlatformOperator{}).
		WithDynamicOwnership().
		WatchesRawSource(source.Channel(
			o.StateChanges(),
			handler.EnqueueRequestsFromMapFunc(r.enqueueEligibleModules),
		)).
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

// enqueueEligibleModules maps a state-change event to reconcile requests
// for PlatformOperator CRs whose modules should reconcile under the
// current mode and runlevel.
func (r *ModuleReconciler) enqueueEligibleModules(
	ctx context.Context,
	_ client.Object,
) []ctrl.Request {
	var list configApi.PlatformOperatorList
	if err := r.client.List(ctx, &list); err != nil {
		return nil
	}

	var requests []ctrl.Request
	for i := range list.Items {
		m := r.o.ModuleByName(list.Items[i].Name)
		if m == nil {
			continue
		}

		if !r.o.ShouldReconcileModule(m) {
			continue
		}

		requests = append(requests, ctrl.Request{
			NamespacedName: client.ObjectKeyFromObject(&list.Items[i]),
		})
	}

	return requests
}
