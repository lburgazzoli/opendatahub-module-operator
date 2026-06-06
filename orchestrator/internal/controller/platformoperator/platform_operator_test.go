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

package platformoperator_test

import (
	"testing"

	"sigs.k8s.io/controller-runtime/pkg/event"

	"github.com/lburgazzoli/opendatahub-module-operator/orchestrator/pkg/module"
	"github.com/lburgazzoli/opendatahub-module-operator/orchestrator/pkg/resources/gvk"
	. "github.com/onsi/gomega"
)

type mockOrchestration struct {
	shouldReconcile bool
	modules         map[string]*module.Module
}

func (m *mockOrchestration) ShouldReconcileModule(_ *module.Module) bool {
	return m.shouldReconcile
}

func (m *mockOrchestration) ModuleByName(name string) *module.Module {
	if m.modules == nil {
		return nil
	}
	return m.modules[name]
}

func (m *mockOrchestration) StateChanges() <-chan event.GenericEvent {
	return make(chan event.GenericEvent)
}

func TestCheckRunlevelBlocked(t *testing.T) {
	g := NewWithT(t)

	o := &mockOrchestration{shouldReconcile: false}

	m := &module.Module{
		GVK:      gvk.Ray,
		Runlevel: 2,
	}

	g.Expect(o.ShouldReconcileModule(m)).To(BeFalse())
}

func TestCheckRunlevelAllowed(t *testing.T) {
	g := NewWithT(t)

	o := &mockOrchestration{shouldReconcile: true}

	m := &module.Module{
		GVK:      gvk.Ray,
		Runlevel: 2,
	}

	g.Expect(o.ShouldReconcileModule(m)).To(BeTrue())
}
