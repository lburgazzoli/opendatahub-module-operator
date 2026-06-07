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

package module_test

import (
	"testing"

	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/lburgazzoli/opendatahub-module-operator/orchestrator/pkg/module"
)

var (
	rayGVK   = schema.GroupVersionKind{Group: "test.io", Version: "v1", Kind: "Ray"}
	sparkGVK = schema.GroupVersionKind{Group: "test.io", Version: "v1", Kind: "Spark"}
	feastGVK = schema.GroupVersionKind{Group: "test.io", Version: "v1", Kind: "Feast"}
)

func newTestRegistry() *module.ModuleRegistry {
	return module.NewModuleRegistry("test-ns", "/charts")
}

func TestRegistryLookup(t *testing.T) {
	g := NewWithT(t)

	r := newTestRegistry()
	r.Register(&module.Module{GVK: rayGVK, Namespace: "ns", Runlevel: 2})
	r.Register(&module.Module{GVK: sparkGVK, Namespace: "ns", Runlevel: 2})

	g.Expect(r.Modules()).To(HaveLen(2))
	g.Expect(r.ModuleByName("ray")).NotTo(BeNil())
	g.Expect(r.ModuleByName("nonexistent")).To(BeNil())
	g.Expect(r.ModuleByGVK(rayGVK)).NotTo(BeNil())
	g.Expect(r.ModuleByGVK(feastGVK)).To(BeNil())
}

func TestRegistryRegisterPanicsOnDuplicateGVK(t *testing.T) {
	g := NewWithT(t)

	r := newTestRegistry()
	r.Register(&module.Module{GVK: rayGVK, Namespace: "ns-a", Runlevel: 1})

	g.Expect(func() {
		r.Register(&module.Module{Name: "ray-copy", GVK: rayGVK, Namespace: "ns-b", Runlevel: 2})
	}).To(Panic())
}

func TestRegistryCacheNamespaces(t *testing.T) {
	t.Run("deduplicates and sorts", func(t *testing.T) {
		g := NewWithT(t)

		r := newTestRegistry()
		r.Register(&module.Module{GVK: rayGVK, Namespace: "b-ns", Runlevel: 1})
		r.Register(&module.Module{GVK: sparkGVK, Namespace: "a-ns", Runlevel: 1})

		g.Expect(r.CacheNamespaces()).To(Equal([]string{"a-ns", "b-ns"}))
	})

	t.Run("deduplicates shared namespace", func(t *testing.T) {
		g := NewWithT(t)

		r := newTestRegistry()
		r.Register(&module.Module{GVK: rayGVK, Namespace: "shared", Runlevel: 1})
		r.Register(&module.Module{GVK: sparkGVK, Namespace: "shared", Runlevel: 1})

		g.Expect(r.CacheNamespaces()).To(HaveLen(1))
	})
}

func TestRegistryRunlevels(t *testing.T) {
	t.Run("groups by runlevel", func(t *testing.T) {
		g := NewWithT(t)

		r := newTestRegistry()
		r.Register(&module.Module{GVK: feastGVK, Namespace: "ns", Runlevel: 0})
		r.Register(&module.Module{GVK: rayGVK, Namespace: "ns", Runlevel: 2})
		r.Register(&module.Module{GVK: sparkGVK, Namespace: "ns", Runlevel: 2})
		r.ComputeRunlevels()

		g.Expect(r.Runlevels()).To(HaveLen(2))
		g.Expect(r.Runlevels()[0]).To(HaveLen(1))
		g.Expect(r.Runlevels()[1]).To(HaveLen(2))
	})
}

func TestRegistryFirstRunlevel(t *testing.T) {
	t.Run("returns 0 with no modules", func(t *testing.T) {
		g := NewWithT(t)
		r := newTestRegistry()
		r.ComputeRunlevels()
		g.Expect(r.FirstRunlevel()).To(Equal(0))
	})

	t.Run("returns lowest runlevel", func(t *testing.T) {
		g := NewWithT(t)
		r := newTestRegistry()
		r.Register(&module.Module{GVK: rayGVK, Namespace: "ns", Runlevel: 3})
		r.Register(&module.Module{GVK: sparkGVK, Namespace: "ns", Runlevel: 1})
		r.ComputeRunlevels()
		g.Expect(r.FirstRunlevel()).To(Equal(1))
	})
}

func TestRegistryNextRunlevel(t *testing.T) {
	r := newTestRegistry()
	r.Register(&module.Module{GVK: feastGVK, Namespace: "ns", Runlevel: 1})
	r.Register(&module.Module{GVK: rayGVK, Namespace: "ns", Runlevel: 3})
	r.Register(&module.Module{GVK: sparkGVK, Namespace: "ns", Runlevel: 5})
	r.ComputeRunlevels()

	t.Run("advances to next group", func(t *testing.T) {
		g := NewWithT(t)
		next, ok := r.NextRunlevel(1)
		g.Expect(ok).To(BeTrue())
		g.Expect(next).To(Equal(3))
	})

	t.Run("advances past gaps", func(t *testing.T) {
		g := NewWithT(t)
		next, ok := r.NextRunlevel(3)
		g.Expect(ok).To(BeTrue())
		g.Expect(next).To(Equal(5))
	})

	t.Run("returns false at last runlevel", func(t *testing.T) {
		g := NewWithT(t)
		_, ok := r.NextRunlevel(5)
		g.Expect(ok).To(BeFalse())
	})

	t.Run("returns false for unknown runlevel", func(t *testing.T) {
		g := NewWithT(t)
		_, ok := r.NextRunlevel(99)
		g.Expect(ok).To(BeFalse())
	})
}

func TestRegistryModulesAtRunlevel(t *testing.T) {
	r := newTestRegistry()
	r.Register(&module.Module{GVK: feastGVK, Namespace: "ns", Runlevel: 1})
	r.Register(&module.Module{GVK: rayGVK, Namespace: "ns", Runlevel: 2})
	r.Register(&module.Module{GVK: sparkGVK, Namespace: "ns", Runlevel: 2})
	r.ComputeRunlevels()

	t.Run("returns modules at existing runlevel", func(t *testing.T) {
		g := NewWithT(t)
		g.Expect(r.ModulesAtRunlevel(2)).To(HaveLen(2))
	})

	t.Run("returns nil for unknown runlevel", func(t *testing.T) {
		g := NewWithT(t)
		g.Expect(r.ModulesAtRunlevel(99)).To(BeNil())
	})
}

func TestRegistryInterface(t *testing.T) {
	g := NewWithT(t)
	r := newTestRegistry()
	g.Expect(r.Namespace()).To(Equal("test-ns"))
	g.Expect(r.ChartsPath()).To(Equal("/charts"))
}
