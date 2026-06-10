package runlevel

import (
	"fmt"
	"os"
	"testing"

	"github.com/lburgazzoli/gomega-matchers/pkg/matchers/jq"
	k8sm "github.com/lburgazzoli/gomega-matchers/pkg/matchers/k8s"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	k8serr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"

	configApi "github.com/lburgazzoli/opendatahub-module-operator/orchestrator/api/config/v1alpha1"
	isupport "github.com/lburgazzoli/opendatahub-module-operator/orchestrator/test/integration/support"
	testsupport "github.com/lburgazzoli/opendatahub-module-operator/orchestrator/test/support"
)

func TestMain(m *testing.M) {
	suite, err := isupport.Setup(isupport.RunConfig{
		Modules:        runlevelModules(),
		CleanupModules: runlevelCleanupModules(),
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

func TestUpgradeTriggeredByVersionMismatch(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()
	suite := isupport.NewSuite(t)
	prepareUpgradeScenario(t, suite)

	for _, name := range runlevelTwoModuleNames {
		g.Eventually(ctx, k8sm.Get(suite.Client, testsupport.PlatformOperator(name))).Should(
			testsupport.HaveNoTrackedResources(),
		)
	}
	g.Eventually(ctx, k8sm.Get(suite.Client, testsupport.Platform())).Should(
		WithTransform(k8sm.Conditions(), ContainElement(SatisfyAll(
			HaveKeyWithValue("type", configApi.ConditionUpToDate),
			HaveKeyWithValue("status", string(metav1.ConditionFalse)),
		))),
	)
	g.Eventually(ctx, k8sm.Get(suite.Client, testsupport.Platform())).Should(
		WithTransform(k8sm.Conditions(), ContainElement(SatisfyAll(
			HaveKeyWithValue("type", configApi.ConditionReady),
			HaveKeyWithValue("status", string(metav1.ConditionFalse)),
		))),
	)
	g.Eventually(ctx, k8sm.Events(suite.Client,
		k8sm.ForObject(corev1.ObjectReference{
			Kind: configApi.PlatformOperatorKind,
			Name: betaModuleName,
		}),
	)).Should(ContainElement(SatisfyAll(
		HaveField("Reason", Equal("RunlevelBlocked")),
		HaveField("Action", Equal("WaitForRunlevel")),
		HaveField("Message", ContainSubstring("waiting for runlevel 2 (current: 1)")),
	)))
}

func TestWrongVersionDoesNotAdvance(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()
	suite := isupport.NewSuite(t)
	prepareUpgradeScenario(t, suite)

	isupport.UpsertModuleCRWithVersion(t, suite, alphaGVK, wrongDistributionVersion)

	g.Eventually(ctx, k8sm.Get(suite.Client, testsupport.PlatformOperator(alphaModuleName))).Should(
		testsupport.HaveCurrentDistributionVersion(wrongDistributionVersion),
	)

	g.Consistently(ctx, k8sm.Get(suite.Client, testsupport.Platform())).
		WithTimeout(runlevelStabilityTimeout).
		Should(testsupport.HaveRunlevel(1))
}

func TestCorrectVersionAdvancesRunlevel(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()
	suite := isupport.NewSuite(t)
	prepareUpgradeScenario(t, suite)

	isupport.UpsertModuleCRWithVersion(t, suite, alphaGVK, upgradedDistributionVersion)

	g.Eventually(ctx, k8sm.Get(suite.Client, testsupport.Platform())).Should(testsupport.HaveRunlevel(2))

	for _, name := range runlevelTwoModuleNames {
		g.Eventually(ctx, k8sm.Get(suite.Client, testsupport.PlatformOperator(name))).Should(
			testsupport.HaveTrackedResources(),
		)
	}
}

func TestAllModulesReadySetsDistributionVersion(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()
	suite := isupport.NewSuite(t)
	prepareUpgradeScenario(t, suite)

	isupport.UpsertModuleCRWithVersion(t, suite, alphaGVK, upgradedDistributionVersion)

	for _, name := range runlevelTwoModuleNames {
		g.Eventually(ctx, k8sm.Get(suite.Client, testsupport.PlatformOperator(name))).Should(
			testsupport.HaveTrackedResources(),
		)
	}

	isupport.UpsertModuleCRWithVersion(t, suite, betaGVK, upgradedDistributionVersion)
	isupport.UpsertModuleCRWithVersion(t, suite, gammaGVK, upgradedDistributionVersion)

	g.Eventually(ctx, k8sm.Get(suite.Client, testsupport.Platform())).Should(
		testsupport.HaveCurrentDistributionVersion(upgradedDistributionVersion),
	)
	g.Eventually(ctx, k8sm.Get(suite.Client, testsupport.Platform())).Should(
		WithTransform(k8sm.Conditions(), ContainElement(SatisfyAll(
			HaveKeyWithValue("type", configApi.ConditionUpToDate),
			HaveKeyWithValue("status", string(metav1.ConditionTrue)),
		))),
	)
	g.Eventually(ctx, k8sm.Get(suite.Client, testsupport.Platform())).Should(
		WithTransform(k8sm.Conditions(), ContainElement(SatisfyAll(
			HaveKeyWithValue("type", configApi.ConditionReady),
			HaveKeyWithValue("status", string(metav1.ConditionTrue)),
		))),
	)
}

func TestAddingLowerRunlevelModuleDoesNotRewind(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()
	suite := isupport.NewSuite(t)
	prepareUpgradeScenario(t, suite)

	isupport.UpsertModuleCRWithVersion(t, suite, alphaGVK, upgradedDistributionVersion)

	g.Eventually(ctx, k8sm.Get(suite.Client, testsupport.Platform())).Should(testsupport.HaveRunlevel(2))

	modules := append(append([]string{}, initialRunlevelModuleNames...), deltaModuleName)
	g.Eventually(ctx, k8sm.Update(suite.Client, testsupport.Platform(), func(p *configApi.Platform) {
		p.Spec.Modules = modules
	})).Should(
		WithTransform(jq.Extract(`.spec.modules`), ConsistOf(modules)),
	)

	g.Eventually(ctx, k8sm.Get(suite.Client, testsupport.PlatformOperator(deltaModuleName))).
		WithTimeout(isupport.Timeout).
		Should(testsupport.HaveTrackedResources())
	g.Consistently(ctx, k8sm.Get(suite.Client, testsupport.Platform())).
		WithTimeout(runlevelStabilityTimeout).
		Should(testsupport.HaveRunlevel(2))
}

func TestAddingHigherRunlevelModuleWaitsForCurrentRunlevel(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()
	suite := isupport.NewSuite(t)
	prepareUpgradeScenario(t, suite)

	isupport.UpsertModuleCRWithVersion(t, suite, alphaGVK, upgradedDistributionVersion)

	g.Eventually(ctx, k8sm.Get(suite.Client, testsupport.Platform())).Should(testsupport.HaveRunlevel(2))

	modules := append(append([]string{}, initialRunlevelModuleNames...), epsilonModuleName)
	g.Eventually(ctx, k8sm.Update(suite.Client, testsupport.Platform(), func(p *configApi.Platform) {
		p.Spec.Modules = modules
	})).Should(
		WithTransform(jq.Extract(`.spec.modules`), ConsistOf(modules)),
	)

	g.Consistently(func() error {
		po := testsupport.PlatformOperator(epsilonModuleName)
		err := suite.Client.Get(ctx, client.ObjectKeyFromObject(po), po)
		switch {
		case k8serr.IsNotFound(err):
			return nil
		case err != nil:
			return err
		case len(po.Status.Resources) != 0:
			return fmt.Errorf("expected %q to have no tracked resources before runlevel 3", epsilonModuleName)
		default:
			return nil
		}
	}).WithContext(ctx).WithTimeout(runlevelStabilityTimeout).Should(Succeed())

	isupport.UpsertModuleCRWithVersion(t, suite, betaGVK, upgradedDistributionVersion)
	isupport.UpsertModuleCRWithVersion(t, suite, gammaGVK, upgradedDistributionVersion)

	g.Eventually(ctx, k8sm.Get(suite.Client, testsupport.Platform())).Should(testsupport.HaveRunlevel(3))
	g.Eventually(ctx, k8sm.Get(suite.Client, testsupport.PlatformOperator(epsilonModuleName))).Should(
		testsupport.HaveTrackedResources(),
	)
}

func TestAddingMiddleRunlevelModuleInSteadyStateReconcilesImmediately(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()
	suite := isupport.NewSuite(t)

	suite.SetupTest(t)
	suite.SetDistributionVersion(initialDistributionVersion)

	initialModules := []string{alphaModuleName, epsilonModuleName}
	p := isupport.NewPlatformWithModules(initialModules)
	g.Expect(suite.Client.Create(ctx, p)).To(Succeed())

	g.Eventually(ctx, k8sm.Get(suite.Client, testsupport.PlatformOperator(alphaModuleName))).Should(
		testsupport.HaveTrackedResources(),
	)
	g.Eventually(ctx, k8sm.Get(suite.Client, testsupport.PlatformOperator(epsilonModuleName))).Should(
		testsupport.HaveTrackedResources(),
	)
	isupport.UpsertModuleCRWithVersion(t, suite, alphaGVK, initialDistributionVersion)
	isupport.UpsertModuleCRWithVersion(t, suite, epsilonGVK, initialDistributionVersion)

	g.Eventually(ctx, k8sm.Get(suite.Client, testsupport.Platform())).Should(
		testsupport.HaveCurrentDistributionVersion(initialDistributionVersion),
	)
	g.Eventually(ctx, k8sm.Get(suite.Client, testsupport.Platform())).Should(testsupport.HaveRunlevel(3))

	modules := []string{alphaModuleName, betaModuleName, epsilonModuleName}
	g.Eventually(ctx, k8sm.Update(suite.Client, testsupport.Platform(), func(p *configApi.Platform) {
		p.Spec.Modules = modules
	})).Should(
		WithTransform(jq.Extract(`.spec.modules`), ConsistOf(modules)),
	)

	g.Eventually(ctx, k8sm.Get(suite.Client, testsupport.PlatformOperator(betaModuleName))).Should(
		testsupport.HaveTrackedResources(),
	)
	g.Consistently(ctx, k8sm.Get(suite.Client, testsupport.Platform())).
		WithTimeout(runlevelStabilityTimeout).
		Should(SatisfyAll(
			testsupport.HaveRunlevel(3),
			testsupport.HaveCurrentDistributionVersion(initialDistributionVersion),
		))
}

func TestSteadyStateIgnoresRunlevel(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()
	suite := isupport.NewSuite(t)
	prepareUpgradeScenario(t, suite)

	isupport.UpsertModuleCRWithVersion(t, suite, alphaGVK, upgradedDistributionVersion)

	for _, gvk := range []schema.GroupVersionKind{betaGVK, gammaGVK} {
		isupport.UpsertModuleCRWithVersion(t, suite, gvk, upgradedDistributionVersion)
	}

	g.Eventually(ctx, k8sm.Get(suite.Client, testsupport.Platform())).Should(
		testsupport.HaveCurrentDistributionVersion(upgradedDistributionVersion),
	)

	modules := append(append([]string{}, initialRunlevelModuleNames...), epsilonModuleName)
	g.Eventually(ctx, k8sm.Update(suite.Client, testsupport.Platform(), func(p *configApi.Platform) {
		p.Spec.Modules = modules
	})).Should(
		WithTransform(jq.Extract(`.spec.modules`), ConsistOf(modules)),
	)

	g.Eventually(ctx, k8sm.Get(suite.Client, testsupport.PlatformOperator(epsilonModuleName))).Should(
		testsupport.HaveTrackedResources(),
	)
}
