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
	"cmp"
	"context"
	"fmt"
	"slices"

	k8serr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/controller-runtime/pkg/client"

	configApi "github.com/lburgazzoli/opendatahub-module-operator/orchestrator/api/config/v1alpha1"
	orchestratorconfig "github.com/lburgazzoli/opendatahub-module-operator/orchestrator/pkg/config"
	"github.com/lburgazzoli/opendatahub-module-operator/orchestrator/pkg/module"
	"github.com/opendatahub-io/operator-actions-framework/controller/types"
)

type platformActions struct {
	registry *module.ModuleRegistry
	cfg      *orchestratorconfig.Config
}

// initialize sets the runlevel from Platform CR status. On a fresh Platform
// (runlevel 0), initializes to the first runlevel that has modules.
func (a *platformActions) initialize(_ context.Context, rr *types.ReconciliationRequest) error {
	obj, ok := rr.Instance.(*configApi.Platform)
	if !ok {
		return fmt.Errorf("instance is not a Platform")
	}

	if obj.Status.Runlevel == 0 {
		obj.Status.Runlevel = a.registry.FirstRunlevel()
	}

	return nil
}

// checkAdminAcks is a placeholder for future admin acknowledgment checks.
func (a *platformActions) checkAdminAcks(_ context.Context, _ *types.ReconciliationRequest) error {
	return nil
}

// ensureModules builds PlatformOperator resources from spec.modules.
func (a *platformActions) ensureModules(_ context.Context, rr *types.ReconciliationRequest) error {
	obj, ok := rr.Instance.(*configApi.Platform)
	if !ok {
		return fmt.Errorf("instance is not a Platform")
	}

	for _, name := range obj.Spec.Modules {
		m := a.registry.ModuleByName(name)
		if m == nil {
			return fmt.Errorf("module %q not registered", name)
		}

		po := configApi.PlatformOperator{}
		po.SetName(m.EffectiveName())

		if err := rr.AddResources(&po); err != nil {
			return fmt.Errorf("%w", err)
		}
	}

	rr.Generated = true

	return nil
}

// pruneModules deletes PlatformOperators that are no longer present in spec.modules.
func (a *platformActions) pruneModules(ctx context.Context, rr *types.ReconciliationRequest) error {
	obj, ok := rr.Instance.(*configApi.Platform)
	if !ok {
		return fmt.Errorf("instance is not a Platform")
	}

	desired := sets.New[string]()
	for _, name := range obj.Spec.Modules {
		m := a.registry.ModuleByName(name)
		if m != nil {
			desired.Insert(m.EffectiveName())
			continue
		}

		desired.Insert(name)
	}

	var poList configApi.PlatformOperatorList
	if err := rr.Client.List(ctx, &poList); err != nil {
		return fmt.Errorf("listing PlatformOperators: %w", err)
	}

	propagation := metav1.DeletePropagationForeground

	for i := range poList.Items {
		po := &poList.Items[i]
		if desired.Has(po.Name) || !po.GetDeletionTimestamp().IsZero() {
			continue
		}

		if err := rr.Client.Delete(ctx, po, &client.DeleteOptions{
			PropagationPolicy: &propagation,
		}); err != nil && !k8serr.IsNotFound(err) {
			return fmt.Errorf("deleting PlatformOperator %q: %w", po.Name, err)
		}
	}

	return nil
}

// advanceRunlevel checks whether all enabled modules at the current runlevel
// have reported the expected distribution version. If so, advances to the
// next runlevel that has enabled modules.
func (a *platformActions) advanceRunlevel(ctx context.Context, rr *types.ReconciliationRequest) error {
	obj, ok := rr.Instance.(*configApi.Platform)
	if !ok {
		return fmt.Errorf("instance is not a Platform")
	}

	complete, err := a.runlevelComplete(ctx, rr, obj, obj.Status.Runlevel)
	if err != nil || !complete {
		return err
	}

	level := obj.Status.Runlevel
	for {
		next, hasNext := a.registry.NextRunlevel(level)
		if !hasNext {
			break
		}

		if len(a.enabledModulesAtRunlevel(obj, next)) > 0 {
			obj.Status.Runlevel = next
			return nil
		}

		level = next
	}

	return nil
}

func (a *platformActions) runlevelComplete(
	ctx context.Context,
	rr *types.ReconciliationRequest,
	obj *configApi.Platform,
	level int,
) (bool, error) {
	upgradeInProgress := obj.Status.Distribution.Version != "" &&
		obj.Status.Distribution.Version != a.cfg.Distribution.Version

	for _, m := range a.enabledModulesAtRunlevel(obj, level) {
		po := &configApi.PlatformOperator{}
		err := rr.Client.Get(ctx, client.ObjectKey{Name: m.EffectiveName()}, po)

		switch {
		case k8serr.IsNotFound(err):
			return false, nil
		case err != nil:
			return false, fmt.Errorf("getting PlatformOperator %q: %w", m.EffectiveName(), err)
		case upgradeInProgress && po.Status.Distribution.Version != a.cfg.Distribution.Version:
			return false, nil
		}
	}

	return true, nil
}

func (a *platformActions) enabledModulesAtRunlevel(
	obj *configApi.Platform,
	level int,
) []*module.Module {
	enabled := sets.New(obj.Spec.Modules...)
	modules := make([]*module.Module, 0)

	for _, m := range a.registry.ModulesAtRunlevel(level) {
		if enabled.Has(m.EffectiveName()) {
			modules = append(modules, m)
		}
	}

	return modules
}

// aggregateStatus populates Platform status from PlatformOperator statuses.
// Modules are sorted by runlevel then name for stable ordering.
func (a *platformActions) aggregateStatus(ctx context.Context, rr *types.ReconciliationRequest) error {
	obj, ok := rr.Instance.(*configApi.Platform)
	if !ok {
		return fmt.Errorf("instance is not a Platform")
	}

	obj.Status.Distribution.Name = a.cfg.Distribution.Name

	obj.Status.Modules = nil
	allUpToDate := true

	for _, name := range obj.Spec.Modules {
		m := a.registry.ModuleByName(name)
		if m == nil {
			continue
		}

		summary := configApi.ModuleStatusSummary{
			Name:     m.EffectiveName(),
			Runlevel: m.Runlevel,
		}

		po := &configApi.PlatformOperator{}
		if err := rr.Client.Get(ctx, client.ObjectKey{Name: m.EffectiveName()}, po); err == nil {
			summary.Version = po.Status.Distribution.Version
		}

		if summary.Version != a.cfg.Distribution.Version {
			allUpToDate = false
		}

		obj.Status.Modules = append(obj.Status.Modules, summary)
	}

	if allUpToDate {
		obj.Status.Distribution.Version = a.cfg.Distribution.Version
	}

	slices.SortFunc(obj.Status.Modules, func(a, b configApi.ModuleStatusSummary) int {
		if c := cmp.Compare(a.Runlevel, b.Runlevel); c != 0 {
			return c
		}
		return cmp.Compare(a.Name, b.Name)
	})

	return nil
}
