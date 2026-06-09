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
	"errors"
	"testing"
	"time"

	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	configApi "github.com/lburgazzoli/opendatahub-module-operator/orchestrator/api/config/v1alpha1"
	orchestratorconfig "github.com/lburgazzoli/opendatahub-module-operator/orchestrator/pkg/config"
	"github.com/lburgazzoli/opendatahub-module-operator/orchestrator/pkg/module"
	actionerrors "github.com/opendatahub-io/operator-actions-framework/controller/actions/errors"
	"github.com/opendatahub-io/operator-actions-framework/controller/types"
)

func TestEligibleModuleRequests(t *testing.T) {
	t.Run("enqueues all registered modules", func(t *testing.T) {
		g := NewWithT(t)
		reconciler, _ := testModuleReconciler(t) //nolint:dogsled

		requests := reconciler.eligibleModuleRequests(t.Context(), newTestPlatform())

		g.Expect(requestNames(requests)).To(ConsistOf("alpha", "beta"))
	})
}

func TestCheckRunlevel(t *testing.T) {
	t.Run("missing Platform pauses with quick retry", func(t *testing.T) {
		g := NewWithT(t)
		reconciler, c := testModuleReconciler(t)
		po := newTestPlatformOperator("alpha", "2.0.0")
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
		reconciler, c := testModuleReconciler(t)
		now := metav1.Now()
		po := newTestPlatformOperator("alpha", "2.0.0")
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
		reconciler, c := testModuleReconciler(t, platform)
		po := newTestPlatformOperator("alpha", "1.0.0")
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
		reconciler, c := testModuleReconciler(t, newTestPlatform())
		po := newTestPlatformOperator("beta", "1.0.0")
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
		reconciler, c := testModuleReconciler(t, platform)
		po := newTestPlatformOperator("beta", "2.0.0")
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

func testModuleReconciler(t *testing.T, objs ...client.Object) (*ModuleReconciler, client.Client) {
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

	return &ModuleReconciler{
		registry: testModuleRegistry(),
		cfg: &orchestratorconfig.Config{
			Distribution: configApi.Distribution{
				Name:    "TestPlatform",
				Version: "2.0.0",
			},
		},
		contexts: make(map[string]*moduleContext),
	}, c
}

func testModuleRegistry() *module.ModuleRegistry {
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
	r.ComputeRunlevels()

	return r
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

func newTestPlatformOperator(name string, version string) *configApi.PlatformOperator {
	return &configApi.PlatformOperator{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Status: configApi.PlatformOperatorStatus{
			Distribution: configApi.DistributionStatus{
				Current: configApi.Distribution{
					Name:    "TestPlatform",
					Version: version,
				},
				Target: configApi.Distribution{
					Name:    "TestPlatform",
					Version: "2.0.0",
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
