//go:build integration

package integration

import (
	"testing"
	"time"

	"github.com/lburgazzoli/gomega-matchers/pkg/matchers/jq"
	k8sm "github.com/lburgazzoli/gomega-matchers/pkg/matchers/k8s"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	configApi "github.com/lburgazzoli/opendatahub-module-operator/orchestrator/api/config/v1alpha1"
	"github.com/lburgazzoli/opendatahub-module-operator/orchestrator/test/support"
)

const (
	runlevelStabilityTimeout    = 2 * time.Second
	initialDistributionVersion  = "1.0.0"
	upgradedDistributionVersion = "2.0.0"
	wrongDistributionVersion    = "wrong-version"
	alphaModuleName             = "alpha"
)

var runlevelTwoModuleNames = []string{"beta", "gamma"}

type runlevelTests struct {
	newSuite suiteFactory
}

func (rt *runlevelTests) Execute(t *testing.T) {
	t.Run("upgrade triggered by version mismatch", rt.testUpgradeTriggered)
	t.Run("wrong version does not advance", rt.testWrongVersionDoesNotAdvance)
	t.Run("correct version advances runlevel", rt.testCorrectVersionAdvances)
	t.Run("all modules ready sets distribution version", rt.testAllModulesReady)
}

// prepareUpgradeScenario starts from an empty cluster and moves the platform into
// the "upgrade pending at runlevel 1" state used by the runlevel tests:
// - the platform first reconciles at the current distribution version
// - the desired distribution version is bumped
// - all modules are enabled
// - only runlevel 1 modules are allowed to deploy resources
func (rt *runlevelTests) prepareUpgradeScenario(t *testing.T, suite *orchestratorTest) {
	t.Helper()
	ctx := t.Context()
	g := NewWithT(t)

	suite.setupTest(t)

	suite.setDistributionVersion(initialDistributionVersion)

	p := &configApi.Platform{
		ObjectMeta: metav1.ObjectMeta{Name: configApi.PlatformInstanceName},
	}

	g.Expect(suite.client.Create(ctx, p)).To(Succeed())
	g.Eventually(ctx, suite.k.Get(p)).Should(support.HaveDistributionVersion(initialDistributionVersion))

	// Change version to trigger upgrade.
	suite.setDistributionVersion(upgradedDistributionVersion)
	g.Eventually(ctx, k8sm.Update(suite.k, support.Platform(), func(p *configApi.Platform) {
		p.Spec.Modules = suite.platformModuleNames()
	})).Should(
		WithTransform(jq.Extract(`.spec.modules`), ConsistOf(suite.platformModuleNames())),
	)

	g.Eventually(ctx, suite.k.Get(support.Platform())).Should(
		support.HaveRunlevel(1),
	)
	g.Eventually(ctx, suite.k.Get(support.PlatformOperator(alphaModuleName))).Should(
		support.HaveTrackedResources(),
	)
}

func (rt *runlevelTests) testUpgradeTriggered(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()
	suite := rt.newSuite(t)
	rt.prepareUpgradeScenario(t, suite)

	for _, name := range runlevelTwoModuleNames {
		g.Eventually(ctx, suite.k.Get(support.PlatformOperator(name))).Should(
			support.HaveNoTrackedResources(),
		)
	}
}

func (rt *runlevelTests) testWrongVersionDoesNotAdvance(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()
	suite := rt.newSuite(t)
	rt.prepareUpgradeScenario(t, suite)

	upsertModuleCRWithVersion(t, suite, alphaGVK, wrongDistributionVersion)

	g.Eventually(ctx, suite.k.Get(support.PlatformOperator(alphaModuleName))).Should(
		support.HaveDistributionVersion(wrongDistributionVersion),
	)

	// Platform should NOT advance past runlevel 1.
	g.Consistently(ctx, suite.k.Get(support.Platform())).
		WithTimeout(runlevelStabilityTimeout).
		Should(support.HaveRunlevel(1))
}

func (rt *runlevelTests) testCorrectVersionAdvances(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()
	suite := rt.newSuite(t)
	rt.prepareUpgradeScenario(t, suite)

	upsertModuleCRWithVersion(t, suite, alphaGVK, upgradedDistributionVersion)

	g.Eventually(ctx, suite.k.Get(support.Platform())).Should(support.HaveRunlevel(2))

	for _, name := range runlevelTwoModuleNames {
		g.Eventually(ctx, suite.k.Get(support.PlatformOperator(name))).Should(
			support.HaveTrackedResources(),
		)
	}
}

func (rt *runlevelTests) testAllModulesReady(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()
	suite := rt.newSuite(t)
	rt.prepareUpgradeScenario(t, suite)

	// Set alpha's version to advance past runlevel 1.
	upsertModuleCRWithVersion(t, suite, alphaGVK, upgradedDistributionVersion)

	for _, name := range runlevelTwoModuleNames {
		g.Eventually(ctx, suite.k.Get(support.PlatformOperator(name))).Should(
			support.HaveTrackedResources(),
		)
	}

	upsertModuleCRWithVersion(t, suite, betaGVK, upgradedDistributionVersion)
	upsertModuleCRWithVersion(t, suite, gammaGVK, upgradedDistributionVersion)

	g.Eventually(ctx, suite.k.Get(support.Platform())).Should(
		support.HaveDistributionVersion(upgradedDistributionVersion),
	)
}
