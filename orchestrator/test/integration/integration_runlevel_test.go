//go:build integration

package integration

import (
	"testing"
	"time"

	. "github.com/onsi/gomega"

	"github.com/lburgazzoli/gomega-matchers/pkg/matchers/jq"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"

	configApi "github.com/lburgazzoli/opendatahub-module-operator/orchestrator/api/config/v1alpha1"
)

const runlevelStabilityTimeout = 2 * time.Second

type runlevelTests struct {
	newSuite suiteFactory
}

func (rt *runlevelTests) Execute(t *testing.T) {
	t.Run("upgrade triggered by version mismatch", rt.testUpgradeTriggered)
	t.Run("wrong version does not advance", rt.testWrongVersionDoesNotAdvance)
	t.Run("correct version advances runlevel", rt.testCorrectVersionAdvances)
	t.Run("all modules ready sets distribution version", rt.testAllModulesReady)
}

// setupUpgradeScenario creates a Platform CR, waits for initial reconciliation,
// changes the distribution version to trigger an upgrade, and enables all modules.
// Returns a cleanup function.
func (rt *runlevelTests) setupUpgradeScenario(t *testing.T, g Gomega, suite *orchestratorTest) {
	t.Helper()

	suite.setupTest(t)
	suite.setDistributionVersion("1.0.0")

	p := &configApi.Platform{
		ObjectMeta: metav1.ObjectMeta{Name: configApi.PlatformInstanceName},
	}

	g.Expect(suite.client.Create(t.Context(), p)).To(Succeed())

	g.Eventually(suite.k.Get(p)).WithContext(t.Context()).Should(
		WithTransform(jq.Extract(`.status.distribution.version`), Equal("1.0.0")),
	)

	// Change version to trigger upgrade.
	suite.setDistributionVersion("2.0.0")

	g.Eventually(func(g Gomega) {
		fresh := &configApi.Platform{}
		g.Expect(suite.client.Get(t.Context(), client.ObjectKey{Name: configApi.PlatformInstanceName}, fresh)).To(Succeed())
		fresh.Spec.Modules = suite.platformModuleNames()
		g.Expect(suite.client.Update(t.Context(), fresh)).To(Succeed())
	}).WithContext(t.Context()).Should(Succeed())

	// Wait for runlevel 1 to be set and alpha to deploy.
	g.Eventually(func(g Gomega) {
		fresh := &configApi.Platform{}
		g.Expect(suite.client.Get(t.Context(), client.ObjectKey{Name: configApi.PlatformInstanceName}, fresh)).To(Succeed())
		g.Expect(fresh.Status.Runlevel).To(Equal(1))
	}).WithContext(t.Context()).Should(Succeed())

	g.Eventually(func(g Gomega) {
		po := &configApi.PlatformOperator{}
		g.Expect(suite.client.Get(t.Context(), client.ObjectKey{Name: "alpha"}, po)).To(Succeed())
		g.Expect(po.Status.Resources).NotTo(BeEmpty())
	}).WithContext(t.Context()).Should(Succeed())
}

// createModuleCR creates an unstructured module CR using the dynamic client.
func (rt *runlevelTests) createModuleCR(
	t *testing.T,
	g Gomega,
	suite *orchestratorTest,
	gvk schema.GroupVersionKind,
	version string,
) {
	upsertModuleCRWithVersion(t, g, suite, gvk, version)
}

func (rt *runlevelTests) testUpgradeTriggered(t *testing.T) {
	g := NewWithT(t)
	suite := rt.newSuite(t)
	rt.setupUpgradeScenario(t, g, suite)

	// Beta/gamma (runlevel 2) should exist but have no resources.
	for _, name := range []string{"beta", "gamma"} {
		g.Eventually(func(g Gomega) {
			po := &configApi.PlatformOperator{}
			g.Expect(suite.client.Get(t.Context(), client.ObjectKey{Name: name}, po)).To(Succeed())
			g.Expect(po.Status.Resources).To(BeEmpty())
		}).WithContext(t.Context()).Should(Succeed())
	}
}

func (rt *runlevelTests) testWrongVersionDoesNotAdvance(t *testing.T) {
	g := NewWithT(t)
	suite := rt.newSuite(t)
	rt.setupUpgradeScenario(t, g, suite)

	rt.createModuleCR(t, g, suite, alphaGVK, "wrong-version")

	// Wait for the version to propagate to PlatformOperator.
	g.Eventually(suite.k.Get(&configApi.PlatformOperator{
		ObjectMeta: metav1.ObjectMeta{Name: "alpha"},
	})).WithContext(t.Context()).Should(
		WithTransform(jq.Extract(`.status.distribution.version`), Equal("wrong-version")),
	)

	// Platform should NOT advance past runlevel 1.
	g.Consistently(func(g Gomega) {
		fresh := &configApi.Platform{}
		g.Expect(suite.client.Get(t.Context(), client.ObjectKey{Name: configApi.PlatformInstanceName}, fresh)).To(Succeed())
		g.Expect(fresh.Status.Runlevel).To(Equal(1))
	}).WithContext(t.Context()).WithTimeout(runlevelStabilityTimeout).Should(Succeed())
}

func (rt *runlevelTests) testCorrectVersionAdvances(t *testing.T) {
	g := NewWithT(t)
	suite := rt.newSuite(t)
	rt.setupUpgradeScenario(t, g, suite)

	rt.createModuleCR(t, g, suite, alphaGVK, "2.0.0")

	// Platform should advance to runlevel 2.
	g.Eventually(func(g Gomega) {
		fresh := &configApi.Platform{}
		g.Expect(suite.client.Get(t.Context(), client.ObjectKey{Name: configApi.PlatformInstanceName}, fresh)).To(Succeed())
		g.Expect(fresh.Status.Runlevel).To(Equal(2))
	}).WithContext(t.Context()).Should(Succeed())

	// Beta and gamma should now deploy resources.
	for _, name := range []string{"beta", "gamma"} {
		g.Eventually(func(g Gomega) {
			po := &configApi.PlatformOperator{}
			g.Expect(suite.client.Get(t.Context(), client.ObjectKey{Name: name}, po)).To(Succeed())
			g.Expect(po.Status.Resources).NotTo(BeEmpty())
		}).WithContext(t.Context()).Should(Succeed())
	}
}

func (rt *runlevelTests) testAllModulesReady(t *testing.T) {
	g := NewWithT(t)
	suite := rt.newSuite(t)
	rt.setupUpgradeScenario(t, g, suite)

	// Set alpha's version to advance past runlevel 1.
	rt.createModuleCR(t, g, suite, alphaGVK, "2.0.0")

	// Wait for beta/gamma to deploy (runlevel 2 unlocked).
	for _, name := range []string{"beta", "gamma"} {
		g.Eventually(func(g Gomega) {
			po := &configApi.PlatformOperator{}
			g.Expect(suite.client.Get(t.Context(), client.ObjectKey{Name: name}, po)).To(Succeed())
			g.Expect(po.Status.Resources).NotTo(BeEmpty())
		}).WithContext(t.Context()).Should(Succeed())
	}

	// Now set beta and gamma versions.
	for _, mod := range suite.modules {
		if mod.Runlevel != 2 {
			continue
		}
		rt.createModuleCR(t, g, suite, mod.GVK, "2.0.0")
	}

	// Platform should report distribution version 2.0.0.
	g.Eventually(suite.k.Get(&configApi.Platform{
		ObjectMeta: metav1.ObjectMeta{Name: configApi.PlatformInstanceName},
	})).WithContext(t.Context()).Should(
		WithTransform(jq.Extract(`.status.distribution.version`), Equal("2.0.0")),
	)
}
