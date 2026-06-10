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

// Registry holds the immutable module set and computed runlevel groups.
type Registry struct {
	modules   []*Module
	runlevels [][]*Module
}

// NewRegistry validates the full module list and precomputes runlevels.
func NewRegistry(modules []*Module) (*Registry, error) {
	runlevels, err := computeRunlevels(modules)
	if err != nil {
		return nil, err
	}

	return &Registry{
		modules:   slices.Clone(modules),
		runlevels: runlevels,
	}, nil
}

// Modules returns all registered modules.
func (r *Registry) Modules() []*Module {
	return slices.Clone(r.modules)
}

// ModuleByName returns the module with the given name, or nil.
func (r *Registry) ModuleByName(name string) *Module {
	for _, m := range r.modules {
		if m.Name == name {
			return m
		}
	}
	return nil
}

// ModuleByGVK returns the module with the given GVK, or nil.
func (r *Registry) ModuleByGVK(g schema.GroupVersionKind) *Module {
	for _, m := range r.modules {
		if m.GVK == g {
			return m
		}
	}
	return nil
}

// CacheNamespaces returns deduplicated, sorted namespaces from all registered modules.
func (r *Registry) CacheNamespaces() []string {
	result := sets.New[string]()
	for _, m := range r.modules {
		result.Insert(m.Namespace)
	}
	sorted := result.UnsortedList()
	sort.Strings(sorted)
	return sorted
}

// Runlevels returns the computed runlevel groups.
func (r *Registry) Runlevels() [][]*Module {
	result := make([][]*Module, len(r.runlevels))
	for i, group := range r.runlevels {
		result[i] = slices.Clone(group)
	}
	return result
}

// ModulesAtRunlevel returns the modules at the given runlevel, or nil.
func (r *Registry) ModulesAtRunlevel(level int) []*Module {
	for _, group := range r.runlevels {
		if len(group) > 0 && group[0].Runlevel == level {
			return group
		}
	}
	return nil
}

// FirstRunlevel returns the lowest runlevel, or 0 if no modules are registered.
func (r *Registry) FirstRunlevel() int {
	if len(r.runlevels) == 0 || len(r.runlevels[0]) == 0 {
		return 0
	}
	return r.runlevels[0][0].Runlevel
}

// NextRunlevel returns the runlevel after current, and whether one exists.
func (r *Registry) NextRunlevel(current int) (int, bool) {
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

func computeRunlevels(modules []*Module) ([][]*Module, error) {
	if err := validateModules(modules); err != nil {
		return nil, err
	}
	if len(modules) == 0 {
		return nil, nil
	}

	byLevel := make(map[int][]*Module)
	for _, m := range modules {
		byLevel[m.Runlevel] = append(byLevel[m.Runlevel], m)
	}

	levels := slices.Sorted(maps.Keys(byLevel))
	runlevels := make([][]*Module, len(levels))
	for i, lvl := range levels {
		runlevels[i] = slices.Clone(byLevel[lvl])
	}

	return runlevels, nil
}

func validateModules(modules []*Module) error {
	byName := make(map[string]*Module, len(modules))
	byGVK := make(map[schema.GroupVersionKind]*Module, len(modules))

	for i, m := range modules {
		if m == nil {
			return fmt.Errorf("module at index %d is nil", i)
		}
		if m.Name == "" {
			return fmt.Errorf("module name must be set for GVK %s", m.GVK.String())
		}
		if m.Manifests.Chart.Object == nil {
			return fmt.Errorf("module %q must be created with NewModule", m.Name)
		}
		if existing := byName[m.Name]; existing != nil {
			return fmt.Errorf(
				"module %q duplicates registered name already used by GVK %s",
				m.Name,
				existing.GVK.String(),
			)
		}
		if existing := byGVK[m.GVK]; existing != nil {
			return fmt.Errorf(
				"module %q duplicates registered GVK %s already used by module %q",
				m.Name,
				m.GVK.String(),
				existing.Name,
			)
		}

		byName[m.Name] = m
		byGVK[m.GVK] = m
	}

	return nil
}
