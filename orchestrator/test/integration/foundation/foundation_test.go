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
	suite, err := isupport.Setup(isupport.RunConfig{
		Modules:        foundationModules(),
		CleanupModules: foundationCleanupModules(),
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to setup integration suite: %v\n", err)
		os.Exit(1)
	}

	code := suite.Run(m)
	if err := suite.TearDown(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to teardown integration suite: %v\n", err)
		if code == 0 {
			code = 1
		}
	}

	os.Exit(code)
}

func TestEmptyPlatformReconciles(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()
	suite := isupport.NewSuite(t)
	suite.SetupTest(t)

	p := &configApi.Platform{
		ObjectMeta: metav1.ObjectMeta{Name: configApi.PlatformInstanceName},
	}

	g.Expect(suite.Client.Create(ctx, p)).To(Succeed())

	g.Eventually(ctx, suite.K.Get(p)).Should(
		jq.Match(`(.status.distribution.current.version // "") | length > 0`),
	)
	g.Eventually(ctx, suite.K.List(&configApi.PlatformOperatorList{})).Should(
		k8sm.IsEmptyList(),
	)
}

func TestModuleDeployment(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()
	suite := isupport.NewSuite(t)
	suite.SetupTest(t)

	p := isupport.NewPlatformWithModules(suite.PlatformModuleNames())
	g.Expect(suite.Client.Create(ctx, p)).To(Succeed())

	for _, mod := range suite.Modules {
		moduleName := mod.Name
		g.Eventually(ctx, suite.K.Get(testsupport.PlatformOperator(moduleName))).Should(
			testsupport.HaveTrackedResources(),
		)
	}

	t.Run("all modules should track resources in status", func(t *testing.T) {
		for _, mod := range suite.Modules {
			t.Run(mod.Name, func(t *testing.T) {
				g := NewWithT(t)
				ctx := t.Context()
				moduleName := mod.Name
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
			t.Run(mod.Name, func(t *testing.T) {
				g := NewWithT(t)
				ctx := t.Context()
				moduleName := mod.Name
				g.Eventually(ctx, suite.K.Get(testsupport.PlatformOperator(moduleName))).Should(
					testsupport.HaveChartInfo("test-module", mod.Manifests.Chart.Path),
				)
			})
		}
	})

	t.Run("all modules should report runlevel", func(t *testing.T) {
		for _, mod := range suite.Modules {
			t.Run(mod.Name, func(t *testing.T) {
				g := NewWithT(t)
				ctx := t.Context()
				moduleName := mod.Name
				g.Eventually(ctx, suite.K.Get(testsupport.PlatformOperator(moduleName))).Should(
					testsupport.HaveRunlevel(mod.Runlevel),
				)
			})
		}
	})

	t.Run("each module should have its own CRD", func(t *testing.T) {
		for _, mod := range suite.Modules {
			crdName := fmt.Sprintf("%ss.%s", mod.Name, mod.GVK.Group)
			t.Run(mod.Name, func(t *testing.T) {
				g := NewWithT(t)
				ctx := t.Context()
				moduleName := mod.Name
				g.Eventually(ctx, suite.K.Get(testsupport.PlatformOperator(moduleName))).Should(
					testsupport.HaveTrackedNamedResource(gvk.CustomResourceDefinition, crdName),
				)
			})
		}
	})

	t.Run("deployed resources should have part-of label", func(t *testing.T) {
		for _, mod := range suite.Modules {
			t.Run(mod.Name, func(t *testing.T) {
				g := NewWithT(t)
				ctx := t.Context()
				moduleName := mod.Name
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
			t.Run(mod.Name, func(t *testing.T) {
				g := NewWithT(t)
				ctx := t.Context()
				moduleName := mod.Name
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
			t.Run(mod.Name, func(t *testing.T) {
				g := NewWithT(t)
				ctx := t.Context()
				moduleName := mod.Name
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
			t.Run(mod.Name, func(t *testing.T) {
				g := NewWithT(t)
				ctx := t.Context()
				moduleName := mod.Name
				g.Eventually(ctx, suite.K.Get(testsupport.PlatformOperator(moduleName))).Should(
					k8sm.IsControlledBy(testsupport.PlatformOwner()),
				)
			})
		}
	})
}

func TestModuleVersionPropagation(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()
	suite := isupport.NewSuite(t)
	suite.SetupTest(t)

	p := isupport.NewPlatformWithModules(suite.PlatformModuleNames())
	g.Expect(suite.Client.Create(ctx, p)).To(Succeed())

	for _, mod := range suite.Modules {
		moduleName := mod.Name
		g.Eventually(ctx, suite.K.Get(testsupport.PlatformOperator(moduleName))).Should(
			testsupport.HaveTrackedResources(),
		)
	}

	mod := suite.Modules[0]

	isupport.UpsertModuleCRWithVersion(t, suite, mod.GVK, "test-version")

	g.Eventually(ctx, suite.K.Get(testsupport.PlatformOperator(mod.Name))).Should(
		testsupport.HaveCurrentDistributionVersion("test-version"),
	)
}

func TestDisablingModulesCleansUpResources(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()
	suite := isupport.NewSuite(t)
	suite.SetupTest(t)

	p := isupport.NewPlatformWithModules(suite.PlatformModuleNames())
	g.Expect(suite.Client.Create(ctx, p)).To(Succeed())

	for _, mod := range suite.Modules {
		moduleName := mod.Name
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
		g.Expect(suite.Client.Get(ctx, client.ObjectKey{Name: mod.Name}, po)).To(Succeed())
		snapshots = append(snapshots, moduleResources{
			name:      mod.Name,
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
