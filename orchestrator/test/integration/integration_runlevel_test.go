//go:build integration

package integration

import (
	"strings"
	"testing"

	. "github.com/onsi/gomega"

	"github.com/lburgazzoli/gomega-matchers/pkg/matchers/jq"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"

	configApi "github.com/lburgazzoli/opendatahub-module-operator/orchestrator/api/config/v1alpha1"
)

type runlevelTests struct {
	suite *orchestratorTest
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
func (rt *runlevelTests) setupUpgradeScenario(t *testing.T, g Gomega) {
	t.Helper()

	// Reset distribution version so initial reconciliation writes the old value.
	rt.suite.cfg.Distribution.Version = "1.0.0"

	p := &configApi.Platform{
		ObjectMeta: metav1.ObjectMeta{Name: configApi.PlatformInstanceName},
	}

	_ = rt.suite.client.Delete(ctx, p)
	g.Eventually(rt.suite.k.Absent(p)).WithContext(ctx).Should(BeTrue())

	// Clean up module CRs from previous tests.
	for _, mod := range rt.suite.modules {
		cr := &unstructured.Unstructured{}
		cr.SetGroupVersionKind(mod.GVK)
		cr.SetName("default")
		_ = rt.suite.client.Delete(ctx, cr)
	}

	g.Expect(rt.suite.client.Create(ctx, p)).To(Succeed())

	g.Eventually(rt.suite.k.Get(p)).WithContext(ctx).Should(
		WithTransform(jq.Extract(`.status.distribution.version`), Equal("1.0.0")),
	)

	// Change version to trigger upgrade.
	rt.suite.cfg.Distribution.Version = "2.0.0"

	moduleNames := make([]string, 0, len(rt.suite.modules))
	for _, mod := range rt.suite.modules {
		moduleNames = append(moduleNames, mod.EffectiveName())
	}

	g.Eventually(func(g Gomega) {
		fresh := &configApi.Platform{}
		g.Expect(rt.suite.client.Get(ctx, client.ObjectKey{Name: configApi.PlatformInstanceName}, fresh)).To(Succeed())
		fresh.Spec.Modules = moduleNames
		g.Expect(rt.suite.client.Update(ctx, fresh)).To(Succeed())
	}).WithContext(ctx).Should(Succeed())

	// Wait for runlevel 1 to be set and alpha to deploy.
	g.Eventually(func(g Gomega) {
		fresh := &configApi.Platform{}
		g.Expect(rt.suite.client.Get(ctx, client.ObjectKey{Name: configApi.PlatformInstanceName}, fresh)).To(Succeed())
		g.Expect(fresh.Status.Runlevel).To(Equal(1))
	}).WithContext(ctx).Should(Succeed())

	g.Eventually(func(g Gomega) {
		po := &configApi.PlatformOperator{}
		g.Expect(rt.suite.client.Get(ctx, client.ObjectKey{Name: "alpha"}, po)).To(Succeed())
		g.Expect(po.Status.Resources).NotTo(BeEmpty())
	}).WithContext(ctx).Should(Succeed())

	t.Cleanup(func() {
		_ = rt.suite.client.Delete(ctx, &configApi.Platform{
			ObjectMeta: metav1.ObjectMeta{Name: configApi.PlatformInstanceName},
		})
		for _, mod := range rt.suite.modules {
			obj := &unstructured.Unstructured{}
			obj.SetGroupVersionKind(mod.GVK)
			obj.SetName("default")
			_ = rt.suite.client.Delete(ctx, obj)
		}
	})
}

// createModuleCR creates an unstructured module CR using the dynamic client.
func (rt *runlevelTests) createModuleCR(g Gomega, gvk schema.GroupVersionKind, version string) {
	gvr := schema.GroupVersionResource{
		Group:    gvk.Group,
		Version:  gvk.Version,
		Resource: gvk.Kind + "s",
	}

	// Lowercase the resource name (CRD convention).
	gvr.Resource = strings.ToLower(gvr.Resource)

	cr := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": gvk.Group + "/" + gvk.Version,
			"kind":       gvk.Kind,
			"metadata": map[string]any{
				"name": "default",
			},
		},
	}

	g.Eventually(func() error {
		existing, err := rt.suite.dynamic.Resource(gvr).Get(ctx, "default", metav1.GetOptions{})
		if err == nil {
			cr.SetResourceVersion(existing.GetResourceVersion())
			_, err = rt.suite.dynamic.Resource(gvr).Update(ctx, cr, metav1.UpdateOptions{})
			return err
		}
		_, err = rt.suite.dynamic.Resource(gvr).Create(ctx, cr, metav1.CreateOptions{})
		return err
	}).WithContext(ctx).Should(Succeed())

	if version != "" {
		// Set the module CR's status.release.version.
		g.Eventually(func(g Gomega) {
			existing, err := rt.suite.dynamic.Resource(gvr).Get(ctx, "default", metav1.GetOptions{})
			g.Expect(err).To(Succeed())
			_ = unstructured.SetNestedField(existing.Object, version, "status", "release", "version")
			_, err = rt.suite.dynamic.Resource(gvr).UpdateStatus(ctx, existing, metav1.UpdateOptions{})
			g.Expect(err).To(Succeed())
		}).WithContext(ctx).Should(Succeed())

	}
}

func (rt *runlevelTests) testUpgradeTriggered(t *testing.T) {
	g := NewWithT(t)
	rt.setupUpgradeScenario(t, g)

	// Beta/gamma (runlevel 2) should exist but have no resources.
	for _, name := range []string{"beta", "gamma"} {
		g.Eventually(func(g Gomega) {
			po := &configApi.PlatformOperator{}
			g.Expect(rt.suite.client.Get(ctx, client.ObjectKey{Name: name}, po)).To(Succeed())
			g.Expect(po.Status.Resources).To(BeEmpty())
		}).WithContext(ctx).Should(Succeed())
	}
}

func (rt *runlevelTests) testWrongVersionDoesNotAdvance(t *testing.T) {
	g := NewWithT(t)
	rt.setupUpgradeScenario(t, g)

	rt.createModuleCR(g, alphaGVK, "wrong-version")

	// Wait for the version to propagate to PlatformOperator.
	g.Eventually(rt.suite.k.Get(&configApi.PlatformOperator{
		ObjectMeta: metav1.ObjectMeta{Name: "alpha"},
	})).WithContext(ctx).Should(
		WithTransform(jq.Extract(`.status.distribution.version`), Equal("wrong-version")),
	)

	// Platform should NOT advance past runlevel 1.
	g.Consistently(func(g Gomega) {
		fresh := &configApi.Platform{}
		g.Expect(rt.suite.client.Get(ctx, client.ObjectKey{Name: configApi.PlatformInstanceName}, fresh)).To(Succeed())
		g.Expect(fresh.Status.Runlevel).To(Equal(1))
	}).WithContext(ctx).WithTimeout(timeout / 3).Should(Succeed())
}

func (rt *runlevelTests) testCorrectVersionAdvances(t *testing.T) {
	g := NewWithT(t)
	rt.setupUpgradeScenario(t, g)

	rt.createModuleCR(g, alphaGVK, "2.0.0")

	// Platform should advance to runlevel 2.
	g.Eventually(func(g Gomega) {
		fresh := &configApi.Platform{}
		g.Expect(rt.suite.client.Get(ctx, client.ObjectKey{Name: configApi.PlatformInstanceName}, fresh)).To(Succeed())
		g.Expect(fresh.Status.Runlevel).To(Equal(2))
	}).WithContext(ctx).Should(Succeed())

	// Beta and gamma should now deploy resources.
	for _, name := range []string{"beta", "gamma"} {
		g.Eventually(func(g Gomega) {
			po := &configApi.PlatformOperator{}
			g.Expect(rt.suite.client.Get(ctx, client.ObjectKey{Name: name}, po)).To(Succeed())
			g.Expect(po.Status.Resources).NotTo(BeEmpty())
		}).WithContext(ctx).Should(Succeed())
	}
}

func (rt *runlevelTests) testAllModulesReady(t *testing.T) {
	g := NewWithT(t)
	rt.setupUpgradeScenario(t, g)

	// Set alpha's version to advance past runlevel 1.
	rt.createModuleCR(g, alphaGVK, "2.0.0")

	// Wait for beta/gamma to deploy (runlevel 2 unlocked).
	for _, name := range []string{"beta", "gamma"} {
		g.Eventually(func(g Gomega) {
			po := &configApi.PlatformOperator{}
			g.Expect(rt.suite.client.Get(ctx, client.ObjectKey{Name: name}, po)).To(Succeed())
			g.Expect(po.Status.Resources).NotTo(BeEmpty())
		}).WithContext(ctx).Should(Succeed())
	}

	// Now set beta and gamma versions.
	for _, mod := range rt.suite.modules {
		if mod.Runlevel != 2 {
			continue
		}
		rt.createModuleCR(g, mod.GVK, "2.0.0")
	}

	// Platform should report distribution version 2.0.0.
	g.Eventually(rt.suite.k.Get(&configApi.Platform{
		ObjectMeta: metav1.ObjectMeta{Name: configApi.PlatformInstanceName},
	})).WithContext(ctx).Should(
		WithTransform(jq.Extract(`.status.distribution.version`), Equal("2.0.0")),
	)
}
