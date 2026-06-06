//go:build integration

package integration

import (
	"fmt"
	"testing"

	. "github.com/onsi/gomega"

	"github.com/lburgazzoli/gomega-matchers/pkg/matchers/jq"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type foundationTests struct {
	suite *orchestratorTest
}

func (ft *foundationTests) Execute(t *testing.T) {
	t.Run("all modules should deploy resources", ft.testAllModulesDeploy)
	t.Run("all modules should track resources in status", ft.testAllModulesTrackResources)
	t.Run("all modules should report chart info", ft.testAllModulesReportChartInfo)
	t.Run("all modules should report runlevel", ft.testAllModulesReportRunlevel)
	t.Run("each module should have its own CRD", ft.testEachModuleHasOwnCRD)
	t.Run("deployed resources should have part-of label", ft.testPartOfLabel)
	t.Run("deployed resources should have owner reference", ft.testOwnerReference)
	t.Run("config values should be projected to configmap", ft.testConfigProjection)
}

func (ft *foundationTests) testAllModulesDeploy(t *testing.T) {
	g := NewWithT(t)

	for i, po := range ft.suite.pos {
		t.Run(ft.suite.modules[i].EffectiveName(), func(t *testing.T) {
			g.Eventually(k.Get(po)).WithContext(ctx).Should(
				jq.Match(`.status.resources | length > 0`),
			)
		})
	}
}

func (ft *foundationTests) testAllModulesTrackResources(t *testing.T) {
	g := NewWithT(t)

	for i, po := range ft.suite.pos {
		t.Run(ft.suite.modules[i].EffectiveName(), func(t *testing.T) {
			g.Eventually(k.Get(po)).WithContext(ctx).Should(And(
				jq.Match(`[.status.resources[] | select(.kind == "ServiceAccount")] | length > 0`),
				jq.Match(`[.status.resources[] | select(.kind == "CustomResourceDefinition")] | length > 0`),
			))
		})
	}
}

func (ft *foundationTests) testAllModulesReportChartInfo(t *testing.T) {
	g := NewWithT(t)

	for i, po := range ft.suite.pos {
		t.Run(ft.suite.modules[i].EffectiveName(), func(t *testing.T) {
			g.Eventually(k.Get(po)).WithContext(ctx).Should(And(
				jq.Match(`.status.chart.name == "test-module"`),
				jq.Match(`.status.chart.path != ""`),
			))
		})
	}
}

func (ft *foundationTests) testAllModulesReportRunlevel(t *testing.T) {
	g := NewWithT(t)

	for i, po := range ft.suite.pos {
		mod := ft.suite.modules[i]
		t.Run(mod.EffectiveName(), func(t *testing.T) {
			g.Eventually(k.Get(po)).WithContext(ctx).Should(
				jq.Match(`.status.runlevel == %d`, mod.Runlevel),
			)
		})
	}
}

func (ft *foundationTests) testEachModuleHasOwnCRD(t *testing.T) {
	g := NewWithT(t)

	for i, po := range ft.suite.pos {
		mod := ft.suite.modules[i]
		crdName := fmt.Sprintf("%ss.%s", mod.EffectiveName(), mod.GVK.Group)

		t.Run(mod.EffectiveName(), func(t *testing.T) {
			q := `[.status.resources[] | select(.kind == "CustomResourceDefinition" and .name == "%s")] | length > 0`
			g.Eventually(k.Get(po)).WithContext(ctx).Should(jq.Match(q, crdName))
		})
	}
}

func (ft *foundationTests) testPartOfLabel(t *testing.T) {
	g := NewWithT(t)

	for i := range ft.suite.pos {
		mod := ft.suite.modules[i]
		t.Run(mod.EffectiveName(), func(t *testing.T) {
			sa := &corev1.ServiceAccount{
				ObjectMeta: metav1.ObjectMeta{
					Name:      mod.EffectiveName(),
					Namespace: mod.Namespace,
				},
			}

			g.Eventually(k.Get(sa)).WithContext(ctx).Should(
				jq.Match(`.metadata.labels."platform.opendatahub.io/part-of" == "%s"`, mod.EffectiveName()),
			)
		})
	}
}

func (ft *foundationTests) testConfigProjection(t *testing.T) {
	g := NewWithT(t)

	// Alpha has a Configurable ext with platform-name and test-key.
	mod := ft.suite.modules[0]
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      mod.EffectiveName() + "-config",
			Namespace: mod.Namespace,
		},
	}

	g.Eventually(k.Get(cm)).WithContext(ctx).Should(And(
		jq.Match(`.data."platform-name" == "TestPlatform"`),
		jq.Match(`.data."test-key" == "test-value"`),
		jq.Match(`.data."distribution.name" != ""`),
		jq.Match(`.data."distribution.version" != ""`),
	))
}

func (ft *foundationTests) testOwnerReference(t *testing.T) {
	g := NewWithT(t)

	for i := range ft.suite.pos {
		mod := ft.suite.modules[i]
		t.Run(mod.EffectiveName(), func(t *testing.T) {
			sa := &corev1.ServiceAccount{
				ObjectMeta: metav1.ObjectMeta{
					Name:      mod.EffectiveName(),
					Namespace: mod.Namespace,
				},
			}

			g.Eventually(k.Get(sa)).WithContext(ctx).Should(
				jq.Match(`[.metadata.ownerReferences[] | select(.kind == "PlatformOperator")] | length > 0`),
			)
		})
	}
}
