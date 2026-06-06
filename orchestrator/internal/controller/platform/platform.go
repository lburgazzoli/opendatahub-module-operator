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
	"maps"
	"slices"
	"sort"
	"sync"

	"k8s.io/apimachinery/pkg/runtime/schema"
	k8stypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/event"

	configApi "github.com/lburgazzoli/opendatahub-module-operator/orchestrator/api/config/v1alpha1"
	orchestratorconfig "github.com/lburgazzoli/opendatahub-module-operator/orchestrator/pkg/config"
	"github.com/lburgazzoli/opendatahub-module-operator/orchestrator/pkg/module"
)

// Orchestrator holds process-lifetime state shared between controllers.
type Orchestrator struct {
	cfg       *orchestratorconfig.Config
	recorder  record.EventRecorder
	modules   []*module.Module
	runlevels [][]*module.Module

	mu          sync.RWMutex
	state       configApi.OperationalState
	platformUID k8stypes.UID
	notify      chan event.GenericEvent
}

// NewOrchestrator creates an Orchestrator.
func NewOrchestrator(cfg *orchestratorconfig.Config) *Orchestrator {
	return &Orchestrator{
		cfg:    cfg,
		notify: make(chan event.GenericEvent, 1),
	}
}

// SetRecorder sets the event recorder for state transition events.
func (o *Orchestrator) SetRecorder(recorder record.EventRecorder) {
	o.recorder = recorder
}

// StateChanges returns a channel that receives an event whenever the
// orchestration state transitions (mode or runlevel change). The
// PlatformOperator controller watches this channel to re-reconcile
// gated modules.
func (o *Orchestrator) StateChanges() <-chan event.GenericEvent {
	return o.notify
}

// Register adds a module to the orchestrator.
func (o *Orchestrator) Register(m *module.Module) {
	o.modules = append(o.modules, m)
}

// CacheNamespaces returns deduplicated, sorted namespaces from all registered modules.
func (o *Orchestrator) CacheNamespaces() []string {
	result := sets.New[string]()
	for _, m := range o.modules {
		result.Insert(m.Namespace)
	}
	sorted := result.UnsortedList()
	sort.Strings(sorted)
	return sorted
}

// Modules returns the registered modules.
func (o *Orchestrator) Modules() []*module.Module {
	return o.modules
}

var (
	_ module.Registry      = (*Orchestrator)(nil)
	_ module.Orchestration = (*Orchestrator)(nil)
)

// Config returns the orchestrator config.
func (o *Orchestrator) Config() *orchestratorconfig.Config {
	return o.cfg
}

// Namespace returns the orchestrator namespace (implements module.Registry).
func (o *Orchestrator) Namespace() string {
	return o.cfg.Namespace()
}

// ChartsPath returns the charts base path (implements module.Registry).
func (o *Orchestrator) ChartsPath() string {
	return o.cfg.ChartsPath
}

// ModuleByName returns the module with the given effective name, or nil.
func (o *Orchestrator) ModuleByName(name string) *module.Module {
	for _, m := range o.modules {
		if m.EffectiveName() == name {
			return m
		}
	}
	return nil
}

// ModuleByGVK returns the module with the given GVK, or nil.
func (o *Orchestrator) ModuleByGVK(g schema.GroupVersionKind) *module.Module {
	for _, m := range o.modules {
		if m.GVK == g {
			return m
		}
	}
	return nil
}

// SetStateFor updates the in-memory orchestration state and emits events on transitions.
// If the Platform CR UID changed (delete + recreate), the state is recomputed
// from the new CR's status rather than carried over from the old instance.
func (o *Orchestrator) SetStateFor(obj *configApi.Platform, state configApi.OperationalState) {
	o.mu.Lock()
	prev := o.state
	prevUID := o.platformUID

	uid := obj.GetUID()
	uidChanged := prevUID != "" && prevUID != uid
	o.platformUID = uid

	if uidChanged {
		prev = configApi.OperationalState{}
	}

	o.state = state
	o.mu.Unlock()

	changed := prev.Mode != state.Mode || prev.Runlevel != state.Runlevel

	if o.recorder != nil {
		if uidChanged {
			o.recorder.Eventf(obj, "Normal", "PlatformReset",
				"Platform CR replaced (UID %s → %s), state reset", prevUID, uid)
		}

		if prev.Mode != state.Mode {
			o.recorder.Eventf(obj, "Normal", "ModeTransition",
				"Mode changed from %q to %q", prev.Mode, state.Mode)
		}

		if prev.Runlevel != state.Runlevel && state.Mode == configApi.ModeUpgrade {
			o.recorder.Eventf(obj, "Normal", "RunlevelAdvance",
				"Runlevel advanced from %d to %d", prev.Runlevel, state.Runlevel)
		}
	}

	if changed || uidChanged {
		select {
		case o.notify <- event.GenericEvent{}:
		default:
		}
	}
}

// SetState updates the in-memory orchestration state without emitting events.
func (o *Orchestrator) SetState(state configApi.OperationalState) {
	o.mu.Lock()
	defer o.mu.Unlock()

	o.state = state
}

// State returns the current orchestration state.
// Mode is ModeUnknown ("") if the Platform controller hasn't run yet.
func (o *Orchestrator) State() configApi.OperationalState {
	o.mu.RLock()
	defer o.mu.RUnlock()

	return o.state
}

// ShouldReconcileModule returns true if the given module is allowed to proceed.
func (o *Orchestrator) ShouldReconcileModule(m *module.Module) bool {
	o.mu.RLock()
	defer o.mu.RUnlock()

	switch o.state.Mode {
	case configApi.ModeUnknown:
		return false
	case configApi.ModeReconcile:
		return true
	default:
		return m.Runlevel <= o.state.Runlevel
	}
}

// Runlevels returns the computed runlevel groups.
func (o *Orchestrator) Runlevels() [][]*module.Module {
	return o.runlevels
}

// ModulesAtRunlevel returns the modules at the given runlevel, or nil.
func (o *Orchestrator) ModulesAtRunlevel(level int) []*module.Module {
	for _, group := range o.runlevels {
		if len(group) > 0 && group[0].Runlevel == level {
			return group
		}
	}
	return nil
}

// FirstRunlevel returns the lowest runlevel, or 0 if no modules are registered.
func (o *Orchestrator) FirstRunlevel() int {
	if len(o.runlevels) == 0 || len(o.runlevels[0]) == 0 {
		return 0
	}
	return o.runlevels[0][0].Runlevel
}

// NextRunlevel returns the runlevel after current, and whether one exists.
func (o *Orchestrator) NextRunlevel(current int) (int, bool) {
	found := false
	for _, group := range o.runlevels {
		if len(group) == 0 {
			continue
		}
		if found {
			return group[0].Runlevel, true
		}
		if group[0].Runlevel == current {
			found = true
		}
	}
	return 0, false
}

// ComputeRunlevels groups registered modules by runlevel.
// Must be called after all modules are registered and before the controller starts.
func (o *Orchestrator) ComputeRunlevels() {
	if len(o.modules) == 0 {
		return
	}

	byLevel := make(map[int][]*module.Module)
	for _, m := range o.modules {
		byLevel[m.Runlevel] = append(byLevel[m.Runlevel], m)
	}

	levels := slices.Sorted(maps.Keys(byLevel))

	o.runlevels = make([][]*module.Module, len(levels))
	for i, lvl := range levels {
		o.runlevels[i] = byLevel[lvl]
	}
}
