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

package platformoperator

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	. "github.com/onsi/gomega"
	k8serr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/event"

	configApi "github.com/lburgazzoli/opendatahub-module-operator/orchestrator/api/config/v1alpha1"
	orchestratorconfig "github.com/lburgazzoli/opendatahub-module-operator/orchestrator/pkg/config"
	"github.com/lburgazzoli/opendatahub-module-operator/orchestrator/pkg/module"
	actionerrors "github.com/opendatahub-io/operator-actions-framework/controller/actions/errors"
	"github.com/opendatahub-io/operator-actions-framework/controller/types"
)

const upgradedTargetVersion = "3.0.0"

func TestEligibleModuleRequests(t *testing.T) {
	t.Run("runlevel-only changes enqueue enabled modules at or above platform runlevel", func(t *testing.T) {
		g := NewWithT(t)
		oldPlatform := newTestPlatform()
		oldPlatform.Spec.Modules = []string{"alpha", "beta"}
		oldPlatform.Status.Runlevel = 1

		newPlatform := oldPlatform.DeepCopy()
		newPlatform.SetResourceVersion("2")
		newPlatform.Status.Runlevel = 2

		reconciler, _ := testReconciler(
			t,
			newTestPlatformOperator("alpha", "1.0.0", "2.0.0"),
			newTestPlatformOperator("beta", "1.0.0", "2.0.0"),
		)
		g.Expect(reconciler.platformChangePredicate().Update(event.UpdateEvent{
			ObjectOld: oldPlatform,
			ObjectNew: newPlatform,
		})).To(BeTrue())

		requests := reconciler.eligibleModuleRequests(t.Context(), newPlatform)

		g.Expect(requestNames(requests)).To(ConsistOf("beta"))
	})

	t.Run("distribution changes enqueue modules with stale target", func(t *testing.T) {
		g := NewWithT(t)
		oldPlatform := newTestPlatform()
		oldPlatform.Spec.Modules = []string{"alpha", "beta"}

		newPlatform := oldPlatform.DeepCopy()
		newPlatform.SetResourceVersion("2")
		newPlatform.Status.Distribution.Target.Version = upgradedTargetVersion

		reconciler, _ := testReconciler(
			t,
			newTestPlatformOperator("alpha", upgradedTargetVersion, upgradedTargetVersion),
			newTestPlatformOperator("beta", "1.0.0", "2.0.0"),
		)
		g.Expect(reconciler.platformChangePredicate().Update(event.UpdateEvent{
			ObjectOld: oldPlatform,
			ObjectNew: newPlatform,
		})).To(BeTrue())

		requests := reconciler.eligibleModuleRequests(t.Context(), newPlatform)

		g.Expect(requestNames(requests)).To(ConsistOf("beta"))
	})

	t.Run("distribution changes in unstructured events trigger the predicate", func(t *testing.T) {
		g := NewWithT(t)
		oldPlatform := newTestPlatform()
		oldPlatform.Spec.Modules = []string{"alpha", "beta"}

		newPlatform := oldPlatform.DeepCopy()
		newPlatform.SetResourceVersion("2")
		newPlatform.Status.Distribution.Target.Version = upgradedTargetVersion

		reconciler, _ := testReconciler(
			t,
			newTestPlatformOperator("alpha", upgradedTargetVersion, upgradedTargetVersion),
			newTestPlatformOperator("beta", "1.0.0", "2.0.0"),
		)
		g.Expect(reconciler.platformChangePredicate().Update(event.UpdateEvent{
			ObjectOld: toUnstructuredObject(t, oldPlatform),
			ObjectNew: toUnstructuredObject(t, newPlatform),
		})).To(BeTrue())
	})

	t.Run("distribution changes wake lower runlevel modules with stale target", func(t *testing.T) {
		g := NewWithT(t)
		oldPlatform := newTestPlatform()
		oldPlatform.Spec.Modules = []string{"alpha", "beta"}
		oldPlatform.Status.Runlevel = 2

		newPlatform := oldPlatform.DeepCopy()
		newPlatform.SetResourceVersion("2")
		newPlatform.Status.Distribution.Target.Version = upgradedTargetVersion

		reconciler, _ := testReconciler(
			t,
			newTestPlatformOperator("alpha", "1.0.0", "2.0.0"),
			newTestPlatformOperator("beta", upgradedTargetVersion, upgradedTargetVersion),
		)
		g.Expect(reconciler.platformChangePredicate().Update(event.UpdateEvent{
			ObjectOld: oldPlatform,
			ObjectNew: newPlatform,
		})).To(BeTrue())

		requests := reconciler.eligibleModuleRequests(t.Context(), newPlatform)

		g.Expect(requestNames(requests)).To(ConsistOf("alpha"))
	})

	t.Run("distribution changes skip missing modules", func(t *testing.T) {
		g := NewWithT(t)
		oldPlatform := newTestPlatform()
		oldPlatform.Spec.Modules = []string{"alpha", "beta"}

		newPlatform := oldPlatform.DeepCopy()
		newPlatform.SetResourceVersion("2")
		newPlatform.Status.Distribution.Target.Version = upgradedTargetVersion

		reconciler, _ := testReconciler(
			t,
			newTestPlatformOperator("alpha", upgradedTargetVersion, upgradedTargetVersion),
		)
		g.Expect(reconciler.platformChangePredicate().Update(event.UpdateEvent{
			ObjectOld: oldPlatform,
			ObjectNew: newPlatform,
		})).To(BeTrue())

		requests := reconciler.eligibleModuleRequests(t.Context(), newPlatform)

		g.Expect(requests).To(BeEmpty())
	})

	t.Run("distribution changes skip converged enabled modules", func(t *testing.T) {
		g := NewWithT(t)
		oldPlatform := newTestPlatform()
		oldPlatform.Spec.Modules = []string{"alpha", "beta"}

		newPlatform := oldPlatform.DeepCopy()
		newPlatform.SetResourceVersion("2")
		newPlatform.Status.Distribution.Target.Version = upgradedTargetVersion

		reconciler, _ := testReconciler(
			t,
			newTestPlatformOperator("alpha", upgradedTargetVersion, upgradedTargetVersion),
			newTestPlatformOperator("beta", upgradedTargetVersion, upgradedTargetVersion),
		)
		g.Expect(reconciler.platformChangePredicate().Update(event.UpdateEvent{
			ObjectOld: oldPlatform,
			ObjectNew: newPlatform,
		})).To(BeTrue())

		requests := reconciler.eligibleModuleRequests(t.Context(), newPlatform)

		g.Expect(requests).To(BeEmpty())
	})

	t.Run("distribution changes skip deleting platform operators", func(t *testing.T) {
		g := NewWithT(t)
		oldPlatform := newTestPlatform()
		oldPlatform.Spec.Modules = []string{"alpha", "beta"}

		newPlatform := oldPlatform.DeepCopy()
		newPlatform.SetResourceVersion("2")
		newPlatform.Status.Distribution.Target.Version = upgradedTargetVersion

		deletingPO := newTestPlatformOperator("beta", "1.0.0", "2.0.0")
		now := metav1.Now()
		deletingPO.SetFinalizers([]string{"test.finalizer"})
		deletingPO.SetDeletionTimestamp(&now)

		reconciler, _ := testReconciler(
			t,
			newTestPlatformOperator("alpha", upgradedTargetVersion, upgradedTargetVersion),
			deletingPO,
		)
		g.Expect(reconciler.platformChangePredicate().Update(event.UpdateEvent{
			ObjectOld: oldPlatform,
			ObjectNew: newPlatform,
		})).To(BeTrue())

		requests := reconciler.eligibleModuleRequests(t.Context(), newPlatform)

		g.Expect(requests).To(BeEmpty())
	})

	t.Run("distribution changes skip modules already at platform target", func(t *testing.T) {
		g := NewWithT(t)
		oldPlatform := newTestPlatform()
		oldPlatform.Spec.Modules = []string{"alpha", "beta"}

		newPlatform := oldPlatform.DeepCopy()
		newPlatform.SetResourceVersion("2")
		newPlatform.Status.Distribution.Target.Version = upgradedTargetVersion

		reconciler, _ := testReconciler(
			t,
			newTestPlatformOperator("alpha", upgradedTargetVersion, "2.0.0"),
			newTestPlatformOperator("beta", "2.0.0", "2.0.0"),
		)
		g.Expect(reconciler.platformChangePredicate().Update(event.UpdateEvent{
			ObjectOld: oldPlatform,
			ObjectNew: newPlatform,
		})).To(BeTrue())

		requests := reconciler.eligibleModuleRequests(t.Context(), newPlatform)

		g.Expect(requestNames(requests)).To(ConsistOf("beta"))
	})

	t.Run("skip registered modules not enabled on platform", func(t *testing.T) {
		g := NewWithT(t)
		oldPlatform := newTestPlatform()
		oldPlatform.Spec.Modules = []string{"alpha"}

		newPlatform := oldPlatform.DeepCopy()
		newPlatform.SetResourceVersion("2")
		newPlatform.Status.Distribution.Target.Version = upgradedTargetVersion

		reconciler, _ := testReconciler(
			t,
			newTestPlatformOperator("alpha", upgradedTargetVersion, upgradedTargetVersion),
			newTestPlatformOperator("beta", "1.0.0", "2.0.0"),
		)
		g.Expect(reconciler.platformChangePredicate().Update(event.UpdateEvent{
			ObjectOld: oldPlatform,
			ObjectNew: newPlatform,
		})).To(BeTrue())

		requests := reconciler.eligibleModuleRequests(t.Context(), newPlatform)

		g.Expect(requests).To(BeEmpty())
	})

	t.Run("eligible requests accept unstructured platform events", func(t *testing.T) {
		g := NewWithT(t)
		platform := newTestPlatform()
		platform.Spec.Modules = []string{"alpha", "beta"}
		platform.Status.Distribution.Target.Version = upgradedTargetVersion

		reconciler, _ := testReconciler(
			t,
			newTestPlatformOperator("alpha", upgradedTargetVersion, upgradedTargetVersion),
			newTestPlatformOperator("beta", "1.0.0", "2.0.0"),
		)

		requests := reconciler.eligibleModuleRequests(t.Context(), toUnstructuredObject(t, platform))

		g.Expect(requestNames(requests)).To(ConsistOf("beta"))
	})
}

func TestCheckRunlevel(t *testing.T) {
	t.Run("missing Platform pauses with quick retry", func(t *testing.T) {
		g := NewWithT(t)
		reconciler, c := testReconciler(t)
		po := newTestPlatformOperator("alpha", "2.0.0", "2.0.0")
		rr := &types.ReconciliationRequest{
			Client:   c,
			Instance: po,
		}
		reconciler.contexts[po.Name] = &moduleContext{
			module: reconciler.registry.ModuleByName(po.Name),
		}

		err := reconciler.checkRunlevel(t.Context(), rr)

		var pauseErr actionerrors.PauseError
		g.Expect(errors.As(err, &pauseErr)).To(BeTrue())
		g.Expect(pauseErr.Delay()).To(Equal(500 * time.Millisecond))
	})

	t.Run("deleting PlatformOperator does not pause on missing Platform", func(t *testing.T) {
		g := NewWithT(t)
		reconciler, c := testReconciler(t)
		now := metav1.Now()
		po := newTestPlatformOperator("alpha", "2.0.0", "2.0.0")
		po.SetDeletionTimestamp(&now)
		rr := &types.ReconciliationRequest{
			Client:   c,
			Instance: po,
		}

		err := reconciler.checkRunlevel(t.Context(), rr)

		g.Expect(err).NotTo(HaveOccurred())
	})

	t.Run("lower runlevel modules are allowed during upgrade", func(t *testing.T) {
		g := NewWithT(t)
		platform := newTestPlatform()
		platform.Status.Runlevel = 2
		reconciler, c := testReconciler(t, platform)
		po := newTestPlatformOperator("alpha", "1.0.0", "2.0.0")
		rr := &types.ReconciliationRequest{
			Client:   c,
			Instance: po,
		}
		reconciler.contexts[po.Name] = &moduleContext{
			module: reconciler.registry.ModuleByName(po.Name),
		}

		err := reconciler.checkRunlevel(t.Context(), rr)

		g.Expect(err).NotTo(HaveOccurred())
	})

	t.Run("higher runlevel modules pause during upgrade", func(t *testing.T) {
		g := NewWithT(t)
		reconciler, c := testReconciler(t, newTestPlatform())
		po := newTestPlatformOperator("beta", "1.0.0", "2.0.0")
		rr := &types.ReconciliationRequest{
			Client:   c,
			Instance: po,
		}
		reconciler.contexts[po.Name] = &moduleContext{
			module: reconciler.registry.ModuleByName(po.Name),
		}

		err := reconciler.checkRunlevel(t.Context(), rr)

		var pauseErr actionerrors.PauseError
		g.Expect(errors.As(err, &pauseErr)).To(BeTrue())
		g.Expect(err.Error()).To(ContainSubstring(`waiting for runlevel 2`))
	})

	t.Run("higher runlevel modules pause during fresh install", func(t *testing.T) {
		g := NewWithT(t)
		platform := newTestPlatform()
		platform.Status.Distribution.Current = configApi.Distribution{}
		reconciler, c := testReconciler(t, platform)
		po := newTestPlatformOperator("beta", "", "2.0.0")
		rr := &types.ReconciliationRequest{
			Client:   c,
			Instance: po,
		}
		reconciler.contexts[po.Name] = &moduleContext{
			module: reconciler.registry.ModuleByName(po.Name),
		}

		err := reconciler.checkRunlevel(t.Context(), rr)

		var pauseErr actionerrors.PauseError
		g.Expect(errors.As(err, &pauseErr)).To(BeTrue())
		g.Expect(err.Error()).To(ContainSubstring(`waiting for runlevel 2`))
	})

	t.Run("steady state ignores runlevel", func(t *testing.T) {
		g := NewWithT(t)
		platform := newTestPlatform()
		platform.Status.Distribution.Current = platform.Status.Distribution.Target
		reconciler, c := testReconciler(t, platform)
		po := newTestPlatformOperator("beta", "2.0.0", "2.0.0")
		rr := &types.ReconciliationRequest{
			Client:   c,
			Instance: po,
		}
		reconciler.contexts[po.Name] = &moduleContext{
			module: reconciler.registry.ModuleByName(po.Name),
		}

		err := reconciler.checkRunlevel(t.Context(), rr)

		g.Expect(err).NotTo(HaveOccurred())
	})
}

func TestReadModuleRelease(t *testing.T) {
	t.Run("returns empty distribution when no module resource exists", func(t *testing.T) {
		g := NewWithT(t)
		reconciler, c := testReconciler(t)
		mc := &moduleContext{module: reconciler.registry.ModuleByName("alpha")}

		release, found, err := reconciler.readModuleRelease(t.Context(), c, mc.module)

		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(found).To(BeFalse())
		g.Expect(release).To(Equal(configApi.Distribution{}))
	})

	t.Run("returns release from singleton module resource", func(t *testing.T) {
		g := NewWithT(t)
		resource := newModuleResource("alpha-cr", schema.GroupVersionKind{Group: "test.io", Version: "v1", Kind: "Alpha"}, "TestPlatform", "1.2.3")
		reconciler, c := testReconciler(t, resource)
		mc := &moduleContext{module: reconciler.registry.ModuleByName("alpha")}

		release, found, err := reconciler.readModuleRelease(t.Context(), c, mc.module)

		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(found).To(BeTrue())
		g.Expect(release).To(Equal(configApi.Distribution{Name: "TestPlatform", Version: "1.2.3"}))
	})

	t.Run("errors when multiple module resources exist", func(t *testing.T) {
		g := NewWithT(t)
		reconciler, c := testReconciler(
			t,
			newModuleResource("alpha-cr-1", schema.GroupVersionKind{Group: "test.io", Version: "v1", Kind: "Alpha"}, "TestPlatform", "1.2.3"),
			newModuleResource("alpha-cr-2", schema.GroupVersionKind{Group: "test.io", Version: "v1", Kind: "Alpha"}, "TestPlatform", "4.5.6"),
		)
		mc := &moduleContext{module: reconciler.registry.ModuleByName("alpha")}

		release, found, err := reconciler.readModuleRelease(t.Context(), c, mc.module)

		g.Expect(release).To(Equal(configApi.Distribution{}))
		g.Expect(found).To(BeFalse())
		g.Expect(err).To(HaveOccurred())
		g.Expect(k8serr.IsNotFound(err)).To(BeFalse())
	})
}

func TestReportStatus(t *testing.T) {
	t.Run("falls back to configured distribution when module resource is missing", func(t *testing.T) {
		g := NewWithT(t)
		reconciler, c := testReconciler(t)
		po := newTestPlatformOperator("alpha", "", "")
		rr := &types.ReconciliationRequest{
			Client:   c,
			Instance: po,
		}

		err := reconciler.reportStatus(t.Context(), rr)

		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(po.Status.Distribution.Current).To(Equal(configApi.Distribution{
			Name:    "TestPlatform",
			Version: "2.0.0",
		}))
		g.Expect(po.Status.Distribution.Target).To(Equal(configApi.Distribution{
			Name:    "TestPlatform",
			Version: "2.0.0",
		}))
	})

	t.Run("does not fall back when module resource exists without release", func(t *testing.T) {
		g := NewWithT(t)
		resource := newModuleResource(
			"alpha-cr",
			schema.GroupVersionKind{Group: "test.io", Version: "v1", Kind: "Alpha"},
			"",
			"",
		)
		reconciler, c := testReconciler(t, resource)
		po := newTestPlatformOperator("alpha", "", "")
		rr := &types.ReconciliationRequest{
			Client:   c,
			Instance: po,
		}

		err := reconciler.reportStatus(t.Context(), rr)

		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(po.Status.Distribution.Current).To(Equal(configApi.Distribution{}))
		g.Expect(po.Status.Distribution.Target).To(Equal(configApi.Distribution{
			Name:    "TestPlatform",
			Version: "2.0.0",
		}))
	})

	t.Run("uses module release when module resource exists with release", func(t *testing.T) {
		g := NewWithT(t)
		resource := newModuleResource(
			"alpha-cr",
			schema.GroupVersionKind{Group: "test.io", Version: "v1", Kind: "Alpha"},
			"TestPlatform",
			"1.2.3",
		)
		reconciler, c := testReconciler(t, resource)
		po := newTestPlatformOperator("alpha", "", "")
		rr := &types.ReconciliationRequest{
			Client:   c,
			Instance: po,
		}

		err := reconciler.reportStatus(t.Context(), rr)

		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(po.Status.Distribution.Current).To(Equal(configApi.Distribution{
			Name:    "TestPlatform",
			Version: "1.2.3",
		}))
		g.Expect(po.Status.Distribution.Target).To(Equal(configApi.Distribution{
			Name:    "TestPlatform",
			Version: "2.0.0",
		}))
	})
}

func TestComputeValuesReturnsConfigError(t *testing.T) {
	g := NewWithT(t)
	reconciler, c := testReconciler(t)

	mod := reconciler.registry.ModuleByName("alpha")
	mod.Config = func(_ context.Context, _ client.Client) (map[string]any, error) {
		return nil, fmt.Errorf("config lookup failed")
	}

	mc := reconciler.contexts["alpha"]

	_, err := reconciler.computeValues(t.Context(), c, mc)

	g.Expect(err).To(MatchError(ContainSubstring("computing config values for module")))
	g.Expect(err).To(MatchError(ContainSubstring("config lookup failed")))
}

func TestRenderChartPropagatesConfigError(t *testing.T) {
	g := NewWithT(t)
	reconciler, c := testReconciler(t)

	mod := reconciler.registry.ModuleByName("alpha")
	mod.Config = func(_ context.Context, _ client.Client) (map[string]any, error) {
		return nil, fmt.Errorf("config lookup failed")
	}

	po := newTestPlatformOperator("alpha", "2.0.0", "2.0.0")
	rr := &types.ReconciliationRequest{
		Client:   c,
		Instance: po,
	}

	err := reconciler.renderChart(t.Context(), rr)

	g.Expect(err).To(MatchError(ContainSubstring("config lookup failed")))
}

func testReconciler(t *testing.T, objs ...client.Object) (*PlatformOperatorReconciler, client.Client) {
	t.Helper()

	scheme := runtime.NewScheme()
	err := configApi.AddToScheme(scheme)
	if err != nil {
		t.Fatalf("add config api to scheme: %v", err)
	}

	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(objs...).
		Build()

	registry := testModuleRegistry()
	cfg := &orchestratorconfig.Config{
		Distribution: configApi.Distribution{
			Name:    "TestPlatform",
			Version: "2.0.0",
		},
	}
	contexts := make(map[string]*moduleContext, len(registry.Modules()))
	for _, mod := range registry.Modules() {
		mc, err := newModuleContext(mod, cfg)
		if err != nil {
			t.Fatalf("create module context: %v", err)
		}
		contexts[mod.Name] = mc
	}

	return &PlatformOperatorReconciler{
		registry: registry,
		cfg:      cfg,
		client:   c,
		contexts: contexts,
	}, c
}

func testModuleRegistry() *module.Registry {
	alpha, err := module.NewModule(module.ModuleSpec{
		Name:      "alpha",
		GVK:       schema.GroupVersionKind{Group: "test.io", Version: "v1", Kind: "Alpha"},
		Namespace: "ns-alpha",
		Runlevel:  1,
		ChartPath: testChartPath(),
	})
	if err != nil {
		panic(err)
	}
	beta, err := module.NewModule(module.ModuleSpec{
		Name:      "beta",
		GVK:       schema.GroupVersionKind{Group: "test.io", Version: "v1", Kind: "Beta"},
		Namespace: "ns-beta",
		Runlevel:  2,
		ChartPath: testChartPath(),
	})
	if err != nil {
		panic(err)
	}
	r, err := module.NewRegistry([]*module.Module{alpha, beta})
	if err != nil {
		panic(err)
	}

	return r
}

func testChartPath() string {
	return filepath.Join("..", "..", "..", "test", "support", "testdata", "charts", "test-module")
}

func newTestPlatform() *configApi.Platform {
	return &configApi.Platform{
		ObjectMeta: metav1.ObjectMeta{Name: configApi.PlatformInstanceName},
		Status: configApi.PlatformStatus{
			Runlevel: 1,
			Distribution: configApi.DistributionStatus{
				Current: configApi.Distribution{
					Name:    "TestPlatform",
					Version: "1.0.0",
				},
				Target: configApi.Distribution{
					Name:    "TestPlatform",
					Version: "2.0.0",
				},
			},
		},
	}
}

func newTestPlatformOperator(name string, currentVersion string, targetVersion string) *configApi.PlatformOperator {
	return &configApi.PlatformOperator{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Status: configApi.PlatformOperatorStatus{
			Distribution: configApi.DistributionStatus{
				Current: configApi.Distribution{
					Name:    "TestPlatform",
					Version: currentVersion,
				},
				Target: configApi.Distribution{
					Name:    "TestPlatform",
					Version: targetVersion,
				},
			},
		},
	}
}

func requestNames(requests []ctrl.Request) []string {
	names := make([]string, 0, len(requests))
	for _, request := range requests {
		names = append(names, request.Name)
	}

	return names
}

func toUnstructuredObject(t *testing.T, obj client.Object) *unstructured.Unstructured {
	t.Helper()

	content, err := runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
	if err != nil {
		t.Fatalf("convert object to unstructured: %v", err)
	}

	return &unstructured.Unstructured{Object: content}
}

func newModuleResource(
	name string,
	gvk schema.GroupVersionKind,
	releaseName string,
	releaseVersion string,
) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(gvk)
	obj.SetName(name)
	if releaseName != "" || releaseVersion != "" {
		obj.Object["status"] = map[string]any{
			"release": map[string]any{
				"name":    releaseName,
				"version": releaseVersion,
			},
		}
	}

	return obj
}
