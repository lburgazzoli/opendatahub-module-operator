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

package support

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	k8serr "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/controller-runtime/pkg/client"

	k8sm "github.com/lburgazzoli/gomega-matchers/pkg/matchers/k8s"
	configApi "github.com/lburgazzoli/opendatahub-module-operator/orchestrator/api/config/v1alpha1"
	"github.com/lburgazzoli/opendatahub-module-operator/orchestrator/pkg/config"
)

const (
	cleanupQuiescenceTimeout = 2 * time.Second
	cleanupWaitTimeout       = 60 * time.Second
)

type cleanupSnapshot struct {
	deletedRefs []configApi.ResourceRef
}

type suiteGomegaT struct {
	err error
}

func (t *suiteGomegaT) Helper() {}

func (t *suiteGomegaT) Fatalf(format string, args ...any) {
	t.err = fmt.Errorf(format, args...)
}

func (suite *Suite) SetupTest(t *testing.T) {
	t.Helper()

	suite.resetConfig()
	suite.resetClusterState(t, t.Context())
}

func (suite *Suite) SetDistributionVersion(version string) {
	suite.Config.Distribution.Version = version
}

func (suite *Suite) PlatformModuleNames() []string {
	moduleNames := make([]string, 0, len(suite.Modules))
	for _, mod := range suite.Modules {
		moduleNames = append(moduleNames, mod.Name)
	}

	return moduleNames
}

func (suite *Suite) CheckResourceResetState(ctx context.Context, ref configApi.ResourceRef) error {
	obj := objectFromResourceRef(ref)
	objGVK := schema.FromAPIVersionAndKind(ref.APIVersion, ref.Kind)

	if resourceShouldSurviveReset(objGVK) {
		err := suite.Client.Get(ctx, client.ObjectKeyFromObject(obj), obj)
		switch {
		case err != nil:
			return err
		default:
			return nil
		}
	}

	err := suite.Client.Get(ctx, client.ObjectKeyFromObject(obj), obj)
	switch {
	case k8serr.IsNotFound(err):
		return nil
	case err != nil:
		return err
	default:
		return fmt.Errorf("%s %s/%s still exists", ref.Kind, ref.Namespace, ref.Name)
	}
}

func NewPlatformWithModules(modules []string) *configApi.Platform {
	return &configApi.Platform{
		ObjectMeta: metav1.ObjectMeta{Name: configApi.PlatformInstanceName},
		Spec: configApi.PlatformSpec{
			Modules: modules,
		},
	}
}

func AdminAcksConfigMap(namespace string) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      config.AdminAcksConfigMapName,
			Namespace: namespace,
		},
	}
}

func UpsertModuleCRWithVersion(
	t *testing.T,
	suite *Suite,
	gvk schema.GroupVersionKind,
	version string,
) {
	t.Helper()
	g := gomega.NewWithT(t)

	g.Eventually(t.Context(), k8sm.Upsert(
		suite.Client, newModuleCR(gvk),
		func(_ *unstructured.Unstructured) {},
	)).Should(gomega.Not(gomega.BeNil()))

	if version == "" {
		return
	}

	distName := suite.Config.Distribution.Name
	g.Eventually(t.Context(), k8sm.StatusUpdate(
		suite.Client, newModuleCR(gvk),
		func(obj *unstructured.Unstructured) {
			_ = unstructured.SetNestedField(
				obj.Object, distName,
				"status", "release", "name",
			)
			_ = unstructured.SetNestedField(
				obj.Object, version,
				"status", "release", "version",
			)
		},
	)).Should(gomega.Not(gomega.BeNil()))
}

func (suite *Suite) resetConfig() {
	*suite.Config = baseConfig
}

func (suite *Suite) resetClusterState(t *testing.T, ctx context.Context) {
	t.Helper()

	snapshot := suite.snapshotClusterState(t, ctx)

	suite.cleanupPlatformResources(t, ctx)
	suite.deleteModuleCRsAndWait(t, ctx)
	suite.assertClusterReset(t, ctx, snapshot)
}

func (suite *Suite) cleanupBeforeRun(ctx context.Context) error {
	snapshot, err := suite.snapshotClusterStateNow(ctx)
	if err != nil {
		return err
	}

	if err := suite.cleanupPlatformResourcesNow(ctx); err != nil {
		return err
	}
	if err := suite.deleteModuleCRsAndWaitNow(ctx); err != nil {
		return err
	}

	return suite.waitForClusterReset(ctx, snapshot)
}

func (suite *Suite) cleanupPlatformResources(t *testing.T, ctx context.Context) {
	t.Helper()
	g := gomega.NewWithT(t)

	p := &configApi.Platform{ObjectMeta: metav1.ObjectMeta{Name: configApi.PlatformInstanceName}}

	g.Eventually(func() error {
		fresh := &configApi.Platform{}
		err := suite.Client.Get(ctx, client.ObjectKeyFromObject(p), fresh)
		switch {
		case k8serr.IsNotFound(err):
			return nil
		case err != nil:
			return err
		case !fresh.GetDeletionTimestamp().IsZero():
			return nil
		default:
			return suite.Client.Delete(ctx, fresh)
		}
	}).WithContext(ctx).Should(gomega.Succeed())

	g.Eventually(ctx, k8sm.NotFound(suite.Client, &configApi.Platform{
		ObjectMeta: metav1.ObjectMeta{Name: configApi.PlatformInstanceName},
	})).Should(gomega.BeTrue())

	adminAcks := AdminAcksConfigMap(suite.Config.Namespace())
	if err := suite.Client.Delete(ctx, adminAcks); err != nil && !k8serr.IsNotFound(err) {
		g.Expect(err).NotTo(gomega.HaveOccurred())
	}
	g.Eventually(ctx, k8sm.NotFound(suite.Client, AdminAcksConfigMap(suite.Config.Namespace()))).
		Should(gomega.BeTrue())

	g.Eventually(ctx, k8sm.List(suite.Client, &configApi.PlatformOperatorList{})).
		Should(k8sm.IsEmptyList())
}

func (suite *Suite) cleanupPlatformResourcesNow(ctx context.Context) error {
	p := &configApi.Platform{ObjectMeta: metav1.ObjectMeta{Name: configApi.PlatformInstanceName}}

	if err := suite.Client.Get(ctx, client.ObjectKeyFromObject(p), p); err == nil {
		if p.GetDeletionTimestamp().IsZero() {
			if err := suite.Client.Delete(ctx, p); err != nil && !k8serr.IsNotFound(err) {
				return fmt.Errorf("deleting platform %q: %w", p.GetName(), err)
			}
		}
	} else if !k8serr.IsNotFound(err) {
		return fmt.Errorf("getting platform %q: %w", p.GetName(), err)
	}

	adminAcks := AdminAcksConfigMap(suite.Config.Namespace())
	if err := suite.Client.Delete(ctx, adminAcks); err != nil && !k8serr.IsNotFound(err) {
		return fmt.Errorf(
			"deleting admin-acks ConfigMap %s/%s: %w",
			adminAcks.GetNamespace(),
			adminAcks.GetName(),
			err,
		)
	}

	return nil
}

func (suite *Suite) assertClusterReset(t *testing.T, ctx context.Context, snapshot cleanupSnapshot) {
	t.Helper()
	g := gomega.NewWithT(t)

	for _, ref := range snapshot.deletedRefs {
		g.Eventually(ctx, k8sm.Absent(suite.Client, objectFromResourceRef(ref))).
			Should(gomega.BeTrue())
	}

	g.Eventually(func() error {
		return suite.checkClusterReset(ctx, snapshot)
	}).WithContext(ctx).Should(gomega.Succeed())

	g.Consistently(func() error {
		return suite.checkClusterReset(ctx, snapshot)
	}).WithContext(ctx).WithTimeout(cleanupQuiescenceTimeout).Should(gomega.Succeed())
}

func (suite *Suite) checkClusterReset(ctx context.Context, snapshot cleanupSnapshot) error {
	for _, ref := range snapshot.deletedRefs {
		obj := objectFromResourceRef(ref)
		err := suite.Client.Get(ctx, client.ObjectKeyFromObject(obj), obj)
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
	err := suite.Client.Get(ctx, client.ObjectKeyFromObject(platform), platform)
	switch {
	case k8serr.IsNotFound(err):
	case err != nil:
		return err
	default:
		return fmt.Errorf("platform %q still exists", configApi.PlatformInstanceName)
	}

	poList := &configApi.PlatformOperatorList{}
	if err := suite.Client.List(ctx, poList); err != nil {
		return err
	}
	if len(poList.Items) != 0 {
		return fmt.Errorf("platformoperators still exist after reset: %d remaining", len(poList.Items))
	}

	adminAcks := AdminAcksConfigMap(suite.Config.Namespace())
	err = suite.Client.Get(ctx, client.ObjectKeyFromObject(adminAcks), adminAcks)
	switch {
	case k8serr.IsNotFound(err):
	case err != nil:
		return err
	default:
		return fmt.Errorf("admin-acks ConfigMap %s/%s still exists", adminAcks.GetNamespace(), adminAcks.GetName())
	}

	return nil
}

func (suite *Suite) waitForClusterReset(ctx context.Context, snapshot cleanupSnapshot) error {
	return eventuallySucceeds(ctx, func() error {
		return suite.checkClusterReset(ctx, snapshot)
	})
}

func (suite *Suite) deleteModuleCRsAndWait(t *testing.T, ctx context.Context) {
	t.Helper()
	g := gomega.NewWithT(t)

	for _, mod := range suite.CleanupModules {
		cr := newModuleCR(mod.GVK)
		err := suite.Client.Delete(ctx, cr)
		switch {
		case k8serr.IsNotFound(err), apimeta.IsNoMatchError(err):
		case err != nil:
			g.Expect(err).NotTo(gomega.HaveOccurred())
		}

		g.Eventually(func() error {
			fresh := newModuleCR(mod.GVK)
			err := suite.Client.Get(ctx, client.ObjectKeyFromObject(fresh), fresh)
			switch {
			case k8serr.IsNotFound(err), apimeta.IsNoMatchError(err):
				return nil
			case err != nil:
				return err
			default:
				return fmt.Errorf("%s %s/%s still exists", mod.GVK.Kind, fresh.GetNamespace(), fresh.GetName())
			}
		}).WithContext(ctx).Should(gomega.Succeed())
	}
}

func (suite *Suite) deleteModuleCRsAndWaitNow(ctx context.Context) error {
	for _, mod := range suite.CleanupModules {
		cr := newModuleCR(mod.GVK)
		err := suite.Client.Delete(ctx, cr)
		switch {
		case k8serr.IsNotFound(err), apimeta.IsNoMatchError(err):
		case err != nil:
			return fmt.Errorf("deleting %s %s/%s: %w", mod.GVK.Kind, cr.GetNamespace(), cr.GetName(), err)
		}

		err = eventuallySucceeds(ctx, func() error {
			fresh := newModuleCR(mod.GVK)
			err := suite.Client.Get(ctx, client.ObjectKeyFromObject(fresh), fresh)
			switch {
			case k8serr.IsNotFound(err), apimeta.IsNoMatchError(err):
				return nil
			case err != nil:
				return err
			default:
				return fmt.Errorf("%s %s/%s still exists", mod.GVK.Kind, fresh.GetNamespace(), fresh.GetName())
			}
		})
		if err != nil {
			return fmt.Errorf("waiting for %s %s/%s deletion: %w", mod.GVK.Kind, cr.GetNamespace(), cr.GetName(), err)
		}
	}

	return nil
}

func (suite *Suite) snapshotClusterState(t *testing.T, ctx context.Context) cleanupSnapshot {
	t.Helper()
	g := gomega.NewWithT(t)

	snapshot := cleanupSnapshot{}
	seen := sets.New[configApi.ResourceRef]()

	var poList configApi.PlatformOperatorList
	err := suite.Client.List(ctx, &poList)
	if err != nil {
		g.Expect(err).NotTo(gomega.HaveOccurred())
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

	for _, mod := range suite.CleanupModules {
		cr := newModuleCR(mod.GVK)
		err := suite.Client.Get(ctx, client.ObjectKeyFromObject(cr), cr)
		switch {
		case k8serr.IsNotFound(err), apimeta.IsNoMatchError(err):
			continue
		case err != nil:
			g.Expect(err).NotTo(gomega.HaveOccurred())
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

func (suite *Suite) snapshotClusterStateNow(ctx context.Context) (cleanupSnapshot, error) {
	snapshot := cleanupSnapshot{}
	seen := sets.New[configApi.ResourceRef]()

	var poList configApi.PlatformOperatorList
	if err := suite.Client.List(ctx, &poList); err != nil {
		return cleanupSnapshot{}, err
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

	for _, mod := range suite.CleanupModules {
		cr := newModuleCR(mod.GVK)
		err := suite.Client.Get(ctx, client.ObjectKeyFromObject(cr), cr)
		switch {
		case k8serr.IsNotFound(err), apimeta.IsNoMatchError(err):
			continue
		case err != nil:
			return cleanupSnapshot{}, err
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

	return snapshot, nil
}

func eventuallySucceeds(ctx context.Context, actual func() error) error {
	tt := &suiteGomegaT{}
	g := gomega.NewWithT(tt)

	g.Eventually(actual).
		WithContext(ctx).
		WithTimeout(cleanupWaitTimeout).
		WithPolling(Interval).
		Should(gomega.Succeed())

	return tt.err
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
