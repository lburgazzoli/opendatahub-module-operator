//go:build integration

/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package integration

import (
	"testing"
	"time"

	. "github.com/onsi/gomega"
	k8serr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/controller-runtime/pkg/client"

	configApi "github.com/lburgazzoli/opendatahub-module-operator/orchestrator/api/config/v1alpha1"
)

const cleanupQuiescenceTimeout = 2 * time.Second

type cleanupSnapshot struct {
	deletedRefs []configApi.ResourceRef
}

func (suite *orchestratorTest) setupTest(t *testing.T) {
	t.Helper()

	suite.resetConfig()
	suite.resetClusterState(t)

	t.Cleanup(func() {
		suite.resetConfig()
		suite.resetClusterState(t)
	})
}

func (suite *orchestratorTest) resetConfig() {
	*suite.cfg = baseConfig
}

func (suite *orchestratorTest) setDistributionVersion(version string) {
	suite.cfg.Distribution.Version = version
}

func (suite *orchestratorTest) resetClusterState(t *testing.T) {
	t.Helper()

	g := NewWithT(t)
	snapshot := suite.snapshotClusterState(t, g)

	suite.cleanupPlatformResources(t, g)
	suite.deleteModuleCRsAndWait(t, g)
	suite.assertClusterReset(t, g, snapshot)
}

func (suite *orchestratorTest) cleanupPlatformResources(t *testing.T, g Gomega) {
	t.Helper()

	p := &configApi.Platform{ObjectMeta: metav1.ObjectMeta{Name: configApi.PlatformInstanceName}}

	g.Eventually(func() error {
		fresh := &configApi.Platform{}
		err := suite.client.Get(ctx, client.ObjectKeyFromObject(p), fresh)
		if k8serr.IsNotFound(err) {
			return nil
		}
		if err != nil {
			return err
		}

		if !fresh.GetDeletionTimestamp().IsZero() {
			return nil
		}

		return suite.client.Delete(ctx, fresh)
	}).WithContext(ctx).Should(Succeed())

	g.Eventually(suite.k.Absent(p)).WithContext(ctx).Should(BeTrue())

	g.Eventually(func(g Gomega) {
		var poList configApi.PlatformOperatorList
		g.Expect(suite.client.List(ctx, &poList)).To(Succeed())
		g.Expect(poList.Items).To(BeEmpty())
	}).WithContext(ctx).Should(Succeed())
}

func (suite *orchestratorTest) assertClusterReset(t *testing.T, g Gomega, snapshot cleanupSnapshot) {
	t.Helper()

	for _, ref := range snapshot.deletedRefs {
		refObj := objectFromResourceRef(ref)
		g.Eventually(suite.k.Absent(refObj)).WithContext(ctx).Should(BeTrue())
	}

	g.Eventually(func(g Gomega) {
		suite.expectClusterReset(g, snapshot)
	}).WithContext(ctx).Should(Succeed())

	g.Consistently(func(g Gomega) {
		suite.expectClusterReset(g, snapshot)
	}).WithContext(ctx).WithTimeout(cleanupQuiescenceTimeout).Should(Succeed())
}

func (suite *orchestratorTest) expectClusterReset(g Gomega, snapshot cleanupSnapshot) {
	for _, ref := range snapshot.deletedRefs {
		obj := objectFromResourceRef(ref)
		g.Expect(suite.client.Get(ctx, client.ObjectKeyFromObject(obj), obj)).To(Satisfy(k8serr.IsNotFound))
	}

	g.Expect(suite.client.Get(ctx, client.ObjectKey{Name: configApi.PlatformInstanceName}, &configApi.Platform{})).
		To(Satisfy(k8serr.IsNotFound))

	var poList configApi.PlatformOperatorList
	g.Expect(suite.client.List(ctx, &poList)).To(Succeed())
	g.Expect(poList.Items).To(BeEmpty())
}

func (suite *orchestratorTest) deleteModuleCRsAndWait(t *testing.T, g Gomega) {
	t.Helper()

	for _, mod := range suite.modules {
		cr := newModuleCR(mod.GVK)

		err := suite.client.Delete(ctx, cr)
		if err != nil && !k8serr.IsNotFound(err) {
			g.Expect(err).NotTo(HaveOccurred())
		}

		g.Eventually(func() bool {
			fresh := newModuleCR(mod.GVK)
			err := suite.client.Get(ctx, client.ObjectKeyFromObject(fresh), fresh)
			return k8serr.IsNotFound(err)
		}).WithContext(ctx).Should(BeTrue())
	}
}

func (suite *orchestratorTest) platformModuleNames() []string {
	moduleNames := make([]string, 0, len(suite.modules))
	for _, mod := range suite.modules {
		moduleNames = append(moduleNames, mod.EffectiveName())
	}

	return moduleNames
}

func (suite *orchestratorTest) snapshotClusterState(t *testing.T, g Gomega) cleanupSnapshot {
	t.Helper()

	snapshot := cleanupSnapshot{}
	seen := sets.New[configApi.ResourceRef]()

	var poList configApi.PlatformOperatorList
	err := suite.client.List(ctx, &poList)
	if err != nil {
		g.Expect(err).NotTo(HaveOccurred())
	}

	for i := range poList.Items {
		for _, ref := range poList.Items[i].Status.Resources {
			refGVK := schema.FromAPIVersionAndKind(ref.APIVersion, ref.Kind)
			if resourceShouldSurviveReset(refGVK) || seen.Has(ref) {
				continue
			}
			seen.Insert(ref)
			snapshot.deletedRefs = append(snapshot.deletedRefs, ref)
		}
	}

	for _, mod := range suite.modules {
		cr := newModuleCR(mod.GVK)
		err := suite.client.Get(ctx, client.ObjectKeyFromObject(cr), cr)
		if k8serr.IsNotFound(err) {
			continue
		}
		if err != nil {
			g.Expect(err).NotTo(HaveOccurred())
		}

		ref := configApi.ResourceRef{
			APIVersion: cr.GetAPIVersion(),
			Kind:       cr.GetKind(),
			Namespace:  cr.GetNamespace(),
			Name:       cr.GetName(),
		}
		if seen.Has(ref) {
			continue
		}
		seen.Insert(ref)
		snapshot.deletedRefs = append(snapshot.deletedRefs, ref)
	}

	return snapshot
}

func resourceShouldSurviveReset(gvk schema.GroupVersionKind) bool {
	switch gvk {
	case schema.GroupVersionKind{
		Group:   "apiextensions.k8s.io",
		Version: "v1",
		Kind:    "CustomResourceDefinition",
	}, schema.GroupVersionKind{
		Group:   "",
		Version: "v1",
		Kind:    "Namespace",
	}:
		return true
	default:
		return false
	}
}

func objectFromResourceRef(ref configApi.ResourceRef) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{}
	obj.SetAPIVersion(ref.APIVersion)
	obj.SetKind(ref.Kind)
	obj.SetNamespace(ref.Namespace)
	obj.SetName(ref.Name)
	return obj
}

func newPlatformWithModules(modules []string) *configApi.Platform {
	return &configApi.Platform{
		ObjectMeta: metav1.ObjectMeta{Name: configApi.PlatformInstanceName},
		Spec: configApi.PlatformSpec{
			Modules: modules,
		},
	}
}

func newModuleCR(gvk schema.GroupVersionKind) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": gvk.Group + "/" + gvk.Version,
			"kind":       gvk.Kind,
			"metadata": map[string]any{
				"name": "default",
			},
		},
	}
}

func upsertModuleCRWithVersion(
	t *testing.T,
	g Gomega,
	suite *orchestratorTest,
	gvk schema.GroupVersionKind,
	version string,
) {
	t.Helper()

	key := client.ObjectKeyFromObject(newModuleCR(gvk))

	g.Eventually(func() error {
		existing := newModuleCR(gvk)
		err := suite.client.Get(t.Context(), key, existing)
		if err == nil {
			desired := newModuleCR(gvk)
			desired.SetResourceVersion(existing.GetResourceVersion())
			return suite.client.Update(t.Context(), desired)
		}
		if !k8serr.IsNotFound(err) {
			return err
		}

		return suite.client.Create(t.Context(), newModuleCR(gvk))
	}).WithContext(t.Context()).Should(Succeed())

	if version == "" {
		return
	}

	g.Eventually(func(g Gomega) {
		existing := newModuleCR(gvk)
		err := suite.client.Get(t.Context(), key, existing)
		g.Expect(err).To(Succeed())
		g.Expect(unstructured.SetNestedField(existing.Object, version, "status", "release", "version")).To(Succeed())
		g.Expect(suite.client.Status().Update(t.Context(), existing)).To(Succeed())
	}).WithContext(t.Context()).Should(Succeed())
}
