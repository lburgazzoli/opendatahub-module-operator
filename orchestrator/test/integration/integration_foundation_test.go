//go:build integration

package integration

import (
	"fmt"
	"testing"

	. "github.com/onsi/gomega"

	"github.com/lburgazzoli/gomega-matchers/pkg/matchers/jq"

	corev1 "k8s.io/api/core/v1"
	k8serr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"

	configApi "github.com/lburgazzoli/opendatahub-module-operator/orchestrator/api/config/v1alpha1"
	"github.com/lburgazzoli/opendatahub-module-operator/orchestrator/pkg/resources/gvk"
)

type foundationTests struct {
	suite *orchestratorTest
}

func (ft *foundationTests) Execute(t *testing.T) {
	t.Cleanup(func() {
		_ = ft.suite.client.Delete(ctx, &configApi.Platform{
			ObjectMeta: metav1.ObjectMeta{Name: configApi.PlatformInstanceName},
		})
	})

	t.Run("empty platform should reconcile", ft.testEmptyPlatform)
	t.Run("enabling modules should deploy them", ft.testEnableModules)
	t.Run("all modules should track resources in status", ft.testAllModulesTrackResources)
	t.Run("all modules should report chart info", ft.testAllModulesReportChartInfo)
	t.Run("all modules should report runlevel", ft.testAllModulesReportRunlevel)
	t.Run("each module should have its own CRD", ft.testEachModuleHasOwnCRD)
	t.Run("deployed resources should have part-of label", ft.testPartOfLabel)
	t.Run("deployed resources should have owner reference", ft.testOwnerReference)
	t.Run("config values should be projected to configmap", ft.testConfigProjection)
	t.Run("disabling modules should clean up resources", ft.testDisableModules)
}

func (ft *foundationTests) testEmptyPlatform(t *testing.T) {
	g := NewWithT(t)

	p := &configApi.Platform{
		ObjectMeta: metav1.ObjectMeta{Name: configApi.PlatformInstanceName},
	}

	_ = ft.suite.client.Delete(ctx, p)

	g.Eventually(ft.suite.k.Absent(p)).WithContext(ctx).Should(BeTrue())

	g.Expect(ft.suite.client.Create(ctx, p)).To(Succeed())

	g.Eventually(ft.suite.k.Get(p)).WithContext(ctx).Should(
		WithTransform(jq.Extract(`.status.mode`), Equal("reconcile")),
	)

	g.Eventually(func(g Gomega) {
		var poList configApi.PlatformOperatorList
		g.Expect(ft.suite.client.List(ctx, &poList)).To(Succeed())
		g.Expect(poList.Items).To(BeEmpty())
	}).WithContext(ctx).Should(Succeed())
}

func (ft *foundationTests) testEnableModules(t *testing.T) {
	g := NewWithT(t)

	moduleNames := make([]string, 0, len(ft.suite.modules))
	for _, mod := range ft.suite.modules {
		moduleNames = append(moduleNames, mod.EffectiveName())
	}

	g.Eventually(func(g Gomega) {
		p := &configApi.Platform{}
		g.Expect(ft.suite.client.Get(ctx, client.ObjectKey{Name: configApi.PlatformInstanceName}, p)).To(Succeed())
		p.Spec.Modules = moduleNames
		g.Expect(ft.suite.client.Update(ctx, p)).To(Succeed())
	}).WithContext(ctx).Should(Succeed())

	for _, mod := range ft.suite.modules {
		po := &configApi.PlatformOperator{
			ObjectMeta: metav1.ObjectMeta{Name: mod.EffectiveName()},
		}

		g.Eventually(ft.suite.k.Get(po)).WithContext(ctx).Should(And(
			WithTransform(
				jq.Extract(`.status.resources`),
				Not(BeEmpty()),
			),
			WithTransform(
				jq.Extract(`.metadata.ownerReferences`),
				ContainElement(SatisfyAll(
					HaveKeyWithValue("apiVersion", configApi.GroupVersion.String()),
					HaveKeyWithValue("kind", configApi.PlatformKind),
					HaveKeyWithValue("name", configApi.PlatformInstanceName),
					HaveKeyWithValue("controller", true),
				)),
			),
		))
	}
}

func (ft *foundationTests) testAllModulesTrackResources(t *testing.T) {
	for _, mod := range ft.suite.modules {
		t.Run(mod.EffectiveName(), func(t *testing.T) {
			g := NewWithT(t)
			po := &configApi.PlatformOperator{
				ObjectMeta: metav1.ObjectMeta{Name: mod.EffectiveName()},
			}

			g.Eventually(ft.suite.k.Get(po)).WithContext(ctx).Should(And(
				WithTransform(
					jq.Extract(`[.status.resources[] | select(.kind == "ServiceAccount")]`),
					Not(BeEmpty()),
				),
				WithTransform(
					jq.Extract(`[.status.resources[] | select(.kind == "CustomResourceDefinition")]`),
					Not(BeEmpty()),
				),
			))
		})
	}
}

func (ft *foundationTests) testAllModulesReportChartInfo(t *testing.T) {
	for _, mod := range ft.suite.modules {
		t.Run(mod.EffectiveName(), func(t *testing.T) {
			g := NewWithT(t)
			po := &configApi.PlatformOperator{
				ObjectMeta: metav1.ObjectMeta{Name: mod.EffectiveName()},
			}

			g.Eventually(ft.suite.k.Get(po)).WithContext(ctx).Should(And(
				WithTransform(jq.Extract(`.status.chart.name`), Equal("test-module")),
				WithTransform(jq.Extract(`.status.chart.path`), Equal(mod.ChartPath)),
			))
		})
	}
}

func (ft *foundationTests) testAllModulesReportRunlevel(t *testing.T) {
	for _, mod := range ft.suite.modules {
		t.Run(mod.EffectiveName(), func(t *testing.T) {
			g := NewWithT(t)
			po := &configApi.PlatformOperator{
				ObjectMeta: metav1.ObjectMeta{Name: mod.EffectiveName()},
			}

			g.Eventually(ft.suite.k.Get(po)).WithContext(ctx).Should(
				WithTransform(jq.Extract(`.status.runlevel`), BeEquivalentTo(mod.Runlevel)),
			)
		})
	}
}

func (ft *foundationTests) testEachModuleHasOwnCRD(t *testing.T) {
	for _, mod := range ft.suite.modules {
		crdName := fmt.Sprintf("%ss.%s", mod.EffectiveName(), mod.GVK.Group)

		t.Run(mod.EffectiveName(), func(t *testing.T) {
			g := NewWithT(t)
			po := &configApi.PlatformOperator{
				ObjectMeta: metav1.ObjectMeta{Name: mod.EffectiveName()},
			}

			g.Eventually(ft.suite.k.Get(po)).WithContext(ctx).Should(
				WithTransform(
					jq.Extract(`.status.resources`),
					ContainElement(SatisfyAll(
						HaveKeyWithValue("kind", "CustomResourceDefinition"),
						HaveKeyWithValue("name", crdName),
					)),
				),
			)
		})
	}
}

func (ft *foundationTests) testPartOfLabel(t *testing.T) {
	for _, mod := range ft.suite.modules {
		t.Run(mod.EffectiveName(), func(t *testing.T) {
			g := NewWithT(t)
			sa := &corev1.ServiceAccount{
				ObjectMeta: metav1.ObjectMeta{
					Name:      mod.EffectiveName(),
					Namespace: mod.Namespace,
				},
			}

			g.Eventually(ft.suite.k.Get(sa)).WithContext(ctx).Should(
				WithTransform(
					jq.Extract(`.metadata.labels."platform.opendatahub.io/part-of"`),
					Equal(mod.EffectiveName()),
				),
			)
		})
	}
}

func (ft *foundationTests) testOwnerReference(t *testing.T) {
	for _, mod := range ft.suite.modules {
		t.Run(mod.EffectiveName(), func(t *testing.T) {
			g := NewWithT(t)
			sa := &corev1.ServiceAccount{
				ObjectMeta: metav1.ObjectMeta{
					Name:      mod.EffectiveName(),
					Namespace: mod.Namespace,
				},
			}

			g.Eventually(ft.suite.k.Get(sa)).WithContext(ctx).Should(
				WithTransform(
					jq.Extract(`.metadata.ownerReferences`),
					ContainElement(SatisfyAll(
						HaveKeyWithValue("apiVersion", configApi.GroupVersion.String()),
						HaveKeyWithValue("kind", configApi.PlatformOperatorKind),
						HaveKeyWithValue("name", mod.EffectiveName()),
						HaveKeyWithValue("controller", true),
					)),
				),
			)
		})
	}
}

func (ft *foundationTests) testConfigProjection(t *testing.T) {
	for _, mod := range ft.suite.modules {
		t.Run(mod.EffectiveName(), func(t *testing.T) {
			g := NewWithT(t)
			cm := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      mod.EffectiveName() + "-config",
					Namespace: mod.Namespace,
				},
			}

			g.Eventually(ft.suite.k.Get(cm)).WithContext(ctx).Should(And(
				WithTransform(jq.Extract(`.data."module-name"`), Equal(mod.EffectiveName())),
				WithTransform(jq.Extract(`.data."distribution.name"`), Not(BeEmpty())),
				WithTransform(jq.Extract(`.data."distribution.version"`), Not(BeEmpty())),
			))
		})
	}
}

func (ft *foundationTests) testDisableModules(t *testing.T) {
	g := NewWithT(t)

	// Snapshot deployed resources from each PlatformOperator status before disabling.
	type moduleResources struct {
		name      string
		resources []configApi.ResourceRef
	}

	var snapshots []moduleResources
	for _, mod := range ft.suite.modules {
		po := &configApi.PlatformOperator{}
		g.Expect(ft.suite.client.Get(ctx, client.ObjectKey{Name: mod.EffectiveName()}, po)).To(Succeed())
		snapshots = append(snapshots, moduleResources{
			name:      mod.EffectiveName(),
			resources: po.Status.Resources,
		})
	}

	// Remove all modules from the Platform CR.
	g.Eventually(func(g Gomega) {
		p := &configApi.Platform{}
		g.Expect(ft.suite.client.Get(ctx, client.ObjectKey{Name: configApi.PlatformInstanceName}, p)).To(Succeed())
		p.Spec.Modules = nil
		g.Expect(ft.suite.client.Update(ctx, p)).To(Succeed())
	}).WithContext(ctx).Should(Succeed())

	// PlatformOperator CRs should be deleted by GC.
	g.Eventually(func(g Gomega) {
		var poList configApi.PlatformOperatorList
		g.Expect(ft.suite.client.List(ctx, &poList)).To(Succeed())
		g.Expect(poList.Items).To(BeEmpty())
	}).WithContext(ctx).Should(Succeed())

	// Verify each previously deployed resource based on its type.
	for _, snap := range snapshots {
		t.Run(snap.name, func(t *testing.T) {
			g := NewWithT(t)

			for _, ref := range snap.resources {
				objGVK := schema.FromAPIVersionAndKind(ref.APIVersion, ref.Kind)

				switch objGVK {
				case gvk.CustomResourceDefinition:
					key := client.ObjectKey{Name: ref.Name}
					g.Eventually(func() error {
						return ft.suite.client.Get(ctx, key, &unstructured.Unstructured{
							Object: map[string]any{"apiVersion": ref.APIVersion, "kind": ref.Kind},
						})
					}).WithContext(ctx).Should(Succeed(),
						"CRD %s should survive cleanup", ref.Name)
				case gvk.Namespace:
					key := client.ObjectKey{Name: ref.Name}
					g.Eventually(func() error {
						return ft.suite.client.Get(ctx, key, &unstructured.Unstructured{
							Object: map[string]any{"apiVersion": ref.APIVersion, "kind": ref.Kind},
						})
					}).WithContext(ctx).Should(Succeed(),
						"Namespace %s should survive cleanup", ref.Name)
				default:
					key := client.ObjectKey{Name: ref.Name, Namespace: ref.Namespace}
					g.Eventually(func() bool {
						return k8serr.IsNotFound(ft.suite.client.Get(ctx, key, &unstructured.Unstructured{
							Object: map[string]any{"apiVersion": ref.APIVersion, "kind": ref.Kind},
						}))
					}).WithContext(ctx).Should(BeTrue(),
						"%s %s/%s should be cleaned up", ref.Kind, ref.Namespace, ref.Name)
				}
			}
		})
	}
}
