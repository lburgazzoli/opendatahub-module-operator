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

package platform_test

import (
	"testing"

	. "github.com/onsi/gomega"

	configApi "github.com/lburgazzoli/opendatahub-module-operator/orchestrator/api/config/v1alpha1"
	"github.com/lburgazzoli/opendatahub-module-operator/orchestrator/internal/controller/platform"
	"github.com/lburgazzoli/opendatahub-module-operator/orchestrator/pkg/config"
	"github.com/lburgazzoli/opendatahub-module-operator/orchestrator/pkg/module"
	"github.com/lburgazzoli/opendatahub-module-operator/orchestrator/pkg/resources/gvk"
)

func newTestOrchestrator() *platform.Orchestrator {
	cfg, _ := config.LoadFromFS(nil)
	return platform.NewOrchestrator(cfg)
}

func TestOrchestratorRegisterAndLookup(t *testing.T) {
	g := NewWithT(t)

	o := newTestOrchestrator()

	o.Register(&module.Module{GVK: gvk.Ray, Namespace: "odh-ray", Runlevel: 2})
	o.Register(&module.Module{GVK: gvk.Spark, Namespace: "odh-spark", Runlevel: 2})

	g.Expect(o.Modules()).To(HaveLen(2))
	g.Expect(o.ModuleByName("ray")).ToNot(BeNil())
	g.Expect(o.ModuleByName("spark")).ToNot(BeNil())
	g.Expect(o.ModuleByName("nonexistent")).To(BeNil())
	g.Expect(o.ModuleByGVK(gvk.Ray)).ToNot(BeNil())
	g.Expect(o.ModuleByGVK(gvk.Feast)).To(BeNil())
}

func TestOrchestratorCacheNamespaces(t *testing.T) {
	g := NewWithT(t)

	o := newTestOrchestrator()

	o.Register(&module.Module{GVK: gvk.Ray, Namespace: "odh-ray", Runlevel: 2})
	o.Register(&module.Module{GVK: gvk.Spark, Namespace: "odh-spark", Runlevel: 2})

	ns := o.CacheNamespaces()
	g.Expect(ns).To(ConsistOf("odh-ray", "odh-spark"))
	g.Expect(ns).To(Equal([]string{"odh-ray", "odh-spark"}))
}

func TestOrchestratorCacheNamespacesDeduplication(t *testing.T) {
	g := NewWithT(t)

	o := newTestOrchestrator()

	o.Register(&module.Module{GVK: gvk.Ray, Namespace: "odh-shared", Runlevel: 2})
	o.Register(&module.Module{GVK: gvk.Spark, Namespace: "odh-shared", Runlevel: 2})

	g.Expect(o.CacheNamespaces()).To(HaveLen(1))
}

func TestOrchestratorRunlevelGrouping(t *testing.T) {
	g := NewWithT(t)

	o := newTestOrchestrator()

	o.Register(&module.Module{GVK: gvk.Feast, Namespace: "odh-feast", Runlevel: 0})
	o.Register(&module.Module{GVK: gvk.Ray, Namespace: "odh-ray", Runlevel: 2})
	o.Register(&module.Module{GVK: gvk.Spark, Namespace: "odh-spark", Runlevel: 2})

	o.ComputeRunlevels()

	levels := o.Runlevels()
	g.Expect(levels).To(HaveLen(2))
	g.Expect(levels[0]).To(HaveLen(1))
	g.Expect(levels[0][0].GVK.Kind).To(Equal("Feast"))
	g.Expect(levels[1]).To(HaveLen(2))
}

func TestOrchestratorStateInitialization(t *testing.T) {
	t.Run("unknown mode by default", func(t *testing.T) {
		g := NewWithT(t)

		o := newTestOrchestrator()

		g.Expect(o.State().Mode).To(Equal(configApi.ModeUnknown))
	})

	t.Run("mode set after SetState", func(t *testing.T) {
		g := NewWithT(t)

		o := newTestOrchestrator()
		o.SetState(configApi.OperationalState{Mode: configApi.ModeReconcile})

		state := o.State()
		g.Expect(state.Mode).To(Equal(configApi.ModeReconcile))
		g.Expect(state.Runlevel).To(Equal(0))
	})
}

func TestOrchestratorShouldReconcileModule(t *testing.T) {
	t.Run("blocks when mode is unknown", func(t *testing.T) {
		g := NewWithT(t)

		o := newTestOrchestrator()
		m := &module.Module{GVK: gvk.Ray, Runlevel: 2}

		g.Expect(o.ShouldReconcileModule(m)).To(BeFalse())
	})

	t.Run("allows all in reconcile mode", func(t *testing.T) {
		g := NewWithT(t)

		o := newTestOrchestrator()
		o.SetState(configApi.OperationalState{Mode: configApi.ModeReconcile})

		m := &module.Module{GVK: gvk.Ray, Runlevel: 99}
		g.Expect(o.ShouldReconcileModule(m)).To(BeTrue())
	})

	t.Run("allows module at current runlevel in upgrade mode", func(t *testing.T) {
		g := NewWithT(t)

		o := newTestOrchestrator()
		o.SetState(configApi.OperationalState{Mode: configApi.ModeUpgrade, Runlevel: 2})

		m := &module.Module{GVK: gvk.Ray, Runlevel: 2}
		g.Expect(o.ShouldReconcileModule(m)).To(BeTrue())
	})

	t.Run("allows module below current runlevel in upgrade mode", func(t *testing.T) {
		g := NewWithT(t)

		o := newTestOrchestrator()
		o.SetState(configApi.OperationalState{Mode: configApi.ModeUpgrade, Runlevel: 2})

		m := &module.Module{GVK: gvk.Feast, Runlevel: 0}
		g.Expect(o.ShouldReconcileModule(m)).To(BeTrue())
	})

	t.Run("blocks module above current runlevel in upgrade mode", func(t *testing.T) {
		g := NewWithT(t)

		o := newTestOrchestrator()
		o.SetState(configApi.OperationalState{Mode: configApi.ModeUpgrade})

		m := &module.Module{GVK: gvk.Ray, Runlevel: 2}
		g.Expect(o.ShouldReconcileModule(m)).To(BeFalse())
	})
}
