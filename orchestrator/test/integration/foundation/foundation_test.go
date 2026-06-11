package foundation

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/lburgazzoli/gomega-matchers/pkg/matchers/jq"
	k8sm "github.com/lburgazzoli/gomega-matchers/pkg/matchers/k8s"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/client"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	odhLabels "github.com/opendatahub-io/opendatahub-operator/v2/pkg/metadata/labels"

	configApi "github.com/lburgazzoli/opendatahub-module-operator/orchestrator/api/config/v1alpha1"
	"github.com/lburgazzoli/opendatahub-module-operator/orchestrator/internal/controller/platform"
	"github.com/lburgazzoli/opendatahub-module-operator/orchestrator/internal/controller/platformoperator"
	"github.com/lburgazzoli/opendatahub-module-operator/orchestrator/pkg/resources/gvk"
	isupport "github.com/lburgazzoli/opendatahub-module-operator/orchestrator/test/integration/support"
	testsupport "github.com/lburgazzoli/opendatahub-module-operator/orchestrator/test/support"
	testmetrics "github.com/lburgazzoli/opendatahub-module-operator/orchestrator/test/support/metrics"
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
		SatisfyAll(
			testsupport.HaveCurrentDistributionName(suite.Config.Distribution.Name),
			testsupport.HaveCurrentDistributionVersion(suite.Config.Distribution.Version),
			testsupport.HaveTargetDistributionName(suite.Config.Distribution.Name),
			testsupport.HaveTargetDistributionVersion(suite.Config.Distribution.Version),
		),
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
					testsupport.HaveTrackedNamedResource(gvk.Namespace, mod.Namespace),
					testsupport.HaveTrackedResource(gvk.ConfigMap),
					testsupport.HaveTrackedResource(gvk.Deployment),
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
					continue
				case gvk.ConfigMap:
					configDataMatcher := SatisfyAll(
						HaveKeyWithValue("module-name", Equal(mod.Name)),
						HaveKeyWithValue("distribution.name", Equal(suite.Config.Distribution.Name)),
						HaveKeyWithValue("distribution.version", Equal(suite.Config.Distribution.Version)),
					)
					if mod.Name == alphaModuleName {
						configDataMatcher = SatisfyAll(
							configDataMatcher,
							HaveKeyWithValue("test-key", Equal("test-value")),
						)
					}

					g.Eventually(ctx, k8sm.Get(suite.Client, obj)).Should(SatisfyAll(
						k8sm.HasNamespace(mod.Namespace),
						k8sm.HasLabel(odhLabels.PlatformPartOf, mod.Name),
						k8sm.IsControlledBy(testsupport.PlatformOperatorOwner(mod.Name)),
						WithTransform(k8sm.Data(), configDataMatcher),
					))
				default:
					g.Eventually(ctx, k8sm.Get(suite.Client, obj)).Should(SatisfyAll(
						k8sm.HasNamespace(mod.Namespace),
						k8sm.HasLabel(odhLabels.PlatformPartOf, mod.Name),
						k8sm.IsControlledBy(testsupport.PlatformOperatorOwner(mod.Name)),
					))
				}
			}
		})
	}
}

func TestMetricsAfterModuleDeployment(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()
	suite := isupport.NewSuite(t)
	suite.SetupTest(t)

	p := isupport.NewPlatformWithModules(suite.PlatformModuleNames())
	g.Expect(suite.Client.Create(ctx, p)).To(Succeed())

	for _, mod := range suite.Modules {
		g.Eventually(ctx, k8sm.Get(suite.Client, testsupport.PlatformOperator(mod.Name))).Should(
			testsupport.HaveTrackedResources(),
		)
	}

	t.Run("platform runlevel gauge is set", func(t *testing.T) {
		g := NewWithT(t)

		val, err := testmetrics.GaugeValue(platform.MetricPlatformRunlevel)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(val).To(BeNumerically(">=", 1))
	})

	t.Run("platform operator info gauge exists per module", func(t *testing.T) {
		g := NewWithT(t)

		for _, mod := range suite.Modules {
			val, err := testmetrics.GaugeVecValue(
				platformoperator.MetricPlatformOperatorInfo,
				prometheus.Labels{
					platformoperator.LabelName:           mod.Name,
					platformoperator.LabelRunlevel:       fmt.Sprintf("%d", mod.Runlevel),
					platformoperator.LabelCurrentVersion: suite.Config.Distribution.Version,
					platformoperator.LabelTargetVersion:  suite.Config.Distribution.Version,
				},
			)
			g.Expect(err).NotTo(HaveOccurred(), "module %s", mod.Name)
			g.Expect(val).To(Equal(float64(1)), "module %s", mod.Name)
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
		g.Eventually(ctx, k8sm.Get(suite.Client, testsupport.PlatformOperator(moduleName))).Should(
			testsupport.HaveTrackedResources(),
		)
	}

	mod := suite.ModuleByName(alphaModuleName)
	g.Expect(mod).NotTo(BeNil())

	isupport.UpsertModuleCRWithVersion(t, suite, mod.GVK, "test-version")

	g.Eventually(ctx, k8sm.Get(suite.Client, testsupport.PlatformOperator(mod.Name))).Should(
		SatisfyAll(
			testsupport.HaveCurrentDistributionName(suite.Config.Distribution.Name),
			testsupport.HaveCurrentDistributionVersion("test-version"),
			testsupport.HaveTargetDistributionName(suite.Config.Distribution.Name),
			testsupport.HaveTargetDistributionVersion(suite.Config.Distribution.Version),
		),
	)
}

func TestModuleWithoutReleaseDoesNotFallBackToTarget(t *testing.T) {
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

	mod := suite.ModuleByName(alphaModuleName)
	isupport.UpsertModuleCRWithVersion(t, suite, mod.GVK, "")

	g.Eventually(ctx, k8sm.Get(suite.Client, testsupport.PlatformOperator(mod.Name))).Should(
		SatisfyAll(
			testsupport.HaveEmptyCurrentDistribution(),
			testsupport.HaveTargetDistributionName(suite.Config.Distribution.Name),
			testsupport.HaveTargetDistributionVersion(suite.Config.Distribution.Version),
		),
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

func TestRemovingModuleCleansUpOnlyThatModule(t *testing.T) {
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

	removedModuleName := betaModuleName
	removedModule := suite.Modules[0]
	for _, mod := range suite.Modules {
		if mod.Name == removedModuleName {
			removedModule = mod
			break
		}
	}

	removedPO := &configApi.PlatformOperator{}
	g.Expect(suite.Client.Get(ctx, client.ObjectKey{Name: removedModuleName}, removedPO)).To(Succeed())
	removedResources := append([]configApi.ResourceRef(nil), removedPO.Status.Resources...)

	remainingModules := make([]string, 0, len(suite.Modules)-1)
	for _, mod := range suite.Modules {
		if mod.Name == removedModuleName {
			continue
		}
		remainingModules = append(remainingModules, mod.Name)
	}

	g.Eventually(ctx, k8sm.Update(suite.Client, testsupport.Platform(), func(p *configApi.Platform) {
		p.Spec.Modules = remainingModules
	})).Should(
		WithTransform(jq.Extract(`.spec.modules`), ConsistOf(remainingModules)),
	)

	g.Eventually(ctx, k8sm.NotFound(suite.Client, testsupport.PlatformOperator(removedModuleName))).Should(BeTrue())

	for _, mod := range suite.Modules {
		if mod.Name == removedModuleName {
			continue
		}

		moduleName := mod.Name
		g.Eventually(ctx, k8sm.Get(suite.Client, testsupport.PlatformOperator(moduleName))).Should(
			testsupport.HaveTrackedResources(),
		)
	}

	t.Run(removedModule.Name, func(t *testing.T) {
		g := NewWithT(t)

		for _, ref := range removedResources {
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

func TestConfigChangeTriggersDeploymentRollout(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()
	suite := isupport.NewSuite(t)
	suite.SetupTest(t)

	p := isupport.NewPlatformWithModules(suite.PlatformModuleNames())
	g.Expect(suite.Client.Create(ctx, p)).To(Succeed())

	for _, mod := range suite.Modules {
		g.Eventually(ctx, k8sm.Get(suite.Client, testsupport.PlatformOperator(mod.Name))).Should(
			testsupport.HaveTrackedResources(),
		)
	}

	alphaMod := suite.ModuleByName(alphaModuleName)
	g.Expect(alphaMod).NotTo(BeNil())

	alphaDeploy := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{
		Name: alphaModuleName, Namespace: alphaMod.Namespace,
	}}

	g.Eventually(ctx, k8sm.Lookup(suite.Client, alphaDeploy)).Should(Succeed())

	g.Expect(alphaDeploy.Spec.Template.Annotations).To(HaveKey("checksum/config"))
	g.Expect(alphaDeploy.Generation).To(BeNumerically(">", 0))

	sum := alphaDeploy.Spec.Template.Annotations["checksum/config"]
	gen := alphaDeploy.Generation

	g.Eventually(ctx, k8sm.Update(suite.Client, testsupport.Platform(),
		k8sm.SetAnnotation("test/config-value", "changed"),
	)).Should(
		k8sm.HasAnnotation("test/config-value", "changed"),
	)

	g.Eventually(ctx, k8sm.Get(suite.Client, alphaDeploy)).
		WithTimeout(isupport.Timeout).
		Should(And(
			jq.Matchf(`.spec.template.metadata.annotations["checksum/config"] != "%s"`, sum),
			jq.Matchf(`.metadata.generation > %d`, gen),
			jq.Match(`.status.observedGeneration == .metadata.generation`),
			jq.Match(`.status.updatedReplicas == .spec.replicas`),
			jq.Match(`.status.availableReplicas == .spec.replicas`),
		))
}
