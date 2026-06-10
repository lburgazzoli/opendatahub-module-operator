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

package manager

import (
	"path/filepath"
	"testing"

	. "github.com/onsi/gomega"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	k8slabels "k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/cache"

	"github.com/lburgazzoli/opendatahub-module-operator/orchestrator/pkg/module"
	odhLabels "github.com/opendatahub-io/opendatahub-operator/v2/pkg/metadata/labels"
)

func TestNewSchemeIncludesCRDType(t *testing.T) {
	g := NewWithT(t)

	scheme := NewScheme()

	g.Expect(scheme.Recognizes(apiextensionsv1.SchemeGroupVersion.WithKind("CustomResourceDefinition"))).To(BeTrue())
}

func TestBuildCacheNamespacesUsesPartOfLabelPresence(t *testing.T) {
	g := NewWithT(t)

	alpha, err := module.NewModule(module.ModuleSpec{
		Name:      "alpha",
		GVK:       schema.GroupVersionKind{Group: "test.io", Version: "v1", Kind: "Alpha"},
		Namespace: "team-a",
		ChartPath: testChartPath(),
	})
	g.Expect(err).NotTo(HaveOccurred())
	registry, err := module.NewRegistry([]*module.Module{alpha})
	g.Expect(err).NotTo(HaveOccurred())

	configs := buildCacheNamespaces(registry)
	selector := configs["team-a"].LabelSelector

	g.Expect(selector.Matches(k8slabels.Set{
		odhLabels.PlatformPartOf: "ray",
	})).To(BeTrue())
	g.Expect(selector.Matches(k8slabels.Set{
		odhLabels.PlatformPartOf: "spark",
	})).To(BeTrue())
	g.Expect(selector.Matches(k8slabels.Set{})).To(BeFalse())
}

func TestBuildCacheNamespacesIncludesModuleNamespacesAndAllNamespaces(t *testing.T) {
	g := NewWithT(t)

	alpha, err := module.NewModule(module.ModuleSpec{
		Name:      "alpha",
		GVK:       schema.GroupVersionKind{Group: "test.io", Version: "v1", Kind: "Alpha"},
		Namespace: "team-b",
		ChartPath: testChartPath(),
	})
	g.Expect(err).NotTo(HaveOccurred())
	beta, err := module.NewModule(module.ModuleSpec{
		Name:      "beta",
		GVK:       schema.GroupVersionKind{Group: "test.io", Version: "v1", Kind: "Beta"},
		Namespace: "team-a",
		ChartPath: testChartPath(),
	})
	g.Expect(err).NotTo(HaveOccurred())
	gamma, err := module.NewModule(module.ModuleSpec{
		Name:      "gamma",
		GVK:       schema.GroupVersionKind{Group: "test.io", Version: "v1", Kind: "Gamma"},
		Namespace: "team-a",
		ChartPath: testChartPath(),
	})
	g.Expect(err).NotTo(HaveOccurred())
	registry, err := module.NewRegistry([]*module.Module{alpha, beta, gamma})
	g.Expect(err).NotTo(HaveOccurred())

	configs := buildCacheNamespaces(registry)

	g.Expect(configs).To(HaveKey("team-a"))
	g.Expect(configs).To(HaveKey("team-b"))
	g.Expect(configs).To(HaveKey(cache.AllNamespaces))
}

func testChartPath() string {
	return filepath.Join("..", "..", "test", "support", "testdata", "charts", "test-module")
}
