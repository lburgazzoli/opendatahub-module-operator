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
	"testing/fstest"

	. "github.com/onsi/gomega"
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
	module "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-modelregistry-operator/pkg/module"
	fwdeploy "github.com/opendatahub-io/odh-platform-utilities/framework/controller/actions/deploy"
)

func testConfig(t *testing.T) *moduleconfig.Config {
	t.Helper()
	cfg, err := moduleconfig.LoadFromFS(fstest.MapFS{
		moduleconfig.KeyPlatformType:    {Data: []byte("OpenDataHub")},
		moduleconfig.KeyPlatformVersion: {Data: []byte("1.0.0")},
	})
	NewWithT(t).Expect(err).NotTo(HaveOccurred())
	return cfg
}

func newTestModule(t *testing.T) *Module {
	t.Helper()

	cfg := testConfig(t)
	cfg.ApplicationsNamespace = "test-ns"

	m, err := NewModule(cfg)
	NewWithT(t).Expect(err).NotTo(HaveOccurred())

	return m
}

func newTestRR(obj *componentApi.ModelRegistry) *odhtypes.ReconciliationRequest {
	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)
	_ = componentApi.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)
	_ = rbacv1.AddToScheme(scheme)

	return &odhtypes.ReconciliationRequest{
		Client:   fake.NewClientBuilder().WithScheme(scheme).Build(),
		Instance: obj,
		Release:  fwapi.Release{},
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

	cfg := testConfig(t)

	m, err := NewModule(cfg)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(m.cfg).To(Equal(cfg))
	g.Expect(m.variant.Name).To(Equal(module.VariantODH))
	g.Expect(m.variant.Kustomize).To(HaveLen(2))
	g.Expect(m.variant.Kustomize[0].ManifestInfo.ContextDir).To(Equal(componentName))
	g.Expect(m.variant.Kustomize[0].ManifestInfo.SourcePath).To(Equal("overlays/odh"))
}

func TestStageManifests(t *testing.T) {
	g := NewWithT(t)

	m := newTestModule(t)
	obj := newTestModelRegistry()
	rr := newTestRR(obj)

	g.Expect(m.stageManifests(context.Background(), rr)).To(Succeed())
	g.Expect(rr.Manifests).To(HaveLen(2))
	g.Expect(rr.Templates).To(HaveLen(1))
	g.Expect(rr.Manifests[0].Path).To(Equal("manifests"))
	g.Expect(rr.Manifests[0].ContextDir).To(Equal(componentName))
	g.Expect(rr.Manifests[0].SourcePath).To(Equal("overlays/odh"))
	g.Expect(rr.Manifests[1].SourcePath).To(Equal("overlays/odh/extras"))
	g.Expect(rr.Templates[0].Path).To(Equal("manifests/ext/openshift-config-grants.yaml.tmpl"))
}

func TestInitLoadsReleases(t *testing.T) {
	g := NewWithT(t)

	m := newTestModule(t)

	g.Expect(m.Init()).To(Succeed())
	g.Expect(m.releases).To(Equal([]common.ComponentRelease{{Name: moduleconfig.ReleasePlatform, Version: "1.0.0"}}))
}

func TestComputeRuntimeParamsRequiresGatewayDomain(t *testing.T) {
	g := NewWithT(t)

	m := newTestModule(t)
	obj := newTestModelRegistry()

	params, err := m.computeRuntimeParams(obj)
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("spec.gateway.domain"))
	g.Expect(params).To(BeNil())
}

func TestConfigureDependenciesAddsNamespace(t *testing.T) {
	g := NewWithT(t)

	m := newTestModule(t)
	obj := newTestModelRegistry()
	obj.Spec.RegistriesNamespace = "odh-model-registries"
	rr := newTestRR(obj)

	g.Expect(m.configureDependencies(context.Background(), rr)).To(Succeed())
	g.Expect(rr.Resources).To(HaveLen(1))

	namespace := rr.Resources[0]
	g.Expect(namespace.GetKind()).To(Equal("Namespace"))
	g.Expect(namespace.GetName()).To(Equal("odh-model-registries"))
	g.Expect(namespace.GetAnnotations()).To(HaveKeyWithValue(fwdeploy.DefaultManagedByAnnotation, "false"))
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
		{Name: moduleconfig.ReleasePlatform, Version: "1.0.0"},
	}
	rr := newTestRR(obj)

	g.Expect(m.upgradeIfNeeded(context.Background(), rr)).To(Succeed())
}

func TestReportStatus(t *testing.T) {
	g := NewWithT(t)

	m := newTestModule(t)
	g.Expect(m.Init()).To(Succeed())
	obj := newTestModelRegistry()
	obj.Spec.RegistriesNamespace = "odh-model-registries"
	obj.Status.Releases = []common.ComponentRelease{
		{Name: "stale", Version: "0.1.0"},
	}
	rr := newTestRR(obj)

	g.Expect(m.stageManifests(context.Background(), rr)).To(Succeed())
	g.Expect(m.reportStatus(context.Background(), rr)).To(Succeed())

	g.Expect(obj.Status.RegistriesNamespace).To(Equal("odh-model-registries"))
	g.Expect(obj.Status.Releases).To(Equal([]common.ComponentRelease{{Name: moduleconfig.ReleasePlatform, Version: "1.0.0"}}))
}
