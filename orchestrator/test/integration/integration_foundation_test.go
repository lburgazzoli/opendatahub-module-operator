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
	newSuite suiteFactory
}

func (ft *foundationTests) Execute(t *testing.T) {
	t.Run("empty platform should reconcile", ft.testEmptyPlatform)
	t.Run("module deployment", ft.testModuleDeployment)
	t.Run("module version propagation", ft.testVersionPropagation)
	t.Run("disabling modules should clean up resources", ft.testDisableModules)
}

func (ft *foundationTests) testEmptyPlatform(t *testing.T) {
	g := NewWithT(t)
	suite := ft.newSuite(t)
	suite.setupTest(t)

	p := &configApi.Platform{
		ObjectMeta: metav1.ObjectMeta{Name: configApi.PlatformInstanceName},
	}

	g.Expect(suite.client.Create(t.Context(), p)).To(Succeed())

	g.Eventually(suite.k.Get(p)).WithContext(t.Context()).Should(
		WithTransform(jq.Extract(`.status.distribution.version`), Not(BeEmpty())),
	)

	g.Eventually(func(g Gomega) {
		var poList configApi.PlatformOperatorList
		g.Expect(suite.client.List(t.Context(), &poList)).To(Succeed())
		g.Expect(poList.Items).To(BeEmpty())
	}).WithContext(t.Context()).Should(Succeed())
}

func (ft *foundationTests) testModuleDeployment(t *testing.T) {
	g := NewWithT(t)
	suite := ft.newSuite(t)
	suite.setupTest(t)

	p := newPlatformWithModules(suite.platformModuleNames())
	g.Expect(suite.client.Create(t.Context(), p)).To(Succeed())

	for _, mod := range suite.modules {
		g.Eventually(func(g Gomega) {
			po := &configApi.PlatformOperator{}
			g.Expect(suite.client.Get(t.Context(), client.ObjectKey{Name: mod.EffectiveName()}, po)).To(Succeed())
			g.Expect(po.Status.Resources).NotTo(BeEmpty())
		}).WithContext(t.Context()).Should(Succeed())
	}

	t.Run("all modules should track resources in status", func(t *testing.T) {
		for _, mod := range suite.modules {
			t.Run(mod.EffectiveName(), func(t *testing.T) {
				g := NewWithT(t)
				po := &configApi.PlatformOperator{ObjectMeta: metav1.ObjectMeta{Name: mod.EffectiveName()}}
				g.Eventually(suite.k.Get(po)).WithContext(t.Context()).Should(And(
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
		for _, mod := range suite.modules {
			t.Run(mod.EffectiveName(), func(t *testing.T) {
				g := NewWithT(t)
				po := &configApi.PlatformOperator{ObjectMeta: metav1.ObjectMeta{Name: mod.EffectiveName()}}
				g.Eventually(suite.k.Get(po)).WithContext(t.Context()).Should(And(
					WithTransform(jq.Extract(`.status.chart.name`), Equal("test-module")),
					WithTransform(jq.Extract(`.status.chart.path`), Equal(mod.ChartPath)),
				))
			})
		}
	})

	t.Run("all modules should report runlevel", func(t *testing.T) {
		for _, mod := range suite.modules {
			t.Run(mod.EffectiveName(), func(t *testing.T) {
				g := NewWithT(t)
				po := &configApi.PlatformOperator{ObjectMeta: metav1.ObjectMeta{Name: mod.EffectiveName()}}
				g.Eventually(suite.k.Get(po)).WithContext(t.Context()).Should(
					WithTransform(jq.Extract(`.status.runlevel`), BeEquivalentTo(mod.Runlevel)),
				)
			})
		}
	})

	t.Run("each module should have its own CRD", func(t *testing.T) {
		for _, mod := range suite.modules {
			crdName := fmt.Sprintf("%ss.%s", mod.EffectiveName(), mod.GVK.Group)
			t.Run(mod.EffectiveName(), func(t *testing.T) {
				g := NewWithT(t)
				po := &configApi.PlatformOperator{ObjectMeta: metav1.ObjectMeta{Name: mod.EffectiveName()}}
				g.Eventually(suite.k.Get(po)).WithContext(t.Context()).Should(
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
		for _, mod := range suite.modules {
			t.Run(mod.EffectiveName(), func(t *testing.T) {
				g := NewWithT(t)
				sa := &corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{
					Name: mod.EffectiveName(), Namespace: mod.Namespace,
				}}
				g.Eventually(suite.k.Get(sa)).WithContext(t.Context()).Should(
					WithTransform(
						jq.Extract(`.metadata.labels."platform.opendatahub.io/part-of"`),
						Equal(mod.EffectiveName()),
					),
				)
			})
		}
	})

	t.Run("deployed resources should have owner reference", func(t *testing.T) {
		for _, mod := range suite.modules {
			t.Run(mod.EffectiveName(), func(t *testing.T) {
				g := NewWithT(t)
				sa := &corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{
					Name: mod.EffectiveName(), Namespace: mod.Namespace,
				}}
				g.Eventually(suite.k.Get(sa)).WithContext(t.Context()).Should(
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
		for _, mod := range suite.modules {
			t.Run(mod.EffectiveName(), func(t *testing.T) {
				g := NewWithT(t)
				cm := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{
					Name: mod.EffectiveName() + "-config", Namespace: mod.Namespace,
				}}
				g.Eventually(suite.k.Get(cm)).WithContext(t.Context()).Should(And(
					WithTransform(jq.Extract(`.data."module-name"`), Equal(mod.EffectiveName())),
					WithTransform(jq.Extract(`.data."distribution.name"`), Not(BeEmpty())),
					WithTransform(jq.Extract(`.data."distribution.version"`), Not(BeEmpty())),
				))
			})
		}
	})

	t.Run("PlatformOperator should be owned by Platform", func(t *testing.T) {
		for _, mod := range suite.modules {
			t.Run(mod.EffectiveName(), func(t *testing.T) {
				g := NewWithT(t)
				po := &configApi.PlatformOperator{ObjectMeta: metav1.ObjectMeta{Name: mod.EffectiveName()}}
				g.Eventually(suite.k.Get(po)).WithContext(t.Context()).Should(
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
	suite := ft.newSuite(t)
	suite.setupTest(t)

	p := newPlatformWithModules(suite.platformModuleNames())
	g.Expect(suite.client.Create(t.Context(), p)).To(Succeed())

	for _, mod := range suite.modules {
		g.Eventually(func(g Gomega) {
			po := &configApi.PlatformOperator{}
			g.Expect(suite.client.Get(t.Context(), client.ObjectKey{Name: mod.EffectiveName()}, po)).To(Succeed())
			g.Expect(po.Status.Resources).NotTo(BeEmpty())
		}).WithContext(t.Context()).Should(Succeed())
	}

	mod := suite.modules[0]

	upsertModuleCRWithVersion(t, g, suite, mod.GVK, "test-version")

	po := &configApi.PlatformOperator{ObjectMeta: metav1.ObjectMeta{Name: mod.EffectiveName()}}
	g.Eventually(suite.k.Get(po)).WithContext(t.Context()).Should(
		WithTransform(jq.Extract(`.status.distribution.version`), Equal("test-version")),
	)
}

func (ft *foundationTests) testDisableModules(t *testing.T) {
	g := NewWithT(t)
	suite := ft.newSuite(t)
	suite.setupTest(t)

	p := newPlatformWithModules(suite.platformModuleNames())
	g.Expect(suite.client.Create(t.Context(), p)).To(Succeed())

	for _, mod := range suite.modules {
		g.Eventually(func(g Gomega) {
			po := &configApi.PlatformOperator{}
			g.Expect(suite.client.Get(t.Context(), client.ObjectKey{Name: mod.EffectiveName()}, po)).To(Succeed())
			g.Expect(po.Status.Resources).NotTo(BeEmpty())
		}).WithContext(t.Context()).Should(Succeed())
	}

	// Snapshot deployed resources.
	type moduleResources struct {
		name      string
		resources []configApi.ResourceRef
	}

	snapshots := make([]moduleResources, 0, len(suite.modules))
	for _, mod := range suite.modules {
		po := &configApi.PlatformOperator{}
		g.Expect(suite.client.Get(t.Context(), client.ObjectKey{Name: mod.EffectiveName()}, po)).To(Succeed())
		snapshots = append(snapshots, moduleResources{
			name:      mod.EffectiveName(),
			resources: po.Status.Resources,
		})
	}

	// Remove all modules.
	g.Eventually(func(g Gomega) {
		p := &configApi.Platform{}
		g.Expect(suite.client.Get(t.Context(), client.ObjectKey{Name: configApi.PlatformInstanceName}, p)).To(Succeed())
		p.Spec.Modules = nil
		g.Expect(suite.client.Update(t.Context(), p)).To(Succeed())
	}).WithContext(t.Context()).Should(Succeed())

	g.Eventually(func(g Gomega) {
		var poList configApi.PlatformOperatorList
		g.Expect(suite.client.List(t.Context(), &poList)).To(Succeed())
		g.Expect(poList.Items).To(BeEmpty())
	}).WithContext(t.Context()).Should(Succeed())

	for _, snap := range snapshots {
		t.Run(snap.name, func(t *testing.T) {
			g := NewWithT(t)
			for _, ref := range snap.resources {
				objGVK := schema.FromAPIVersionAndKind(ref.APIVersion, ref.Kind)
				switch objGVK {
				case gvk.CustomResourceDefinition:
					key := client.ObjectKey{Name: ref.Name}
					g.Eventually(func() error {
						return suite.client.Get(t.Context(), key, &unstructured.Unstructured{
							Object: map[string]any{"apiVersion": ref.APIVersion, "kind": ref.Kind},
						})
					}).WithContext(t.Context()).Should(Succeed(), "CRD %s should survive cleanup", ref.Name)
				case gvk.Namespace:
					key := client.ObjectKey{Name: ref.Name}
					g.Eventually(func() error {
						return suite.client.Get(t.Context(), key, &unstructured.Unstructured{
							Object: map[string]any{"apiVersion": ref.APIVersion, "kind": ref.Kind},
						})
					}).WithContext(t.Context()).Should(Succeed(), "Namespace %s should survive cleanup", ref.Name)
				default:
					key := client.ObjectKey{Name: ref.Name, Namespace: ref.Namespace}
					g.Eventually(func() bool {
						return k8serr.IsNotFound(suite.client.Get(t.Context(), key, &unstructured.Unstructured{
							Object: map[string]any{"apiVersion": ref.APIVersion, "kind": ref.Kind},
						}))
					}).WithContext(t.Context()).Should(BeTrue(), "%s %s/%s should be cleaned up", ref.Kind, ref.Namespace, ref.Name)
				}
			}
		})
	}
}
