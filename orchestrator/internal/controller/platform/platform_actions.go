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

	if mode == "" {
		if obj.Status.Version != "" && obj.Status.Version != o.cfg.PlatformVersion {
			mode = configApi.ModeUpgrade
		} else {
			mode = configApi.ModeReconcile
		}
	}

	if obj.Status.CurrentRunlevel != nil {
		runlevel = *obj.Status.CurrentRunlevel
	}

	o.SetStateFor(obj, configApi.OperationalState{Mode: mode, Runlevel: runlevel})

	return nil
}

// checkAdminAcks is a preflight gate — checks ALL modules across ALL runlevels.
func (o *Orchestrator) checkAdminAcks(_ context.Context, _ *types.ReconciliationRequest) error {
	// TODO: iterate o.modules, check AdminAcks against platform-admin-acks ConfigMap
	return nil
}

// ensureModules diffs spec.modules against existing PlatformOperator CRs.
// Creates missing PlatformOperator CRs and spawns their ReconcilerFor.
// Deletes PlatformOperator CRs for removed modules.
func (o *Orchestrator) ensureModules(_ context.Context, rr *types.ReconciliationRequest) error {
	obj, ok := rr.Instance.(*configApi.Platform)
	if !ok {
		return fmt.Errorf("instance is not a Platform")
	}

	_ = obj.Spec.Modules

	// TODO:
	// 1. List existing PlatformOperator CRs
	// 2. For each name in spec.modules:
	//    - Look up Module from registry (o.ModuleByName)
	//    - If PlatformOperator CR missing: create it + spawn ReconcilerFor
	// 3. For PlatformOperator CRs not in spec.modules: delete them

	return nil
}

// checkAdvancement reads PlatformOperator statuses at the current runlevel.
func (o *Orchestrator) checkAdvancement(_ context.Context, _ *types.ReconciliationRequest) error {
	// TODO: for each PlatformOperator at current runlevel:
	// - Check status.deployedVersion == cfg.PlatformVersion
	// - Module CR not present → Deployed phase, doesn't block
	// - Module CR present but not Ready → NotReady, blocks
	return nil
}

// advanceOrSwitch advances to the next runlevel or switches to reconcile mode.
func (o *Orchestrator) advanceOrSwitch(_ context.Context, rr *types.ReconciliationRequest) error {
	obj, ok := rr.Instance.(*configApi.Platform)
	if !ok {
		return fmt.Errorf("instance is not a Platform")
	}

	_ = obj

	// TODO: if all modules at current runlevel are done:
	// - More runlevels → advance currentRunlevel, update in-memory state, notify channel
	// - All done → mode = reconcile, clear currentRunlevel
	return nil
}

// aggregateStatus rolls up PlatformOperator statuses into Platform status.
func (o *Orchestrator) aggregateStatus(_ context.Context, rr *types.ReconciliationRequest) error {
	obj, ok := rr.Instance.(*configApi.Platform)
	if !ok {
		return fmt.Errorf("instance is not a Platform")
	}

	state := o.State()

	obj.Status.Version = o.cfg.PlatformVersion
	obj.Status.CurrentRunlevel = nil
	obj.Status.Mode = state.Mode

	if state.Mode == configApi.ModeUpgrade {
		obj.Status.CurrentRunlevel = &state.Runlevel
	}

	// TODO: build ModuleStatusSummary from PlatformOperator statuses
	return nil
}
