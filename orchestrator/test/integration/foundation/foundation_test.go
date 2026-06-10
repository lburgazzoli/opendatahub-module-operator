package foundation

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/lburgazzoli/gomega-matchers/pkg/matchers/jq"
	k8sm "github.com/lburgazzoli/gomega-matchers/pkg/matchers/k8s"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	odhLabels "github.com/opendatahub-io/opendatahub-operator/v2/pkg/metadata/labels"
	"sigs.k8s.io/controller-runtime/pkg/client"

	configApi "github.com/lburgazzoli/opendatahub-module-operator/orchestrator/api/config/v1alpha1"
	"github.com/lburgazzoli/opendatahub-module-operator/orchestrator/pkg/resources/gvk"
	isupport "github.com/lburgazzoli/opendatahub-module-operator/orchestrator/test/integration/support"
	testsupport "github.com/lburgazzoli/opendatahub-module-operator/orchestrator/test/support"
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

	g.Eventually(ctx, k8sm.Get(suite.Client, p)).Should(
		jq.Match(`(.status.distribution.current.version // "") | length > 0`),
	)
	g.Eventually(ctx, k8sm.List(suite.Client, &configApi.PlatformOperatorList{})).Should(
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
		g.Eventually(ctx, k8sm.Get(suite.Client, testsupport.PlatformOperator(moduleName))).Should(
			testsupport.HaveTrackedResources(),
		)
	}

	for _, mod := range suite.Modules {
		t.Run(mod.Name, func(t *testing.T) {
			g := NewWithT(t)
			ctx := t.Context()

			crdName := fmt.Sprintf("%ss.%s", mod.Name, mod.GVK.Group)

			g.Eventually(ctx, k8sm.Get(suite.Client, testsupport.PlatformOperator(mod.Name))).Should(
				SatisfyAll(
					testsupport.HaveTrackedResource(gvk.Namespace),
					testsupport.HaveTrackedResource(gvk.ServiceAccount),
					testsupport.HaveTrackedResource(gvk.CustomResourceDefinition),
					testsupport.HaveTrackedNamedResource(gvk.CustomResourceDefinition, crdName),
					testsupport.HaveChartInfo("test-module", mod.Manifests.Chart.Path),
					testsupport.HaveRunlevel(mod.Runlevel),
					k8sm.IsControlledBy(testsupport.PlatformOwner()),
				),
			)

			po := &configApi.PlatformOperator{}
			g.Expect(suite.Client.Get(ctx, client.ObjectKey{Name: mod.Name}, po)).To(Succeed())

			for _, ref := range po.Status.Resources {
				refGVK := schema.FromAPIVersionAndKind(ref.APIVersion, ref.Kind)
				obj := isupport.ObjectFromResourceRef(ref)

				switch refGVK {
				case gvk.Namespace, gvk.CustomResourceDefinition:
					g.Eventually(ctx, k8sm.Get(suite.Client, obj)).Should(
						k8sm.HasLabel(odhLabels.PlatformPartOf, mod.Name),
					)
				case gvk.ConfigMap:
					g.Eventually(ctx, k8sm.Get(suite.Client, obj)).Should(SatisfyAll(
						k8sm.HasLabel(odhLabels.PlatformPartOf, mod.Name),
						k8sm.IsControlledBy(testsupport.PlatformOperatorOwner(mod.Name)),
						WithTransform(k8sm.Data(), SatisfyAll(
							HaveKeyWithValue("module-name", Equal(mod.Name)),
							HaveKeyWithValue("distribution.name", Not(BeEmpty())),
							HaveKeyWithValue("distribution.version", Not(BeEmpty())),
						)),
					))
				default:
					g.Eventually(ctx, k8sm.Get(suite.Client, obj)).Should(SatisfyAll(
						k8sm.HasLabel(odhLabels.PlatformPartOf, mod.Name),
						k8sm.IsControlledBy(testsupport.PlatformOperatorOwner(mod.Name)),
					))
				}
			}
		})
	}
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
		g.Eventually(ctx, k8sm.Get(suite.Client, testsupport.PlatformOperator(moduleName))).Should(
			testsupport.HaveTrackedResources(),
		)
	}

	mod := suite.Modules[0]

	isupport.UpsertModuleCRWithVersion(t, suite, mod.GVK, "test-version")

	g.Eventually(ctx, k8sm.Get(suite.Client, testsupport.PlatformOperator(mod.Name))).Should(
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
		g.Eventually(ctx, k8sm.Get(suite.Client, testsupport.PlatformOperator(moduleName))).Should(
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

	g.Eventually(ctx, k8sm.Update(suite.Client, testsupport.Platform(), func(p *configApi.Platform) {
		p.Spec.Modules = nil
	})).Should(
		jq.Match(`(.spec.modules // []) | length == 0`),
	)

	g.Eventually(ctx, k8sm.List(suite.Client, &configApi.PlatformOperatorList{})).Should(
		k8sm.IsEmptyList(),
	)

	for _, snap := range snapshots {
		t.Run(snap.name, func(t *testing.T) {
			g := NewWithT(t)

			for _, ref := range snap.resources {
				g.Eventually(t.Context(), func(ctx context.Context) error {
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
