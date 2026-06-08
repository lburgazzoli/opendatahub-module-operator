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
	"time"

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

func TestModuleEffectiveName(t *testing.T) {
	t.Run("defaults to lowercase kind", func(t *testing.T) {
		g := NewWithT(t)

		m := &module.Module{GVK: testGVK}
		g.Expect(m.EffectiveName()).To(Equal("ray"))
	})

	t.Run("uses explicit name when set", func(t *testing.T) {
		g := NewWithT(t)

		m := &module.Module{Name: "custom", GVK: testGVK}
		g.Expect(m.EffectiveName()).To(Equal("custom"))
	})
}

func TestModuleDefaults(t *testing.T) {
	g := NewWithT(t)

	m := &module.Module{GVK: testGVK}

	g.Expect(m.Timeout).To(Equal(time.Duration(0)))
	g.Expect(m.ConfigHashRollout).To(BeFalse())
	g.Expect(m.AdminAcks).To(BeNil())
	g.Expect(m.Values).To(BeNil())
	g.Expect(m.Ext).To(BeNil())
}

func TestModuleChartLazyLoad(t *testing.T) {
	t.Run("errors when chart path is empty", func(t *testing.T) {
		g := NewWithT(t)

		m := &module.Module{GVK: testGVK}
		chrt, err := m.Chart()
		g.Expect(err).To(HaveOccurred())
		g.Expect(err).To(MatchError(ContainSubstring("chart path not set")))
		g.Expect(chrt).To(BeNil())
	})

	t.Run("errors when chart path does not exist", func(t *testing.T) {
		g := NewWithT(t)

		m := &module.Module{GVK: testGVK, ChartPath: "/nonexistent/chart"}
		chrt, err := m.Chart()
		g.Expect(err).To(HaveOccurred())
		g.Expect(chrt).To(BeNil())
	})

	t.Run("caches error on repeated calls", func(t *testing.T) {
		g := NewWithT(t)

		m := &module.Module{GVK: testGVK}
		_, err1 := m.Chart()
		_, err2 := m.Chart()
		g.Expect(err1).To(HaveOccurred())
		g.Expect(err2).To(HaveOccurred())
		g.Expect(err1).To(Equal(err2))
	})
}

func TestModuleConfig(t *testing.T) {
	t.Run("nil config by default", func(t *testing.T) {
		g := NewWithT(t)

		m := &module.Module{GVK: testGVK}
		g.Expect(m.Config).To(BeNil())
	})

	t.Run("config function returns values", func(t *testing.T) {
		g := NewWithT(t)

		m := &module.Module{
			GVK: testGVK,
			Config: func(_ context.Context, _ client.Client) (map[string]any, error) {
				return map[string]any{"key": "val"}, nil
			},
		}

		vals, err := m.Config(context.Background(), nil)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(vals).To(HaveKeyWithValue("key", "val"))
	})
}
