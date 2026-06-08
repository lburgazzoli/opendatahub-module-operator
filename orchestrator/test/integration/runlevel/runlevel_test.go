//go:build integration

package runlevel

import (
	"os"
	"testing"

	. "github.com/onsi/gomega"

	isupport "github.com/lburgazzoli/opendatahub-module-operator/orchestrator/test/integration/support"
	testsupport "github.com/lburgazzoli/opendatahub-module-operator/orchestrator/test/support"
)

type runlevelTests struct {
}

func TestMain(m *testing.M) {
	os.Exit(isupport.Run(m, isupport.RunConfig{
		Modules:        runlevelModules(),
		CleanupModules: runlevelCleanupModules(),
	}))
}

func TestRunlevel(t *testing.T) {
	runlevels := &runlevelTests{}
	runlevels.Execute(t)
}

func (rt *runlevelTests) Execute(t *testing.T) {
	t.Run("upgrade triggered by version mismatch", rt.testUpgradeTriggered)
	t.Run("wrong version does not advance", rt.testWrongVersionDoesNotAdvance)
	t.Run("correct version advances runlevel", rt.testCorrectVersionAdvances)
	t.Run("all modules ready sets distribution version", rt.testAllModulesReady)
}

func (rt *runlevelTests) testUpgradeTriggered(t *testing.T) {
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

func (rt *runlevelTests) testWrongVersionDoesNotAdvance(t *testing.T) {
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

func (rt *runlevelTests) testCorrectVersionAdvances(t *testing.T) {
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

func (rt *runlevelTests) testAllModulesReady(t *testing.T) {
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
