//go:build integration

package integration

import (
	"fmt"
	"strings"
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
	t.Run("empty platform should reconcile", ft.testEmptyPlatform)
	t.Run("module deployment", ft.testModuleDeployment)
	t.Run("module version propagation", ft.testVersionPropagation)
	t.Run("disabling modules should clean up resources", ft.testDisableModules)
}

// createPlatformWithModules creates a Platform CR with all test modules enabled
// and waits for all PlatformOperator CRs to have deployed resources.
func (ft *foundationTests) createPlatformWithModules(t *testing.T, g Gomega) {
	t.Helper()

	p := &configApi.Platform{
		ObjectMeta: metav1.ObjectMeta{Name: configApi.PlatformInstanceName},
	}

	_ = ft.suite.client.Delete(ctx, p)
	g.Eventually(ft.suite.k.Absent(p)).WithContext(ctx).Should(BeTrue())

	moduleNames := make([]string, 0, len(ft.suite.modules))
	for _, mod := range ft.suite.modules {
		moduleNames = append(moduleNames, mod.EffectiveName())
	}

	p.Spec.Modules = moduleNames
	g.Expect(ft.suite.client.Create(ctx, p)).To(Succeed())

	for _, mod := range ft.suite.modules {
		g.Eventually(func(g Gomega) {
			po := &configApi.PlatformOperator{}
			g.Expect(ft.suite.client.Get(ctx, client.ObjectKey{Name: mod.EffectiveName()}, po)).To(Succeed())
			g.Expect(po.Status.Resources).NotTo(BeEmpty())
		}).WithContext(ctx).Should(Succeed())
	}

	t.Cleanup(func() {
		_ = ft.suite.client.Delete(ctx, p)
	})
}

func (ft *foundationTests) testEmptyPlatform(t *testing.T) {
	g := NewWithT(t)

	p := &configApi.Platform{
		ObjectMeta: metav1.ObjectMeta{Name: configApi.PlatformInstanceName},
	}

	_ = ft.suite.client.Delete(ctx, p)
	g.Eventually(ft.suite.k.Absent(p)).WithContext(ctx).Should(BeTrue())

	g.Expect(ft.suite.client.Create(ctx, p)).To(Succeed())

	t.Cleanup(func() {
		_ = ft.suite.client.Delete(ctx, p)
	})

	g.Eventually(ft.suite.k.Get(p)).WithContext(ctx).Should(
		WithTransform(jq.Extract(`.status.distribution.version`), Not(BeEmpty())),
	)

	g.Eventually(func(g Gomega) {
		var poList configApi.PlatformOperatorList
		g.Expect(ft.suite.client.List(ctx, &poList)).To(Succeed())
		g.Expect(poList.Items).To(BeEmpty())
	}).WithContext(ctx).Should(Succeed())
}

func (ft *foundationTests) testModuleDeployment(t *testing.T) {
	g := NewWithT(t)
	ft.createPlatformWithModules(t, g)

	t.Run("all modules should track resources in status", func(t *testing.T) {
		for _, mod := range ft.suite.modules {
			t.Run(mod.EffectiveName(), func(t *testing.T) {
				g := NewWithT(t)
				po := &configApi.PlatformOperator{ObjectMeta: metav1.ObjectMeta{Name: mod.EffectiveName()}}
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
	})

	t.Run("all modules should report chart info", func(t *testing.T) {
		for _, mod := range ft.suite.modules {
			t.Run(mod.EffectiveName(), func(t *testing.T) {
				g := NewWithT(t)
				po := &configApi.PlatformOperator{ObjectMeta: metav1.ObjectMeta{Name: mod.EffectiveName()}}
				g.Eventually(ft.suite.k.Get(po)).WithContext(ctx).Should(And(
					WithTransform(jq.Extract(`.status.chart.name`), Equal("test-module")),
					WithTransform(jq.Extract(`.status.chart.path`), Equal(mod.ChartPath)),
				))
			})
		}
	})

	t.Run("all modules should report runlevel", func(t *testing.T) {
		for _, mod := range ft.suite.modules {
			t.Run(mod.EffectiveName(), func(t *testing.T) {
				g := NewWithT(t)
				po := &configApi.PlatformOperator{ObjectMeta: metav1.ObjectMeta{Name: mod.EffectiveName()}}
				g.Eventually(ft.suite.k.Get(po)).WithContext(ctx).Should(
					WithTransform(jq.Extract(`.status.runlevel`), BeEquivalentTo(mod.Runlevel)),
				)
			})
		}
	})

	t.Run("each module should have its own CRD", func(t *testing.T) {
		for _, mod := range ft.suite.modules {
			crdName := fmt.Sprintf("%ss.%s", mod.EffectiveName(), mod.GVK.Group)
			t.Run(mod.EffectiveName(), func(t *testing.T) {
				g := NewWithT(t)
				po := &configApi.PlatformOperator{ObjectMeta: metav1.ObjectMeta{Name: mod.EffectiveName()}}
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
	})

	t.Run("deployed resources should have part-of label", func(t *testing.T) {
		for _, mod := range ft.suite.modules {
			t.Run(mod.EffectiveName(), func(t *testing.T) {
				g := NewWithT(t)
				sa := &corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{
					Name: mod.EffectiveName(), Namespace: mod.Namespace,
				}}
				g.Eventually(ft.suite.k.Get(sa)).WithContext(ctx).Should(
					WithTransform(
						jq.Extract(`.metadata.labels."platform.opendatahub.io/part-of"`),
						Equal(mod.EffectiveName()),
					),
				)
			})
		}
	})

	t.Run("deployed resources should have owner reference", func(t *testing.T) {
		for _, mod := range ft.suite.modules {
			t.Run(mod.EffectiveName(), func(t *testing.T) {
				g := NewWithT(t)
				sa := &corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{
					Name: mod.EffectiveName(), Namespace: mod.Namespace,
				}}
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
	})

	t.Run("config values should be projected to configmap", func(t *testing.T) {
		for _, mod := range ft.suite.modules {
			t.Run(mod.EffectiveName(), func(t *testing.T) {
				g := NewWithT(t)
				cm := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{
					Name: mod.EffectiveName() + "-config", Namespace: mod.Namespace,
				}}
				g.Eventually(ft.suite.k.Get(cm)).WithContext(ctx).Should(And(
					WithTransform(jq.Extract(`.data."module-name"`), Equal(mod.EffectiveName())),
					WithTransform(jq.Extract(`.data."distribution.name"`), Not(BeEmpty())),
					WithTransform(jq.Extract(`.data."distribution.version"`), Not(BeEmpty())),
				))
			})
		}
	})

	t.Run("PlatformOperator should be owned by Platform", func(t *testing.T) {
		for _, mod := range ft.suite.modules {
			t.Run(mod.EffectiveName(), func(t *testing.T) {
				g := NewWithT(t)
				po := &configApi.PlatformOperator{ObjectMeta: metav1.ObjectMeta{Name: mod.EffectiveName()}}
				g.Eventually(ft.suite.k.Get(po)).WithContext(ctx).Should(
					WithTransform(
						jq.Extract(`.metadata.ownerReferences`),
						ContainElement(SatisfyAll(
							HaveKeyWithValue("apiVersion", configApi.GroupVersion.String()),
							HaveKeyWithValue("kind", configApi.PlatformKind),
							HaveKeyWithValue("name", configApi.PlatformInstanceName),
							HaveKeyWithValue("controller", true),
						)),
					),
				)
			})
		}
	})
}

func (ft *foundationTests) testVersionPropagation(t *testing.T) {
	g := NewWithT(t)
	ft.createPlatformWithModules(t, g)

	mod := ft.suite.modules[0]

	gvr := schema.GroupVersionResource{
		Group:    mod.GVK.Group,
		Version:  mod.GVK.Version,
		Resource: strings.ToLower(mod.GVK.Kind) + "s",
	}

	cr := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": mod.GVK.Group + "/" + mod.GVK.Version,
			"kind":       mod.GVK.Kind,
			"metadata":   map[string]any{"name": "default"},
		},
	}

	g.Eventually(func() error {
		_, err := ft.suite.dynamic.Resource(gvr).Create(ctx, cr, metav1.CreateOptions{})
		return err
	}).WithContext(ctx).Should(Succeed())

	t.Cleanup(func() {
		_ = ft.suite.dynamic.Resource(gvr).Delete(ctx, "default", metav1.DeleteOptions{})
	})

	g.Eventually(func(g Gomega) {
		existing, err := ft.suite.dynamic.Resource(gvr).Get(ctx, "default", metav1.GetOptions{})
		g.Expect(err).To(Succeed())
		_ = unstructured.SetNestedField(existing.Object, "test-version", "status", "release", "version")
		_, err = ft.suite.dynamic.Resource(gvr).UpdateStatus(ctx, existing, metav1.UpdateOptions{})
		g.Expect(err).To(Succeed())
	}).WithContext(ctx).Should(Succeed())

	po := &configApi.PlatformOperator{ObjectMeta: metav1.ObjectMeta{Name: mod.EffectiveName()}}
	g.Eventually(ft.suite.k.Get(po)).WithContext(ctx).Should(
		WithTransform(jq.Extract(`.status.distribution.version`), Equal("test-version")),
	)
}

func (ft *foundationTests) testDisableModules(t *testing.T) {
	g := NewWithT(t)
	ft.createPlatformWithModules(t, g)

	// Snapshot deployed resources.
	type moduleResources struct {
		name      string
		resources []configApi.ResourceRef
	}

	snapshots := make([]moduleResources, 0, len(ft.suite.modules))
	for _, mod := range ft.suite.modules {
		po := &configApi.PlatformOperator{}
		g.Expect(ft.suite.client.Get(ctx, client.ObjectKey{Name: mod.EffectiveName()}, po)).To(Succeed())
		snapshots = append(snapshots, moduleResources{
			name:      mod.EffectiveName(),
			resources: po.Status.Resources,
		})
	}

	// Remove all modules.
	g.Eventually(func(g Gomega) {
		p := &configApi.Platform{}
		g.Expect(ft.suite.client.Get(ctx, client.ObjectKey{Name: configApi.PlatformInstanceName}, p)).To(Succeed())
		p.Spec.Modules = nil
		g.Expect(ft.suite.client.Update(ctx, p)).To(Succeed())
	}).WithContext(ctx).Should(Succeed())

	g.Eventually(func(g Gomega) {
		var poList configApi.PlatformOperatorList
		g.Expect(ft.suite.client.List(ctx, &poList)).To(Succeed())
		g.Expect(poList.Items).To(BeEmpty())
	}).WithContext(ctx).Should(Succeed())

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
					}).WithContext(ctx).Should(Succeed(), "CRD %s should survive cleanup", ref.Name)
				case gvk.Namespace:
					key := client.ObjectKey{Name: ref.Name}
					g.Eventually(func() error {
						return ft.suite.client.Get(ctx, key, &unstructured.Unstructured{
							Object: map[string]any{"apiVersion": ref.APIVersion, "kind": ref.Kind},
						})
					}).WithContext(ctx).Should(Succeed(), "Namespace %s should survive cleanup", ref.Name)
				default:
					key := client.ObjectKey{Name: ref.Name, Namespace: ref.Namespace}
					g.Eventually(func() bool {
						return k8serr.IsNotFound(ft.suite.client.Get(ctx, key, &unstructured.Unstructured{
							Object: map[string]any{"apiVersion": ref.APIVersion, "kind": ref.Kind},
						}))
					}).WithContext(ctx).Should(BeTrue(), "%s %s/%s should be cleaned up", ref.Kind, ref.Namespace, ref.Name)
				}
			}
		})
	}
}
