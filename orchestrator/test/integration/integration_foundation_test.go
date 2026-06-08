//go:build integration

package integration

import (
	"fmt"
	"testing"

	k8sm "github.com/lburgazzoli/gomega-matchers/pkg/matchers/k8s"
	. "github.com/onsi/gomega"

	"github.com/lburgazzoli/gomega-matchers/pkg/matchers/jq"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	configApi "github.com/lburgazzoli/opendatahub-module-operator/orchestrator/api/config/v1alpha1"
	"github.com/lburgazzoli/opendatahub-module-operator/orchestrator/test/support"
)

type foundationTests struct {
	newSuite suiteFactory
}

func (ft *foundationTests) Execute(t *testing.T) {
	t.Run("empty platform should reconcile", ft.testEmptyPlatform)
	t.Run("module deployment", ft.testModuleDeployment)
	t.Run("module version propagation", ft.testVersionPropagation)
	t.Run("disabling modules should clean up resources", ft.testDisableModules)
}

func (ft *foundationTests) testEmptyPlatform(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()
	suite := ft.newSuite(t)
	suite.setupTest(t)

	p := &configApi.Platform{
		ObjectMeta: metav1.ObjectMeta{Name: configApi.PlatformInstanceName},
	}

	g.Expect(suite.client.Create(ctx, p)).To(Succeed())

	g.Eventually(ctx, suite.k.Get(p)).Should(
		jq.Match(`(.status.distribution.version // "") | length > 0`),
	)
	g.Eventually(ctx, suite.k.List(&configApi.PlatformOperatorList{})).Should(
		jq.Match(`length == 0`),
	)
}

func (ft *foundationTests) testModuleDeployment(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()
	suite := ft.newSuite(t)
	suite.setupTest(t)

	p := newPlatformWithModules(suite.platformModuleNames())
	g.Expect(suite.client.Create(ctx, p)).To(Succeed())

	for _, mod := range suite.modules {
		moduleName := mod.EffectiveName()
		g.Eventually(ctx, suite.k.Get(support.PlatformOperator(moduleName))).Should(
			support.HaveTrackedResources(),
		)
	}

	t.Run("all modules should track resources in status", func(t *testing.T) {
		for _, mod := range suite.modules {
			t.Run(mod.EffectiveName(), func(t *testing.T) {
				g := NewWithT(t)
				ctx := t.Context()
				moduleName := mod.EffectiveName()
				g.Eventually(ctx, suite.k.Get(support.PlatformOperator(moduleName))).Should(
					SatisfyAll(
						support.HaveTrackedResourceKind("ServiceAccount"),
						support.HaveTrackedResourceKind("CustomResourceDefinition"),
					),
				)
			})
		}
	})

	t.Run("all modules should report chart info", func(t *testing.T) {
		for _, mod := range suite.modules {
			t.Run(mod.EffectiveName(), func(t *testing.T) {
				g := NewWithT(t)
				ctx := t.Context()
				moduleName := mod.EffectiveName()
				g.Eventually(ctx, suite.k.Get(support.PlatformOperator(moduleName))).Should(
					support.HaveChartInfo("test-module", mod.ChartPath),
				)
			})
		}
	})

	t.Run("all modules should report runlevel", func(t *testing.T) {
		for _, mod := range suite.modules {
			t.Run(mod.EffectiveName(), func(t *testing.T) {
				g := NewWithT(t)
				ctx := t.Context()
				moduleName := mod.EffectiveName()
				g.Eventually(ctx, suite.k.Get(support.PlatformOperator(moduleName))).Should(
					support.HaveRunlevel(mod.Runlevel),
				)
			})
		}
	})

	t.Run("each module should have its own CRD", func(t *testing.T) {
		for _, mod := range suite.modules {
			crdName := fmt.Sprintf("%ss.%s", mod.EffectiveName(), mod.GVK.Group)
			t.Run(mod.EffectiveName(), func(t *testing.T) {
				g := NewWithT(t)
				ctx := t.Context()
				moduleName := mod.EffectiveName()
				g.Eventually(ctx, suite.k.Get(support.PlatformOperator(moduleName))).Should(
					support.HaveTrackedResource("CustomResourceDefinition", crdName),
				)
			})
		}
	})

	t.Run("deployed resources should have part-of label", func(t *testing.T) {
		for _, mod := range suite.modules {
			t.Run(mod.EffectiveName(), func(t *testing.T) {
				g := NewWithT(t)
				ctx := t.Context()
				moduleName := mod.EffectiveName()
				sa := &corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{
					Name: moduleName, Namespace: mod.Namespace,
				}}
				g.Eventually(ctx, suite.k.Get(sa)).Should(
					k8sm.HasLabel("platform.opendatahub.io/part-of", moduleName),
				)
			})
		}
	})

	t.Run("deployed resources should have owner reference", func(t *testing.T) {
		for _, mod := range suite.modules {
			t.Run(mod.EffectiveName(), func(t *testing.T) {
				g := NewWithT(t)
				ctx := t.Context()
				moduleName := mod.EffectiveName()
				sa := &corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{
					Name: moduleName, Namespace: mod.Namespace,
				}}
				g.Eventually(ctx, suite.k.Get(sa)).Should(
					k8sm.IsControlledBy(support.PlatformOperatorOwner(moduleName)),
				)
			})
		}
	})

	t.Run("config values should be projected to configmap", func(t *testing.T) {
		for _, mod := range suite.modules {
			t.Run(mod.EffectiveName(), func(t *testing.T) {
				g := NewWithT(t)
				ctx := t.Context()
				moduleName := mod.EffectiveName()
				cm := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{
					Name: moduleName + "-config", Namespace: mod.Namespace,
				}}
				g.Eventually(ctx, suite.k.Get(cm)).Should(
					WithTransform(jq.Extract(`.data`), SatisfyAll(
						HaveKeyWithValue("module-name", Equal(moduleName)),
						HaveKeyWithValue("distribution.name", Not(BeEmpty())),
						HaveKeyWithValue("distribution.version", Not(BeEmpty())),
					)),
				)
			})
		}
	})

	t.Run("PlatformOperator should be owned by Platform", func(t *testing.T) {
		for _, mod := range suite.modules {
			t.Run(mod.EffectiveName(), func(t *testing.T) {
				g := NewWithT(t)
				ctx := t.Context()
				moduleName := mod.EffectiveName()
				g.Eventually(ctx, suite.k.Get(support.PlatformOperator(moduleName))).Should(
					k8sm.IsControlledBy(support.PlatformOwner()),
				)
			})
		}
	})
}

func (ft *foundationTests) testVersionPropagation(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()
	suite := ft.newSuite(t)
	suite.setupTest(t)

	p := newPlatformWithModules(suite.platformModuleNames())
	g.Expect(suite.client.Create(ctx, p)).To(Succeed())

	for _, mod := range suite.modules {
		moduleName := mod.EffectiveName()
		g.Eventually(ctx, suite.k.Get(support.PlatformOperator(moduleName))).Should(
			support.HaveTrackedResources(),
		)
	}

	mod := suite.modules[0]

	upsertModuleCRWithVersion(t, suite, mod.GVK, "test-version")

	g.Eventually(ctx, suite.k.Get(support.PlatformOperator(mod.EffectiveName()))).Should(
		support.HaveDistributionVersion("test-version"),
	)
}

func (ft *foundationTests) testDisableModules(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()
	suite := ft.newSuite(t)
	suite.setupTest(t)

	p := newPlatformWithModules(suite.platformModuleNames())
	g.Expect(suite.client.Create(ctx, p)).To(Succeed())

	for _, mod := range suite.modules {
		moduleName := mod.EffectiveName()
		g.Eventually(ctx, suite.k.Get(support.PlatformOperator(moduleName))).Should(
			support.HaveTrackedResources(),
		)
	}

	// Snapshot deployed resources.
	type moduleResources struct {
		name      string
		resources []configApi.ResourceRef
	}

	snapshots := make([]moduleResources, 0, len(suite.modules))
	for _, mod := range suite.modules {
		po := &configApi.PlatformOperator{}
		g.Expect(suite.client.Get(ctx, client.ObjectKey{Name: mod.EffectiveName()}, po)).To(Succeed())
		snapshots = append(snapshots, moduleResources{
			name:      mod.EffectiveName(),
			resources: po.Status.Resources,
		})
	}

	// Remove all modules.
	g.Eventually(ctx, k8sm.Update(suite.k, support.Platform(), func(p *configApi.Platform) {
		p.Spec.Modules = nil
	})).Should(
		jq.Match(`(.spec.modules // []) | length == 0`),
	)

	g.Eventually(ctx, suite.k.List(&configApi.PlatformOperatorList{})).Should(
		jq.Match(`length == 0`),
	)

	for _, snap := range snapshots {
		t.Run(snap.name, func(t *testing.T) {
			g := NewWithT(t)
			ctx := t.Context()
			for _, ref := range snap.resources {
				g.Eventually(ctx, func() error {
					return suite.checkResourceResetState(ctx, ref)
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
