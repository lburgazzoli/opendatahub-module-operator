//go:build integration

package runlevel

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/lburgazzoli/gomega-matchers/pkg/matchers/jq"
	k8sm "github.com/lburgazzoli/gomega-matchers/pkg/matchers/k8s"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"

	configApi "github.com/lburgazzoli/opendatahub-module-operator/orchestrator/api/config/v1alpha1"
	"github.com/lburgazzoli/opendatahub-module-operator/orchestrator/pkg/module"
	isupport "github.com/lburgazzoli/opendatahub-module-operator/orchestrator/test/integration/support"
	testsupport "github.com/lburgazzoli/opendatahub-module-operator/orchestrator/test/support"
)

const (
	runlevelStabilityTimeout    = 2 * time.Second
	initialDistributionVersion  = "1.0.0"
	upgradedDistributionVersion = "2.0.0"
	wrongDistributionVersion    = "wrong-version"
	alphaModuleName             = "alpha"
	betaModuleName              = "beta"
	gammaModuleName             = "gamma"
	gatedModuleName             = "gated"
)

var (
	componentsGV = schema.GroupVersion{
		Group:   "test.opendatahub.io",
		Version: "v1alpha1",
	}
	alphaGVK = componentsGV.WithKind("Alpha")
	betaGVK  = componentsGV.WithKind("Beta")
	gammaGVK = componentsGV.WithKind("Gamma")
	gatedGVK = componentsGV.WithKind("Gated")

	runlevelTwoModuleNames = []string{betaModuleName, gammaModuleName}
)

func runlevelModules() []*module.Module {
	return []*module.Module{
		newModule(alphaModuleName, alphaGVK, 1, map[string]any{"module-name": alphaModuleName}),
		newModule(betaModuleName, betaGVK, 2, map[string]any{"module-name": betaModuleName}),
		newModule(gammaModuleName, gammaGVK, 2, map[string]any{"module-name": gammaModuleName}),
	}
}

func runlevelCleanupModules() []*module.Module {
	return []*module.Module{
		newModule(alphaModuleName, alphaGVK, 1, nil),
		newModule(betaModuleName, betaGVK, 2, nil),
		newModule(gammaModuleName, gammaGVK, 2, nil),
		newModule(gatedModuleName, gatedGVK, 1, nil),
	}
}

func newModule(
	name string,
	gvk schema.GroupVersionKind,
	runlevel int,
	values map[string]any,
) *module.Module {
	mod := &module.Module{
		Name:      name,
		GVK:       gvk,
		Namespace: isupport.TestNamespace + "-" + strings.ToLower(gvk.Kind),
		Runlevel:  runlevel,
	}
	if values != nil {
		mod.Config = func(_ context.Context, _ client.Client) (map[string]any, error) {
			return values, nil
		}
	}

	return mod
}

func prepareUpgradeScenario(t *testing.T, suite *isupport.Suite) {
	t.Helper()
	ctx := t.Context()
	g := NewWithT(t)

	suite.SetupTest(t)
	suite.SetDistributionVersion(initialDistributionVersion)

	p := &configApi.Platform{
		ObjectMeta: metav1.ObjectMeta{Name: configApi.PlatformInstanceName},
	}

	g.Expect(suite.Client.Create(ctx, p)).To(Succeed())
	g.Eventually(ctx, suite.K.Get(p)).Should(testsupport.HaveDistributionVersion(initialDistributionVersion))

	suite.SetDistributionVersion(upgradedDistributionVersion)
	g.Eventually(ctx, k8sm.Update(suite.K, testsupport.Platform(), func(p *configApi.Platform) {
		p.Spec.Modules = suite.PlatformModuleNames()
	})).Should(
		WithTransform(jq.Extract(`.spec.modules`), ConsistOf(suite.PlatformModuleNames())),
	)

	g.Eventually(ctx, suite.K.Get(testsupport.Platform())).Should(testsupport.HaveRunlevel(1))
	g.Eventually(ctx, suite.K.Get(testsupport.PlatformOperator(alphaModuleName))).Should(
		testsupport.HaveTrackedResources(),
	)
}
