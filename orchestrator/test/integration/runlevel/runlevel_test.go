//go:build integration

package runlevel

import (
	"os"
	"testing"

	. "github.com/onsi/gomega"

	isupport "github.com/lburgazzoli/opendatahub-module-operator/orchestrator/test/integration/support"
	testsupport "github.com/lburgazzoli/opendatahub-module-operator/orchestrator/test/support"
)

func TestMain(m *testing.M) {
	os.Exit(isupport.Run(m, isupport.RunConfig{
		Modules:        runlevelModules(),
		CleanupModules: runlevelCleanupModules(),
	}))
}

func TestRunlevel(t *testing.T) {
	t.Run("upgrade triggered by version mismatch", testUpgradeTriggered)
	t.Run("wrong version does not advance", testWrongVersionDoesNotAdvance)
	t.Run("correct version advances runlevel", testCorrectVersionAdvances)
	t.Run("all modules ready sets distribution version", testAllModulesReady)
}

func testUpgradeTriggered(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()
	suite := isupport.NewSuite(t)
	prepareUpgradeScenario(t, suite)

	for _, name := range runlevelTwoModuleNames {
		g.Eventually(ctx, suite.K.Get(testsupport.PlatformOperator(name))).Should(
			testsupport.HaveNoTrackedResources(),
		)
	}
}

func testWrongVersionDoesNotAdvance(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()
	suite := isupport.NewSuite(t)
	prepareUpgradeScenario(t, suite)

	isupport.UpsertModuleCRWithVersion(t, suite, alphaGVK, wrongDistributionVersion)

	g.Eventually(ctx, suite.K.Get(testsupport.PlatformOperator(alphaModuleName))).Should(
		testsupport.HaveDistributionVersion(wrongDistributionVersion),
	)

	g.Consistently(ctx, suite.K.Get(testsupport.Platform())).
		WithTimeout(runlevelStabilityTimeout).
		Should(testsupport.HaveRunlevel(1))
}

func testCorrectVersionAdvances(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()
	suite := isupport.NewSuite(t)
	prepareUpgradeScenario(t, suite)

	isupport.UpsertModuleCRWithVersion(t, suite, alphaGVK, upgradedDistributionVersion)

	g.Eventually(ctx, suite.K.Get(testsupport.Platform())).Should(testsupport.HaveRunlevel(2))

	for _, name := range runlevelTwoModuleNames {
		g.Eventually(ctx, suite.K.Get(testsupport.PlatformOperator(name))).Should(
			testsupport.HaveTrackedResources(),
		)
	}
}

func testAllModulesReady(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()
	suite := isupport.NewSuite(t)
	prepareUpgradeScenario(t, suite)

	isupport.UpsertModuleCRWithVersion(t, suite, alphaGVK, upgradedDistributionVersion)

	for _, name := range runlevelTwoModuleNames {
		g.Eventually(ctx, suite.K.Get(testsupport.PlatformOperator(name))).Should(
			testsupport.HaveTrackedResources(),
		)
	}

	isupport.UpsertModuleCRWithVersion(t, suite, betaGVK, upgradedDistributionVersion)
	isupport.UpsertModuleCRWithVersion(t, suite, gammaGVK, upgradedDistributionVersion)

	g.Eventually(ctx, suite.K.Get(testsupport.Platform())).Should(
		testsupport.HaveDistributionVersion(upgradedDistributionVersion),
	)
}
