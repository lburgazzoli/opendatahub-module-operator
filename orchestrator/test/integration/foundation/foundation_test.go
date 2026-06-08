//go:build integration

package foundation

import (
	"fmt"
	"os"
	"testing"

	k8sm "github.com/lburgazzoli/gomega-matchers/pkg/matchers/k8s"
	. "github.com/onsi/gomega"

	"github.com/lburgazzoli/gomega-matchers/pkg/matchers/jq"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	configApi "github.com/lburgazzoli/opendatahub-module-operator/orchestrator/api/config/v1alpha1"
	"github.com/lburgazzoli/opendatahub-module-operator/orchestrator/pkg/resources/gvk"
	isupport "github.com/lburgazzoli/opendatahub-module-operator/orchestrator/test/integration/support"
	testsupport "github.com/lburgazzoli/opendatahub-module-operator/orchestrator/test/support"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func TestMain(m *testing.M) {
	os.Exit(isupport.Run(m, isupport.RunConfig{
		Modules:        foundationModules(),
		CleanupModules: foundationCleanupModules(),
	}))
}

func TestFoundation(t *testing.T) {
	t.Run("empty platform should reconcile", testEmptyPlatform)
	t.Run("module deployment", testModuleDeployment)
	t.Run("module version propagation", testVersionPropagation)
	t.Run("disabling modules should clean up resources", testDisableModules)
}

func testEmptyPlatform(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()
	suite := isupport.NewSuite(t)
	suite.SetupTest(t)

	p := &configApi.Platform{
		ObjectMeta: metav1.ObjectMeta{Name: configApi.PlatformInstanceName},
	}

	g.Expect(suite.Client.Create(ctx, p)).To(Succeed())

	g.Eventually(ctx, suite.K.Get(p)).Should(
		jq.Match(`(.status.distribution.version // "") | length > 0`),
	)
	g.Eventually(ctx, suite.K.List(&configApi.PlatformOperatorList{})).Should(
		k8sm.IsEmptyList(),
	)
}

func testModuleDeployment(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()
	suite := isupport.NewSuite(t)
	suite.SetupTest(t)

	p := isupport.NewPlatformWithModules(suite.PlatformModuleNames())
	g.Expect(suite.Client.Create(ctx, p)).To(Succeed())

	for _, mod := range suite.Modules {
		moduleName := mod.EffectiveName()
		g.Eventually(ctx, suite.K.Get(testsupport.PlatformOperator(moduleName))).Should(
			testsupport.HaveTrackedResources(),
		)
	}

	t.Run("all modules should track resources in status", func(t *testing.T) {
		for _, mod := range suite.Modules {
			t.Run(mod.EffectiveName(), func(t *testing.T) {
				g := NewWithT(t)
				ctx := t.Context()
				moduleName := mod.EffectiveName()
				g.Eventually(ctx, suite.K.Get(testsupport.PlatformOperator(moduleName))).Should(
					SatisfyAll(
						testsupport.HaveTrackedResource(gvk.ServiceAccount),
						testsupport.HaveTrackedResource(gvk.CustomResourceDefinition),
					),
				)
			})
		}
	})

	t.Run("all modules should report chart info", func(t *testing.T) {
		for _, mod := range suite.Modules {
			t.Run(mod.EffectiveName(), func(t *testing.T) {
				g := NewWithT(t)
				ctx := t.Context()
				moduleName := mod.EffectiveName()
				g.Eventually(ctx, suite.K.Get(testsupport.PlatformOperator(moduleName))).Should(
					testsupport.HaveChartInfo("test-module", mod.ChartPath),
				)
			})
		}
	})

	t.Run("all modules should report runlevel", func(t *testing.T) {
		for _, mod := range suite.Modules {
			t.Run(mod.EffectiveName(), func(t *testing.T) {
				g := NewWithT(t)
				ctx := t.Context()
				moduleName := mod.EffectiveName()
				g.Eventually(ctx, suite.K.Get(testsupport.PlatformOperator(moduleName))).Should(
					testsupport.HaveRunlevel(mod.Runlevel),
				)
			})
		}
	})

	t.Run("each module should have its own CRD", func(t *testing.T) {
		for _, mod := range suite.Modules {
			crdName := fmt.Sprintf("%ss.%s", mod.EffectiveName(), mod.GVK.Group)
			t.Run(mod.EffectiveName(), func(t *testing.T) {
				g := NewWithT(t)
				ctx := t.Context()
				moduleName := mod.EffectiveName()
				g.Eventually(ctx, suite.K.Get(testsupport.PlatformOperator(moduleName))).Should(
					testsupport.HaveTrackedNamedResource(gvk.CustomResourceDefinition, crdName),
				)
			})
		}
	})

	t.Run("deployed resources should have part-of label", func(t *testing.T) {
		for _, mod := range suite.Modules {
			t.Run(mod.EffectiveName(), func(t *testing.T) {
				g := NewWithT(t)
				ctx := t.Context()
				moduleName := mod.EffectiveName()
				sa := &corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{
					Name: moduleName, Namespace: mod.Namespace,
				}}
				g.Eventually(ctx, suite.K.Get(sa)).Should(
					k8sm.HasLabel("platform.opendatahub.io/part-of", moduleName),
				)
			})
		}
	})

	t.Run("deployed resources should have owner reference", func(t *testing.T) {
		for _, mod := range suite.Modules {
			t.Run(mod.EffectiveName(), func(t *testing.T) {
				g := NewWithT(t)
				ctx := t.Context()
				moduleName := mod.EffectiveName()
				sa := &corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{
					Name: moduleName, Namespace: mod.Namespace,
				}}
				g.Eventually(ctx, suite.K.Get(sa)).Should(
					k8sm.IsControlledBy(testsupport.PlatformOperatorOwner(moduleName)),
				)
			})
		}
	})

	t.Run("config values should be projected to configmap", func(t *testing.T) {
		for _, mod := range suite.Modules {
			t.Run(mod.EffectiveName(), func(t *testing.T) {
				g := NewWithT(t)
				ctx := t.Context()
				moduleName := mod.EffectiveName()
				cm := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{
					Name: moduleName + "-config", Namespace: mod.Namespace,
				}}
				g.Eventually(ctx, suite.K.Get(cm)).Should(
					WithTransform(k8sm.Data(), SatisfyAll(
						HaveKeyWithValue("module-name", Equal(moduleName)),
						HaveKeyWithValue("distribution.name", Not(BeEmpty())),
						HaveKeyWithValue("distribution.version", Not(BeEmpty())),
					)),
				)
			})
		}
	})

	t.Run("PlatformOperator should be owned by Platform", func(t *testing.T) {
		for _, mod := range suite.Modules {
			t.Run(mod.EffectiveName(), func(t *testing.T) {
				g := NewWithT(t)
				ctx := t.Context()
				moduleName := mod.EffectiveName()
				g.Eventually(ctx, suite.K.Get(testsupport.PlatformOperator(moduleName))).Should(
					k8sm.IsControlledBy(testsupport.PlatformOwner()),
				)
			})
		}
	})
}

func testVersionPropagation(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()
	suite := isupport.NewSuite(t)
	suite.SetupTest(t)

	p := isupport.NewPlatformWithModules(suite.PlatformModuleNames())
	g.Expect(suite.Client.Create(ctx, p)).To(Succeed())

	for _, mod := range suite.Modules {
		moduleName := mod.EffectiveName()
		g.Eventually(ctx, suite.K.Get(testsupport.PlatformOperator(moduleName))).Should(
			testsupport.HaveTrackedResources(),
		)
	}

	mod := suite.Modules[0]

	isupport.UpsertModuleCRWithVersion(t, suite, mod.GVK, "test-version")

	g.Eventually(ctx, suite.K.Get(testsupport.PlatformOperator(mod.EffectiveName()))).Should(
		testsupport.HaveDistributionVersion("test-version"),
	)
}

func testDisableModules(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()
	suite := isupport.NewSuite(t)
	suite.SetupTest(t)

	p := isupport.NewPlatformWithModules(suite.PlatformModuleNames())
	g.Expect(suite.Client.Create(ctx, p)).To(Succeed())

	for _, mod := range suite.Modules {
		moduleName := mod.EffectiveName()
		g.Eventually(ctx, suite.K.Get(testsupport.PlatformOperator(moduleName))).Should(
			testsupport.HaveTrackedResources(),
		)
	}

	type moduleResources struct {
		name      string
		resources []configApi.ResourceRef
	}

	snapshots := make([]moduleResources, 0, len(suite.Modules))
	for _, mod := range suite.Modules {
		po := &configApi.PlatformOperator{}
		g.Expect(suite.Client.Get(ctx, client.ObjectKey{Name: mod.EffectiveName()}, po)).To(Succeed())
		snapshots = append(snapshots, moduleResources{
			name:      mod.EffectiveName(),
			resources: po.Status.Resources,
		})
	}

	g.Eventually(ctx, k8sm.Update(suite.K, testsupport.Platform(), func(p *configApi.Platform) {
		p.Spec.Modules = nil
	})).Should(
		jq.Match(`(.spec.modules // []) | length == 0`),
	)

	g.Eventually(ctx, suite.K.List(&configApi.PlatformOperatorList{})).Should(
		k8sm.IsEmptyList(),
	)

	for _, snap := range snapshots {
		t.Run(snap.name, func(t *testing.T) {
			g := NewWithT(t)
			ctx := t.Context()
			for _, ref := range snap.resources {
				g.Eventually(ctx, func() error {
					return suite.CheckResourceResetState(ctx, ref)
				}).Should(
					Succeed(),
					"%s %s/%s should match reset behavior",
					ref.Kind,
					ref.Namespace,
					ref.Name,
				)
			}
		})
	}
}
