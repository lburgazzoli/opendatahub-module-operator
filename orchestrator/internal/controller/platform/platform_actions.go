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
	"fmt"

	k8serr "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	configApi "github.com/lburgazzoli/opendatahub-module-operator/orchestrator/api/config/v1alpha1"
	"github.com/opendatahub-io/operator-actions-framework/controller/types"
)

// initialize hydrates in-memory state from Platform CR status.
func (o *Orchestrator) initialize(_ context.Context, rr *types.ReconciliationRequest) error {
	obj, ok := rr.Instance.(*configApi.Platform)
	if !ok {
		return fmt.Errorf("instance is not a Platform")
	}

	mode := obj.Status.Mode
	runlevel := 0

	versionMismatch := obj.Status.Version != "" && obj.Status.Version != o.cfg.PlatformVersion

	switch {
	case mode == configApi.ModeUpgrade:
		if obj.Status.CurrentRunlevel != nil {
			runlevel = *obj.Status.CurrentRunlevel
		} else {
			runlevel = o.FirstRunlevel()
		}
	case versionMismatch:
		mode = configApi.ModeUpgrade
		runlevel = o.FirstRunlevel()
	default:
		mode = configApi.ModeReconcile
	}

	o.SetStateFor(obj, configApi.OperationalState{Mode: mode, Runlevel: runlevel})

	return nil
}

// checkAdminAcks is a preflight gate — checks ALL modules across ALL runlevels.
func (o *Orchestrator) checkAdminAcks(_ context.Context, _ *types.ReconciliationRequest) error {
	// TODO: iterate o.modules, check AdminAcks against platform-admin-acks ConfigMap
	return nil
}

// ensureModules builds PlatformOperator resources from spec.modules into
// rr.Resources. The deploy action creates/updates them via SSA (setting
// ownerRef and tracking annotations); the GC action deletes PlatformOperator
// CRs for modules removed from spec.
func (o *Orchestrator) ensureModules(_ context.Context, rr *types.ReconciliationRequest) error {
	obj, ok := rr.Instance.(*configApi.Platform)
	if !ok {
		return fmt.Errorf("instance is not a Platform")
	}

	for _, name := range obj.Spec.Modules {
		m := o.ModuleByName(name)
		if m == nil {
			return fmt.Errorf("module %q not registered", name)
		}

		po := configApi.PlatformOperator{}
		po.SetName(m.EffectiveName())

		err := rr.AddResources(&po)
		if err != nil {
			return fmt.Errorf("%w", err)
		}
	}

	rr.Generated = true

	return nil
}

// checkAdvancement is a no-op placeholder for future preflight checks
// before runlevel advancement.
func (o *Orchestrator) checkAdvancement(_ context.Context, _ *types.ReconciliationRequest) error {
	return nil
}

// advanceOrSwitch checks whether all enabled modules at the current runlevel
// have reported the expected platform version. If so, it advances to the next
// runlevel that has enabled modules, or switches to reconcile mode when done.
func (o *Orchestrator) advanceOrSwitch(ctx context.Context, rr *types.ReconciliationRequest) error {
	obj, ok := rr.Instance.(*configApi.Platform)
	if !ok {
		return fmt.Errorf("instance is not a Platform")
	}

	state := o.State()
	if state.Mode != configApi.ModeUpgrade {
		return nil
	}

	complete, err := o.runlevelComplete(ctx, rr, state.Runlevel)
	if err != nil {
		return err
	}

	if !complete {
		return nil
	}

	level := state.Runlevel
	for {
		next, hasNext := o.NextRunlevel(level)
		if !hasNext {
			o.SetStateFor(obj, configApi.OperationalState{Mode: configApi.ModeReconcile})
			return nil
		}

		if o.hasEnabledModules(ctx, rr, next) {
			o.SetStateFor(obj, configApi.OperationalState{Mode: configApi.ModeUpgrade, Runlevel: next})
			return nil
		}

		level = next
	}
}

// runlevelComplete returns true if all enabled modules at the given runlevel
// have reported the expected platform version. Returns false if any module's
// PlatformOperator CR is missing or hasn't reported the expected version.
func (o *Orchestrator) runlevelComplete(ctx context.Context, rr *types.ReconciliationRequest, level int) (bool, error) {
	for _, m := range o.ModulesAtRunlevel(level) {
		po := &configApi.PlatformOperator{}
		err := rr.Client.Get(ctx, client.ObjectKey{Name: m.EffectiveName()}, po)

		switch {
		case k8serr.IsNotFound(err):
			return false, nil
		case err != nil:
			return false, fmt.Errorf("getting PlatformOperator %q: %w", m.EffectiveName(), err)
		case po.Status.DeployedVersion != o.cfg.PlatformVersion:
			return false, nil
		}
	}

	return true, nil
}

// hasEnabledModules returns true if any module at the given runlevel has a
// PlatformOperator CR (is enabled in spec.modules).
func (o *Orchestrator) hasEnabledModules(ctx context.Context, rr *types.ReconciliationRequest, level int) bool {
	for _, m := range o.ModulesAtRunlevel(level) {
		po := &configApi.PlatformOperator{}
		if err := rr.Client.Get(ctx, client.ObjectKey{Name: m.EffectiveName()}, po); err == nil {
			return true
		}
	}

	return false
}

// aggregateStatus rolls up PlatformOperator statuses into Platform status.
func (o *Orchestrator) aggregateStatus(ctx context.Context, rr *types.ReconciliationRequest) error {
	obj, ok := rr.Instance.(*configApi.Platform)
	if !ok {
		return fmt.Errorf("instance is not a Platform")
	}

	state := o.State()

	obj.Status.Mode = state.Mode
	obj.Status.CurrentRunlevel = nil

	switch state.Mode {
	case configApi.ModeUpgrade:
		obj.Status.CurrentRunlevel = &state.Runlevel
	case configApi.ModeReconcile:
		obj.Status.Version = o.cfg.PlatformVersion
	}

	obj.Status.Modules = nil

	for _, name := range obj.Spec.Modules {
		m := o.ModuleByName(name)
		if m == nil {
			continue
		}

		summary := configApi.ModuleStatusSummary{
			Name:     m.EffectiveName(),
			Runlevel: m.Runlevel,
		}

		po := &configApi.PlatformOperator{}
		if err := rr.Client.Get(ctx, client.ObjectKey{Name: m.EffectiveName()}, po); err == nil {
			summary.Version = po.Status.DeployedVersion
		}

		obj.Status.Modules = append(obj.Status.Modules, summary)
	}

	return nil
}
