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

package module

import (
	"fmt"
	"maps"
	"slices"
	"sort"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/sets"
)

// ModuleRegistry holds registered modules and their computed runlevel groups.
type ModuleRegistry struct {
	namespace  string
	chartsPath string
	modules    []*Module
	runlevels  [][]*Module
}

// NewModuleRegistry creates a ModuleRegistry.
func NewModuleRegistry(namespace string, chartsPath string) *ModuleRegistry {
	return &ModuleRegistry{
		namespace:  namespace,
		chartsPath: chartsPath,
	}
}

// Register adds a module.
func (r *ModuleRegistry) Register(m *Module) {
	if existing := r.ModuleByGVK(m.GVK); existing != nil {
		panic(fmt.Sprintf(
			"module %q duplicates registered GVK %s already used by module %q",
			m.EffectiveName(),
			m.GVK.String(),
			existing.EffectiveName(),
		))
	}

	r.modules = append(r.modules, m)
}

// Namespace implements Registry.
func (r *ModuleRegistry) Namespace() string {
	return r.namespace
}

// ChartsPath implements Registry.
func (r *ModuleRegistry) ChartsPath() string {
	return r.chartsPath
}

// Modules returns all registered modules.
func (r *ModuleRegistry) Modules() []*Module {
	return r.modules
}

// ModuleByName returns the module with the given effective name, or nil.
func (r *ModuleRegistry) ModuleByName(name string) *Module {
	for _, m := range r.modules {
		if m.EffectiveName() == name {
			return m
		}
	}
	return nil
}

// ModuleByGVK returns the module with the given GVK, or nil.
func (r *ModuleRegistry) ModuleByGVK(g schema.GroupVersionKind) *Module {
	for _, m := range r.modules {
		if m.GVK == g {
			return m
		}
	}
	return nil
}

// CacheNamespaces returns deduplicated, sorted namespaces from all registered modules.
func (r *ModuleRegistry) CacheNamespaces() []string {
	result := sets.New[string]()
	for _, m := range r.modules {
		result.Insert(m.Namespace)
	}
	sorted := result.UnsortedList()
	sort.Strings(sorted)
	return sorted
}

// ComputeRunlevels groups registered modules by runlevel.
// Must be called after all modules are registered and before controllers start.
func (r *ModuleRegistry) ComputeRunlevels() {
	if len(r.modules) == 0 {
		return
	}

	byLevel := make(map[int][]*Module)
	for _, m := range r.modules {
		byLevel[m.Runlevel] = append(byLevel[m.Runlevel], m)
	}

	levels := slices.Sorted(maps.Keys(byLevel))

	r.runlevels = make([][]*Module, len(levels))
	for i, lvl := range levels {
		r.runlevels[i] = byLevel[lvl]
	}
}

// Runlevels returns the computed runlevel groups.
func (r *ModuleRegistry) Runlevels() [][]*Module {
	return r.runlevels
}

// ModulesAtRunlevel returns the modules at the given runlevel, or nil.
func (r *ModuleRegistry) ModulesAtRunlevel(level int) []*Module {
	for _, group := range r.runlevels {
		if len(group) > 0 && group[0].Runlevel == level {
			return group
		}
	}
	return nil
}

// FirstRunlevel returns the lowest runlevel, or 0 if no modules are registered.
func (r *ModuleRegistry) FirstRunlevel() int {
	if len(r.runlevels) == 0 || len(r.runlevels[0]) == 0 {
		return 0
	}
	return r.runlevels[0][0].Runlevel
}

// NextRunlevel returns the runlevel after current, and whether one exists.
func (r *ModuleRegistry) NextRunlevel(current int) (int, bool) {
	found := false
	for _, group := range r.runlevels {
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

var _ Registry = (*ModuleRegistry)(nil)
