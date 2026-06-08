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

package platform

import (
	"testing"

	. "github.com/onsi/gomega"
	k8serr "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	configApi "github.com/lburgazzoli/opendatahub-module-operator/orchestrator/api/config/v1alpha1"
	orchestratorconfig "github.com/lburgazzoli/opendatahub-module-operator/orchestrator/pkg/config"
	"github.com/lburgazzoli/opendatahub-module-operator/orchestrator/pkg/module"
	odhTypes "github.com/opendatahub-io/operator-actions-framework/controller/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestEnabledModulesAtRunlevelFiltersBySpec(t *testing.T) {
	g := NewWithT(t)

	actions := testActions()
	p := &configApi.Platform{
		ObjectMeta: metav1.ObjectMeta{Name: configApi.PlatformInstanceName},
		Spec: configApi.PlatformSpec{
			Modules: []string{"alpha", "beta"},
		},
	}

	modules := actions.enabledModulesAtRunlevel(p, 2)

	g.Expect(modules).To(HaveLen(1))
	g.Expect(modules[0].EffectiveName()).To(Equal("beta"))
}

func TestRunlevelCompleteIgnoresDisabledModulesAtSameRunlevel(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()
	scheme := testScheme(t)
	actions := testActions()

	p := newPlatform([]string{"alpha", "beta"}, 2)
	beta := newPlatformOperator("beta", "2.0.0")
	rr := testRR(t, scheme, p, beta)

	complete, err := actions.runlevelComplete(ctx, rr, p, 2)

	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(complete).To(BeTrue())
}

func TestRunlevelCompleteBlocksOnMissingEnabledModule(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()
	scheme := testScheme(t)
	actions := testActions()

	p := newPlatform([]string{"alpha", "beta"}, 2)
	rr := testRR(t, scheme, p)

	complete, err := actions.runlevelComplete(ctx, rr, p, 2)

	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(complete).To(BeFalse())
}

func TestAdvanceRunlevelSkipsRunlevelsWithoutDesiredModules(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()
	scheme := testScheme(t)
	actions := testActions()

	p := newPlatform([]string{"alpha", "delta"}, 1)
	alpha := newPlatformOperator("alpha", "2.0.0")
	rr := testRR(t, scheme, p, alpha)

	err := actions.advanceRunlevel(ctx, rr)

	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(p.Status.Runlevel).To(Equal(3))
}

func TestAdvanceRunlevelStaysWhenEnabledModuleVersionIsOutdated(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()
	scheme := testScheme(t)
	actions := testActions()

	p := newPlatform([]string{"alpha", "beta"}, 1)
	alpha := newPlatformOperator("alpha", "wrong-version")
	rr := testRR(t, scheme, p, alpha)

	err := actions.advanceRunlevel(ctx, rr)

	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(p.Status.Runlevel).To(Equal(1))
}

func TestPruneModulesDeletesDisabledPlatformOperators(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()
	scheme := testScheme(t)
	actions := testActions()

	p := newPlatform([]string{"alpha"}, 1)
	alpha := newPlatformOperator("alpha", "2.0.0")
	beta := newPlatformOperator("beta", "2.0.0")
	rr := testRR(t, scheme, p, alpha, beta)

	err := actions.pruneModules(ctx, rr)

	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(rr.Client.Get(ctx, client.ObjectKey{Name: "alpha"}, &configApi.PlatformOperator{})).To(Succeed())
	g.Expect(rr.Client.Get(ctx, client.ObjectKey{Name: "beta"}, &configApi.PlatformOperator{})).
		To(Satisfy(k8serr.IsNotFound))
}

func TestPruneModulesKeepsDesiredPlatformOperators(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()
	scheme := testScheme(t)
	actions := testActions()

	p := newPlatform([]string{"alpha", "beta"}, 1)
	alpha := newPlatformOperator("alpha", "2.0.0")
	beta := newPlatformOperator("beta", "2.0.0")
	rr := testRR(t, scheme, p, alpha, beta)

	err := actions.pruneModules(ctx, rr)

	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(rr.Client.Get(ctx, client.ObjectKey{Name: "alpha"}, &configApi.PlatformOperator{})).To(Succeed())
	g.Expect(rr.Client.Get(ctx, client.ObjectKey{Name: "beta"}, &configApi.PlatformOperator{})).To(Succeed())
}

func testScheme(t *testing.T) *runtime.Scheme {
	t.Helper()

	scheme := runtime.NewScheme()
	err := configApi.AddToScheme(scheme)
	if err != nil {
		t.Fatalf("add config api to scheme: %v", err)
	}

	return scheme
}

func testActions() *platformActions {
	return &platformActions{
		registry: testRegistry(),
		cfg: &orchestratorconfig.Config{
			Distribution: orchestratorconfig.DistributionConfig{
				Name:    "TestPlatform",
				Version: "2.0.0",
			},
		},
	}
}

func testRegistry() *module.ModuleRegistry {
	r := module.NewModuleRegistry("opendatahub", "/charts")
	r.Register(&module.Module{
		Name:      "alpha",
		GVK:       schema.GroupVersionKind{Group: "test.io", Version: "v1", Kind: "Alpha"},
		Namespace: "ns-alpha",
		Runlevel:  1,
	})
	r.Register(&module.Module{
		Name:      "beta",
		GVK:       schema.GroupVersionKind{Group: "test.io", Version: "v1", Kind: "Beta"},
		Namespace: "ns-beta",
		Runlevel:  2,
	})
	r.Register(&module.Module{
		Name:      "gamma",
		GVK:       schema.GroupVersionKind{Group: "test.io", Version: "v1", Kind: "Gamma"},
		Namespace: "ns-gamma",
		Runlevel:  2,
	})
	r.Register(&module.Module{
		Name:      "delta",
		GVK:       schema.GroupVersionKind{Group: "test.io", Version: "v1", Kind: "Delta"},
		Namespace: "ns-delta",
		Runlevel:  3,
	})
	r.ComputeRunlevels()

	return r
}

func testRR(
	t *testing.T,
	scheme *runtime.Scheme,
	platform *configApi.Platform,
	objs ...client.Object,
) *odhTypes.ReconciliationRequest {
	t.Helper()

	allObjs := make([]client.Object, 0, len(objs)+1)
	allObjs = append(allObjs, objs...)
	allObjs = append(allObjs, platform)

	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(allObjs...).
		Build()

	return &odhTypes.ReconciliationRequest{
		Client:   c,
		Instance: platform,
	}
}

func newPlatform(modules []string, runlevel int) *configApi.Platform {
	return &configApi.Platform{
		ObjectMeta: metav1.ObjectMeta{Name: configApi.PlatformInstanceName},
		Spec: configApi.PlatformSpec{
			Modules: modules,
		},
		Status: configApi.PlatformStatus{
			Runlevel: runlevel,
			Distribution: configApi.DistributionInfo{
				Name:    "TestPlatform",
				Version: "1.0.0",
			},
		},
	}
}

func newPlatformOperator(name string, version string) *configApi.PlatformOperator {
	return &configApi.PlatformOperator{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Status: configApi.PlatformOperatorStatus{
			Distribution: configApi.DistributionInfo{
				Name:    "TestPlatform",
				Version: version,
			},
		},
	}
}
