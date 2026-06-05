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
	"os"
	"path/filepath"
	"testing"

	configApi "github.com/lburgazzoli/opendatahub-module-operator/orchestrator/api/config/v1alpha1"
	"github.com/lburgazzoli/opendatahub-module-operator/orchestrator/internal/controller/platform"
	"github.com/lburgazzoli/opendatahub-module-operator/orchestrator/pkg/config"
	"github.com/lburgazzoli/opendatahub-module-operator/orchestrator/pkg/module"
	"github.com/lburgazzoli/opendatahub-module-operator/orchestrator/pkg/resources/gvk"
	"github.com/lburgazzoli/opendatahub-module-operator/orchestrator/test/support"
	. "github.com/onsi/gomega"
)

func testChartPath(t *testing.T) string {
	t.Helper()

	root, err := support.ProjectRoot()
	if err != nil {
		t.Fatalf("finding project root: %v", err)
	}

	p := filepath.Join(root, "modules", "opendatahub-mymodule-operator", "config", "chart")
	if _, err := os.Stat(p); os.IsNotExist(err) {
		t.Skipf("test chart not found at %s (run 'make helm' in mymodule first)", p)
	}

	return p
}

func newTestOrchestrator(t *testing.T) *platform.Orchestrator {
	t.Helper()

	cfg, err := config.LoadFromFS(nil)
	if err != nil {
		t.Fatalf("loading config: %v", err)
	}

	return platform.NewOrchestrator(cfg)
}

func TestCheckRunlevelBlocked(t *testing.T) {
	g := NewWithT(t)

	o := newTestOrchestrator(t)

	m := &module.Module{
		GVK:       gvk.Ray,
		Namespace: "odh-ray",
		ChartPath: testChartPath(t),
		Runlevel:  2,
	}

	o.Register(m)
	o.ComputeRunlevels()
	o.SetState(configApi.OperationalState{Mode: configApi.ModeUpgrade})

	g.Expect(o.ShouldReconcileModule(m)).To(BeFalse())
}

func TestCheckRunlevelAllowed(t *testing.T) {
	g := NewWithT(t)

	o := newTestOrchestrator(t)

	m := &module.Module{
		GVK:       gvk.Ray,
		Namespace: "odh-ray",
		ChartPath: testChartPath(t),
		Runlevel:  2,
	}

	o.Register(m)
	o.ComputeRunlevels()
	o.SetState(configApi.OperationalState{Mode: configApi.ModeUpgrade, Runlevel: 2})

	g.Expect(o.ShouldReconcileModule(m)).To(BeTrue())
}

func TestCheckRunlevelReconcileMode(t *testing.T) {
	g := NewWithT(t)

	o := newTestOrchestrator(t)

	m := &module.Module{
		GVK:       gvk.Ray,
		Namespace: "odh-ray",
		ChartPath: testChartPath(t),
		Runlevel:  99,
	}

	o.Register(m)
	o.ComputeRunlevels()
	o.SetState(configApi.OperationalState{Mode: configApi.ModeReconcile})

	g.Expect(o.ShouldReconcileModule(m)).To(BeTrue())
}
