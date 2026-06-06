//go:build integration

package integration

import (
	"testing"

	. "github.com/onsi/gomega"

	"github.com/lburgazzoli/gomega-matchers/pkg/matchers/jq"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"

	configApi "github.com/lburgazzoli/opendatahub-module-operator/orchestrator/api/config/v1alpha1"
)

type runlevelTests struct {
	suite *orchestratorTest
}

func (rt *runlevelTests) Execute(t *testing.T) {
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

	t.Run("upgrade triggered by version mismatch", rt.testUpgradeTriggered)
	t.Run("wrong version does not advance", rt.testWrongVersionDoesNotAdvance)
	t.Run("correct version advances runlevel", rt.testCorrectVersionAdvances)
	t.Run("upgrade complete switches to reconcile", rt.testUpgradeComplete)
}

func (rt *runlevelTests) testUpgradeTriggered(t *testing.T) {
	g := NewWithT(t)

	// Create empty Platform CR, wait for it to reconcile in ModeReconcile.
	p := &configApi.Platform{
		ObjectMeta: metav1.ObjectMeta{Name: configApi.PlatformInstanceName},
	}

	_ = rt.suite.client.Delete(ctx, p)
	g.Eventually(rt.suite.k.Absent(p)).WithContext(ctx).Should(BeTrue())

	g.Expect(rt.suite.client.Create(ctx, p)).To(Succeed())

	g.Eventually(rt.suite.k.Get(p)).WithContext(ctx).Should(
		WithTransform(jq.Extract(`.status.version`), Not(BeEmpty())),
	)

	// Change PlatformVersion to create a version mismatch, then enable
	// modules. The controller detects the mismatch and enters ModeUpgrade.
	rt.suite.orchestrator.Config().PlatformVersion = "2.0.0"

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

	// Platform should enter upgrade mode at runlevel 1.
	g.Eventually(func(g Gomega) {
		fresh := &configApi.Platform{}
		g.Expect(rt.suite.client.Get(ctx, client.ObjectKey{Name: configApi.PlatformInstanceName}, fresh)).To(Succeed())
		rl := -1
		if fresh.Status.CurrentRunlevel != nil {
			rl = *fresh.Status.CurrentRunlevel
		}
		t.Logf("Platform: mode=%q version=%q currentRunlevel=%d", fresh.Status.Mode, fresh.Status.Version, rl)
		g.Expect(string(fresh.Status.Mode)).To(Equal("upgrade"))
		g.Expect(fresh.Status.CurrentRunlevel).NotTo(BeNil())
		g.Expect(*fresh.Status.CurrentRunlevel).To(Equal(1))
	}).WithContext(ctx).Should(Succeed())

	// Alpha (runlevel 1) should deploy resources.
	g.Eventually(func(g Gomega) {
		po := &configApi.PlatformOperator{}
		g.Expect(rt.suite.client.Get(ctx, client.ObjectKey{Name: "alpha"}, po)).To(Succeed())
		t.Logf("Alpha PO: resources=%d deployedVersion=%q", len(po.Status.Resources), po.Status.DeployedVersion)
		g.Expect(po.Status.Resources).NotTo(BeEmpty())
	}).WithContext(ctx).Should(Succeed())

	// Beta and gamma (runlevel 2) should exist but have no resources.
	for _, name := range []string{"beta", "gamma"} {
		g.Eventually(func(g Gomega) {
			po := &configApi.PlatformOperator{}
			g.Expect(rt.suite.client.Get(ctx, client.ObjectKey{Name: name}, po)).To(Succeed())
			t.Logf("%s PO: resources=%d", name, len(po.Status.Resources))
			g.Expect(po.Status.Resources).To(BeEmpty())
		}).WithContext(ctx).Should(Succeed())
	}
}

func (rt *runlevelTests) testWrongVersionDoesNotAdvance(t *testing.T) {
	g := NewWithT(t)

	// Create alpha module CR with wrong version.
	alphaCR := &unstructured.Unstructured{}
	alphaCR.SetGroupVersionKind(alphaGVK)
	alphaCR.SetName("default")

	g.Expect(rt.suite.client.Create(ctx, alphaCR)).To(Succeed())

	// Set wrong version in status.
	g.Eventually(func(g Gomega) {
		g.Expect(rt.suite.client.Get(ctx, client.ObjectKeyFromObject(alphaCR), alphaCR)).To(Succeed())
		_ = unstructured.SetNestedField(alphaCR.Object, "wrong-version", "status", "release", "version")
		g.Expect(rt.suite.client.Status().Update(ctx, alphaCR)).To(Succeed())
	}).WithContext(ctx).Should(Succeed())

	// Wait for the module reconciler to pick up the version.
	alphaPO := &configApi.PlatformOperator{ObjectMeta: metav1.ObjectMeta{Name: "alpha"}}
	g.Eventually(rt.suite.k.Get(alphaPO)).WithContext(ctx).Should(
		WithTransform(jq.Extract(`.status.deployedVersion`), Equal("wrong-version")),
	)

	// Platform should NOT advance — still at runlevel 1.
	p := &configApi.Platform{ObjectMeta: metav1.ObjectMeta{Name: configApi.PlatformInstanceName}}
	g.Consistently(rt.suite.k.Get(p)).WithContext(ctx).WithTimeout(timeout / 3).Should(
		WithTransform(jq.Extract(`.status.currentRunlevel`), BeEquivalentTo(1)),
	)

	// Beta/gamma should still have no resources.
	for _, name := range []string{"beta", "gamma"} {
		po := &configApi.PlatformOperator{ObjectMeta: metav1.ObjectMeta{Name: name}}
		g.Eventually(rt.suite.k.Get(po)).WithContext(ctx).Should(
			WithTransform(jq.Extract(`.status.resources`), BeEmpty()),
		)
	}
}

func (rt *runlevelTests) testCorrectVersionAdvances(t *testing.T) {
	g := NewWithT(t)

	// Update alpha module CR to the correct version.
	alphaCR := &unstructured.Unstructured{}
	alphaCR.SetGroupVersionKind(alphaGVK)
	alphaCR.SetName("default")

	g.Eventually(func(g Gomega) {
		g.Expect(rt.suite.client.Get(ctx, client.ObjectKeyFromObject(alphaCR), alphaCR)).To(Succeed())
		_ = unstructured.SetNestedField(alphaCR.Object, "2.0.0", "status", "release", "version")
		g.Expect(rt.suite.client.Status().Update(ctx, alphaCR)).To(Succeed())
	}).WithContext(ctx).Should(Succeed())

	// Platform should advance to runlevel 2.
	p := &configApi.Platform{ObjectMeta: metav1.ObjectMeta{Name: configApi.PlatformInstanceName}}
	g.Eventually(rt.suite.k.Get(p)).WithContext(ctx).Should(
		WithTransform(jq.Extract(`.status.currentRunlevel`), BeEquivalentTo(2)),
	)

	// Beta and gamma should now deploy resources.
	for _, name := range []string{"beta", "gamma"} {
		po := &configApi.PlatformOperator{ObjectMeta: metav1.ObjectMeta{Name: name}}
		g.Eventually(rt.suite.k.Get(po)).WithContext(ctx).Should(
			WithTransform(jq.Extract(`.status.resources`), Not(BeEmpty())),
		)
	}
}

func (rt *runlevelTests) testUpgradeComplete(t *testing.T) {
	g := NewWithT(t)

	// Create beta and gamma module CRs with correct version.
	for _, gvk := range []interface{ WithKind(string) interface{} }{} {
		_ = gvk
	}

	for _, mod := range rt.suite.modules {
		if mod.Runlevel != 2 {
			continue
		}

		cr := &unstructured.Unstructured{}
		cr.SetGroupVersionKind(mod.GVK)
		cr.SetName("default")

		g.Expect(rt.suite.client.Create(ctx, cr)).To(Succeed())

		g.Eventually(func(g Gomega) {
			g.Expect(rt.suite.client.Get(ctx, client.ObjectKeyFromObject(cr), cr)).To(Succeed())
			_ = unstructured.SetNestedField(cr.Object, "2.0.0", "status", "release", "version")
			g.Expect(rt.suite.client.Status().Update(ctx, cr)).To(Succeed())
		}).WithContext(ctx).Should(Succeed())
	}

	// Platform should switch to reconcile mode.
	p := &configApi.Platform{ObjectMeta: metav1.ObjectMeta{Name: configApi.PlatformInstanceName}}
	g.Eventually(rt.suite.k.Get(p)).WithContext(ctx).Should(And(
		WithTransform(jq.Extract(`.status.mode`), Equal("reconcile")),
		WithTransform(jq.Extract(`.status.currentRunlevel`), BeNil()),
		WithTransform(jq.Extract(`.status.version`), Equal("2.0.0")),
	))
}
