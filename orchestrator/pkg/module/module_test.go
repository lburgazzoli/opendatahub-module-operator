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
	"context"
	"testing"

	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/lburgazzoli/opendatahub-module-operator/orchestrator/pkg/module"
)

var testGVK = schema.GroupVersionKind{
	Group:   "components.platform.opendatahub.io",
	Version: "v1alpha1",
	Kind:    "Ray",
}

func TestModuleDefaults(t *testing.T) {
	g := NewWithT(t)

	m, err := module.NewModule(module.ModuleSpec{
		Name:      "ray",
		GVK:       testGVK,
		ChartPath: testChartPath(),
	})

	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(m.AdminAcks).To(BeNil())
	g.Expect(m.Values).To(BeNil())
}

func TestNewModule(t *testing.T) {
	t.Run("errors when chart path is not set", func(t *testing.T) {
		g := NewWithT(t)

		m, err := module.NewModule(module.ModuleSpec{
			Name: "ray",
			GVK:  testGVK,
		})

		g.Expect(err).To(HaveOccurred())
		g.Expect(err).To(MatchError(ContainSubstring("chart path")))
		g.Expect(m).To(BeNil())
	})

	t.Run("loads chart metadata eagerly", func(t *testing.T) {
		g := NewWithT(t)

		m, err := module.NewModule(module.ModuleSpec{
			Name:      "ray",
			GVK:       testGVK,
			ChartPath: testChartPath(),
		})

		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(m.Manifests.Chart.Object).NotTo(BeNil())
		g.Expect(m.Manifests.Chart).To(Equal(module.ModuleChart{
			Path:       testChartPath(),
			Name:       "test-module",
			Version:    "0.1.0",
			AppVersion: "1.0.0",
			Object:     m.Manifests.Chart.Object,
		}))
	})
}

func TestModuleConfig(t *testing.T) {
	t.Run("nil config by default", func(t *testing.T) {
		g := NewWithT(t)

		m, err := module.NewModule(module.ModuleSpec{
			Name:      "ray",
			GVK:       testGVK,
			ChartPath: testChartPath(),
		})
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(m.Config).To(BeNil())
	})

	t.Run("config function returns values", func(t *testing.T) {
		g := NewWithT(t)

		m, err := module.NewModule(module.ModuleSpec{
			Name:      "ray",
			GVK:       testGVK,
			ChartPath: testChartPath(),
			Config: func(_ context.Context, _ client.Client) (map[string]any, error) {
				return map[string]any{"key": "val"}, nil
			},
		})

		g.Expect(err).NotTo(HaveOccurred())
		vals, err := m.Config(context.Background(), nil)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(vals).To(HaveKeyWithValue("key", "val"))
	})
}
