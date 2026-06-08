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
	"context"
	"fmt"
	"testing"
	"time"

	. "github.com/onsi/gomega"
	k8serr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/lburgazzoli/gomega-matchers/pkg/matchers/jq"
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

	snapshot := suite.snapshotClusterState(t)

	suite.cleanupPlatformResources(t)
	suite.deleteModuleCRsAndWait(t)
	suite.assertClusterReset(t, snapshot)
}

func (suite *orchestratorTest) cleanupPlatformResources(t *testing.T) {
	t.Helper()
	g := NewWithT(t)

	p := &configApi.Platform{ObjectMeta: metav1.ObjectMeta{Name: configApi.PlatformInstanceName}}

	g.Eventually(func() error {
		fresh := &configApi.Platform{}
		err := suite.client.Get(ctx, client.ObjectKeyFromObject(p), fresh)
		switch {
		case k8serr.IsNotFound(err):
			return nil
		case err != nil:
			return err
		case !fresh.GetDeletionTimestamp().IsZero():
			return nil
		default:
			return suite.client.Delete(ctx, fresh)
		}
	}).WithContext(ctx).Should(Succeed())

	g.Eventually(ctx, suite.k.NotFound(&configApi.Platform{
		ObjectMeta: metav1.ObjectMeta{Name: configApi.PlatformInstanceName},
	})).Should(
		BeTrue(),
	)

	g.Eventually(suite.k.List(&configApi.PlatformOperatorList{})).
		WithContext(ctx).
		Should(jq.Match(`length == 0`))
}

func (suite *orchestratorTest) assertClusterReset(t *testing.T, snapshot cleanupSnapshot) {
	t.Helper()
	g := NewWithT(t)

	for _, ref := range snapshot.deletedRefs {
		g.Eventually(ctx, suite.k.NotFound(objectFromResourceRef(ref))).Should(
			BeTrue(),
		)
	}

	g.Eventually(func() error {
		return suite.checkClusterReset(ctx, snapshot)
	}).WithContext(ctx).Should(Succeed())

	g.Consistently(func() error {
		return suite.checkClusterReset(ctx, snapshot)
	}).WithContext(ctx).WithTimeout(cleanupQuiescenceTimeout).Should(Succeed())
}

func (suite *orchestratorTest) checkClusterReset(ctx context.Context, snapshot cleanupSnapshot) error {
	for _, ref := range snapshot.deletedRefs {
		obj := objectFromResourceRef(ref)
		err := suite.client.Get(ctx, client.ObjectKeyFromObject(obj), obj)
		switch {
		case k8serr.IsNotFound(err):
			continue
		case err != nil:
			return err
		default:
			return fmt.Errorf("%s %s/%s still exists", ref.Kind, ref.Namespace, ref.Name)
		}
	}

	platform := &configApi.Platform{ObjectMeta: metav1.ObjectMeta{Name: configApi.PlatformInstanceName}}
	err := suite.client.Get(ctx, client.ObjectKeyFromObject(platform), platform)
	switch {
	case k8serr.IsNotFound(err):
	case err != nil:
		return err
	default:
		return fmt.Errorf("platform %q still exists", configApi.PlatformInstanceName)
	}

	poList := &configApi.PlatformOperatorList{}
	if err := suite.client.List(ctx, poList); err != nil {
		return err
	}
	if len(poList.Items) != 0 {
		return fmt.Errorf("platformoperators still exist after reset: %d remaining", len(poList.Items))
	}

	return nil
}

func (suite *orchestratorTest) deleteModuleCRsAndWait(t *testing.T) {
	t.Helper()
	g := NewWithT(t)

	for _, mod := range suite.modules {
		cr := newModuleCR(mod.GVK)

		g.Expect(suite.k.Delete(cr)(ctx)).To(Or(
			Succeed(),
			Satisfy(k8serr.IsNotFound)),
		)

		g.Eventually(ctx, suite.k.NotFound(newModuleCR(mod.GVK))).Should(
			BeTrue(),
		)
	}
}

func (suite *orchestratorTest) checkResourceResetState(
	ctx context.Context,
	ref configApi.ResourceRef,
) error {
	obj := objectFromResourceRef(ref)
	objGVK := schema.FromAPIVersionAndKind(ref.APIVersion, ref.Kind)

	if resourceShouldSurviveReset(objGVK) {
		err := suite.client.Get(ctx, client.ObjectKeyFromObject(obj), obj)
		switch {
		case err != nil:
			return err
		default:
			return nil
		}
	}

	err := suite.client.Get(ctx, client.ObjectKeyFromObject(obj), obj)
	switch {
	case k8serr.IsNotFound(err):
		return nil
	case err != nil:
		return err
	default:
		return fmt.Errorf("%s %s/%s still exists", ref.Kind, ref.Namespace, ref.Name)
	}
}

func (suite *orchestratorTest) platformModuleNames() []string {
	moduleNames := make([]string, 0, len(suite.modules))
	for _, mod := range suite.modules {
		moduleNames = append(moduleNames, mod.EffectiveName())
	}

	return moduleNames
}

func (suite *orchestratorTest) snapshotClusterState(t *testing.T) cleanupSnapshot {
	t.Helper()
	g := NewWithT(t)

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
		switch {
		case k8serr.IsNotFound(err):
			continue
		case err != nil:
			g.Expect(err).NotTo(HaveOccurred())
		default:
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
	suite *orchestratorTest,
	gvk schema.GroupVersionKind,
	version string,
) {
	t.Helper()
	ctx := t.Context()
	g := NewWithT(t)

	key := client.ObjectKeyFromObject(newModuleCR(gvk))

	g.Eventually(ctx, func() error {
		existing := newModuleCR(gvk)
		err := suite.client.Get(ctx, key, existing)
		switch {
		case k8serr.IsNotFound(err):
			return suite.client.Create(ctx, newModuleCR(gvk))
		case err != nil:
			return err
		default:
			desired := newModuleCR(gvk)
			desired.SetResourceVersion(existing.GetResourceVersion())
			return suite.client.Update(ctx, desired)
		}
	}).Should(Succeed())

	if version == "" {
		return
	}

	g.Eventually(ctx, func(g Gomega) {
		existing := newModuleCR(gvk)
		err := suite.client.Get(ctx, key, existing)
		g.Expect(err).To(Succeed())
		g.Expect(unstructured.SetNestedField(existing.Object, version, "status", "release", "version")).To(Succeed())
		g.Expect(suite.client.Status().Update(ctx, existing)).To(Succeed())
	}).Should(Succeed())
}
