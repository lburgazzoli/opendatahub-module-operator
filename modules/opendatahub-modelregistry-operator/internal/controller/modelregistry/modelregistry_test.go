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

package modelregistry

import (
	"context"
	"testing"

	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	common "github.com/opendatahub-io/odh-platform-utilities/api/common"
	fwapi "github.com/opendatahub-io/odh-platform-utilities/framework/api"
	odhtypes "github.com/opendatahub-io/odh-platform-utilities/framework/controller/types"

	componentApi "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-modelregistry-operator/api/components/v1alpha1"
	moduleconfig "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-modelregistry-operator/pkg/config"
	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-modelregistry-operator/pkg/releases"
	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-modelregistry-operator/pkg/resources/gvk"
	fwdeploy "github.com/opendatahub-io/odh-platform-utilities/framework/controller/actions/deploy"
)

func newTestModule(t *testing.T) *Module {
	t.Helper()

	cfg := &moduleconfig.Config{
		PlatformVersion:       "1.0.0",
		ManifestsPath:         "/manifests",
		ApplicationsNamespace: "test-ns",
	}

	m, err := NewModule(cfg)
	NewWithT(t).Expect(err).NotTo(HaveOccurred())

	return m
}

func newTestRR(obj *componentApi.ModelRegistry) *odhtypes.ReconciliationRequest {
	rel := (&moduleconfig.Config{PlatformVersion: "1.0.0"}).Release()

	v, err := releases.ParseVersion(rel.Version)
	if err != nil {
		panic(err)
	}

	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)
	_ = componentApi.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)
	_ = rbacv1.AddToScheme(scheme)

	return &odhtypes.ReconciliationRequest{
		Client:            fake.NewClientBuilder().WithScheme(scheme).Build(),
		Instance:          obj,
		ManifestsBasePath: "/manifests",
		Release: fwapi.Release{
			Name:    fwapi.Platform(rel.Name),
			Version: v,
		},
	}
}

func newTestModelRegistry() *componentApi.ModelRegistry {
	return &componentApi.ModelRegistry{
		ObjectMeta: metav1.ObjectMeta{
			Name: componentApi.ModelRegistryInstanceName,
		},
	}
}

func TestNewModule(t *testing.T) {
	g := NewWithT(t)

	cfg := &moduleconfig.Config{
		PlatformVersion: "1.0.0",
		ManifestsPath:   "/manifests",
	}

	m, err := NewModule(cfg)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(m.cfg).To(Equal(cfg))
	g.Expect(m.manifestInfo.ContextDir).To(Equal(componentName))
	g.Expect(m.manifestInfo.SourcePath).To(Equal(baseManifestsSourcePath))
}

func TestInitialize(t *testing.T) {
	g := NewWithT(t)

	m := newTestModule(t)
	obj := newTestModelRegistry()
	rr := newTestRR(obj)

	g.Expect(m.initialize(context.Background(), rr)).To(Succeed())
	g.Expect(rr.Manifests).To(HaveLen(2))
	g.Expect(rr.Manifests[0].Path).To(Equal("/manifests"))
	g.Expect(rr.Manifests[0].ContextDir).To(Equal(componentName))
	g.Expect(rr.Manifests[0].SourcePath).To(Equal(baseManifestsSourcePath))
}

func TestConfigureDependenciesAddsNamespaceAndOpenShiftRBAC(t *testing.T) {
	g := NewWithT(t)

	m := newTestModule(t)
	obj := newTestModelRegistry()
	obj.Spec.RegistriesNamespace = "odh-model-registries"
	rr := newTestRR(obj)
	g.Expect(rr.AddResources(&appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "rendered-controller",
			Namespace: "test-ns",
		},
		Spec: appsv1.DeploymentSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					ServiceAccountName: "rendered-controller-sa",
				},
			},
		},
	})).To(Succeed())

	g.Expect(m.configureDependencies(context.Background(), rr)).To(Succeed())
	g.Expect(rr.Resources).To(HaveLen(4))

	namespace := rr.Resources[1]
	g.Expect(namespace.GetKind()).To(Equal("Namespace"))
	g.Expect(namespace.GetName()).To(Equal("odh-model-registries"))
	g.Expect(namespace.GetAnnotations()).To(HaveKeyWithValue(fwdeploy.DefaultManagedByAnnotation, "false"))

	clusterRole := rr.Resources[2]
	g.Expect(clusterRole.GetKind()).To(Equal("ClusterRole"))
	g.Expect(clusterRole.GetName()).To(Equal(openShiftAPIServerReaderRoleName))
	g.Expect(clusterRole.Object["rules"]).To(Equal([]any{
		map[string]any{
			"apiGroups": []any{"config.openshift.io"},
			"resources": []any{"apiservers"},
			"verbs":     []any{"get", "list", "watch"},
		},
	}))

	clusterRoleBinding := rr.Resources[3]
	g.Expect(clusterRoleBinding.GetKind()).To(Equal("ClusterRoleBinding"))
	g.Expect(clusterRoleBinding.GetName()).To(Equal(openShiftAPIServerReaderRoleBindingName))
	g.Expect(clusterRoleBinding.Object["roleRef"]).To(Equal(map[string]any{
		"apiGroup": gvk.ClusterRole.Group,
		"kind":     gvk.ClusterRole.Kind,
		"name":     openShiftAPIServerReaderRoleName,
	}))
	g.Expect(clusterRoleBinding.Object["subjects"]).To(Equal([]any{
		map[string]any{
			"kind":      gvk.ServiceAccount.Kind,
			"name":      "rendered-controller-sa",
			"namespace": "test-ns",
		},
	}))
}

func TestUpgradeIfNeededNoVersion(t *testing.T) {
	g := NewWithT(t)

	m := newTestModule(t)
	obj := newTestModelRegistry()
	rr := newTestRR(obj)

	g.Expect(m.upgradeIfNeeded(context.Background(), rr)).To(Succeed())
}

func TestUpgradeIfNeededSameVersion(t *testing.T) {
	g := NewWithT(t)

	m := newTestModule(t)
	obj := newTestModelRegistry()

	obj.Status.Releases = []common.ComponentRelease{
		{Name: releases.Platform, Version: "1.0.0"},
	}
	rr := newTestRR(obj)

	g.Expect(m.upgradeIfNeeded(context.Background(), rr)).To(Succeed())
}

func TestReportStatus(t *testing.T) {
	g := NewWithT(t)

	m := newTestModule(t)
	obj := newTestModelRegistry()
	rr := newTestRR(obj)

	g.Expect(m.initialize(context.Background(), rr)).To(Succeed())
	g.Expect(m.reportStatus(context.Background(), rr)).To(Succeed())

	g.Expect(obj.Status.Releases).To(ContainElement(
		common.ComponentRelease{Name: releases.Platform, Version: "1.0.0"},
	))
}
