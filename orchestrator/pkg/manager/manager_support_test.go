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
	"testing"

	. "github.com/onsi/gomega"
	k8slabels "k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/cache"

	"github.com/lburgazzoli/opendatahub-module-operator/orchestrator/pkg/module"
	odhLabels "github.com/opendatahub-io/opendatahub-operator/v2/pkg/metadata/labels"
)

func TestBuildCacheNamespacesUsesPartOfLabelPresence(t *testing.T) {
	g := NewWithT(t)

	registry := module.NewModuleRegistry("opendatahub", "/charts")
	registry.Register(&module.Module{
		GVK:       schema.GroupVersionKind{Group: "test.io", Version: "v1", Kind: "Alpha"},
		Namespace: "team-a",
	})

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

	registry := module.NewModuleRegistry("opendatahub", "/charts")
	registry.Register(&module.Module{
		GVK:       schema.GroupVersionKind{Group: "test.io", Version: "v1", Kind: "Alpha"},
		Namespace: "team-b",
	})
	registry.Register(&module.Module{
		GVK:       schema.GroupVersionKind{Group: "test.io", Version: "v1", Kind: "Beta"},
		Namespace: "team-a",
	})
	registry.Register(&module.Module{
		GVK:       schema.GroupVersionKind{Group: "test.io", Version: "v1", Kind: "Gamma"},
		Namespace: "team-a",
	})

	configs := buildCacheNamespaces(registry)

	g.Expect(configs).To(HaveKey("team-a"))
	g.Expect(configs).To(HaveKey("team-b"))
	g.Expect(configs).To(HaveKey(cache.AllNamespaces))
}
