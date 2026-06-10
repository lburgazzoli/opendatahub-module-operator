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
	"path/filepath"
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

func testChartPath() string {
	return filepath.Join("..", "..", "test", "support", "testdata", "charts", "test-module")
}

func newTestModule(name string, gvk schema.GroupVersionKind, namespace string, runlevel int) *module.Module {
	m, err := module.NewModule(module.ModuleSpec{
		Name:      name,
		GVK:       gvk,
		Namespace: namespace,
		Runlevel:  runlevel,
		ChartPath: testChartPath(),
	})
	if err != nil {
		panic(err)
	}

	return m
}

func newTestRegistry(modules ...*module.Module) *module.Registry {
	r, err := module.NewRegistry(modules)
	if err != nil {
		panic(err)
	}

	return r
}

func TestNewRegistryRejectsInvalidModules(t *testing.T) {
	t.Run("errors on nil module", func(t *testing.T) {
		g := NewWithT(t)

		r, err := module.NewRegistry([]*module.Module{nil})

		g.Expect(err).To(HaveOccurred())
		g.Expect(r).To(BeNil())
	})

	t.Run("errors on missing name", func(t *testing.T) {
		g := NewWithT(t)

		r, err := module.NewRegistry([]*module.Module{{GVK: rayGVK, Namespace: "ns-a", Runlevel: 1}})

		g.Expect(err).To(HaveOccurred())
		g.Expect(r).To(BeNil())
	})

	t.Run("errors when module bypasses constructor", func(t *testing.T) {
		g := NewWithT(t)

		r, err := module.NewRegistry([]*module.Module{{
			Name:      "ray",
			GVK:       rayGVK,
			Namespace: "ns-a",
			Runlevel:  1,
		}})

		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring("must be created with NewModule"))
		g.Expect(r).To(BeNil())
	})

	t.Run("errors on duplicate name", func(t *testing.T) {
		g := NewWithT(t)

		r, err := module.NewRegistry([]*module.Module{
			newTestModule("ray", rayGVK, "ns-a", 1),
			newTestModule("ray", sparkGVK, "ns-b", 2),
		})

		g.Expect(err).To(HaveOccurred())
		g.Expect(r).To(BeNil())
	})

	t.Run("errors on duplicate gvk", func(t *testing.T) {
		g := NewWithT(t)

		r, err := module.NewRegistry([]*module.Module{
			newTestModule("ray", rayGVK, "ns-a", 1),
			newTestModule("ray-copy", rayGVK, "ns-b", 2),
		})

		g.Expect(err).To(HaveOccurred())
		g.Expect(r).To(BeNil())
	})
}

func TestNewRegistryPreservesLoadedChartMetadata(t *testing.T) {
	g := NewWithT(t)
	mod := newTestModule("ray", rayGVK, "ns-a", 1)

	r, err := module.NewRegistry([]*module.Module{mod})

	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(r.Modules()).To(HaveLen(1))
	g.Expect(mod.Manifests.Chart.Object).NotTo(BeNil())
	g.Expect(mod.Manifests.Chart).To(Equal(module.ModuleChart{
		Path:       testChartPath(),
		Name:       "test-module",
		Version:    "0.1.0",
		AppVersion: "1.0.0",
		Object:     mod.Manifests.Chart.Object,
	}))
}

func TestRegistryLookup(t *testing.T) {
	g := NewWithT(t)
	r := newTestRegistry(
		newTestModule("ray", rayGVK, "ns", 2),
		newTestModule("spark", sparkGVK, "ns", 2),
	)

	g.Expect(r.Modules()).To(HaveLen(2))
	g.Expect(r.ModuleByName("ray")).NotTo(BeNil())
	g.Expect(r.ModuleByName("nonexistent")).To(BeNil())
	g.Expect(r.ModuleByGVK(rayGVK)).NotTo(BeNil())
	g.Expect(r.ModuleByGVK(feastGVK)).To(BeNil())
}

func TestRegistryCacheNamespaces(t *testing.T) {
	t.Run("deduplicates and sorts", func(t *testing.T) {
		g := NewWithT(t)
		r := newTestRegistry(
			newTestModule("ray", rayGVK, "b-ns", 1),
			newTestModule("spark", sparkGVK, "a-ns", 1),
		)

		g.Expect(r.CacheNamespaces()).To(Equal([]string{"a-ns", "b-ns"}))
	})

	t.Run("deduplicates shared namespace", func(t *testing.T) {
		g := NewWithT(t)
		r := newTestRegistry(
			newTestModule("ray", rayGVK, "shared", 1),
			newTestModule("spark", sparkGVK, "shared", 1),
		)

		g.Expect(r.CacheNamespaces()).To(HaveLen(1))
	})
}

func TestRegistryRunlevels(t *testing.T) {
	t.Run("groups by runlevel", func(t *testing.T) {
		g := NewWithT(t)
		r := newTestRegistry(
			newTestModule("feast", feastGVK, "ns", 0),
			newTestModule("ray", rayGVK, "ns", 2),
			newTestModule("spark", sparkGVK, "ns", 2),
		)

		g.Expect(r.Runlevels()).To(HaveLen(2))
		g.Expect(r.Runlevels()[0]).To(HaveLen(1))
		g.Expect(r.Runlevels()[1]).To(HaveLen(2))
	})

	t.Run("returns 0 first runlevel when empty", func(t *testing.T) {
		g := NewWithT(t)

		r, err := module.NewRegistry(nil)

		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(r.FirstRunlevel()).To(Equal(0))
	})

	t.Run("returns lowest runlevel", func(t *testing.T) {
		g := NewWithT(t)
		r := newTestRegistry(
			newTestModule("ray", rayGVK, "ns", 3),
			newTestModule("spark", sparkGVK, "ns", 1),
		)

		g.Expect(r.FirstRunlevel()).To(Equal(1))
	})
}

func TestRegistryNextRunlevel(t *testing.T) {
	r := newTestRegistry(
		newTestModule("feast", feastGVK, "ns", 1),
		newTestModule("ray", rayGVK, "ns", 3),
		newTestModule("spark", sparkGVK, "ns", 5),
	)

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
	r := newTestRegistry(
		newTestModule("feast", feastGVK, "ns", 1),
		newTestModule("ray", rayGVK, "ns", 2),
		newTestModule("spark", sparkGVK, "ns", 2),
	)

	t.Run("returns modules at existing runlevel", func(t *testing.T) {
		g := NewWithT(t)
		g.Expect(r.ModulesAtRunlevel(2)).To(HaveLen(2))
	})

	t.Run("returns nil for unknown runlevel", func(t *testing.T) {
		g := NewWithT(t)
		g.Expect(r.ModulesAtRunlevel(99)).To(BeNil())
	})
}
