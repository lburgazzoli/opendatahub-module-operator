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

package ray

import (
	"context"
	"testing"
	"testing/fstest"

	. "github.com/onsi/gomega"
	common "github.com/opendatahub-io/odh-platform-utilities/api/common"
	fwtypes "github.com/opendatahub-io/odh-platform-utilities/framework/controller/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	componentApi "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-ray-operator/api/components/v1alpha1"
	moduleconfig "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-ray-operator/pkg/config"
	module "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-ray-operator/pkg/module"
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
	NewWithT(t).Expect(m.Init()).To(Succeed())

	return m
}

func newTestRR(t *testing.T, obj *componentApi.Ray) *fwtypes.ReconciliationRequest {
	t.Helper()

	cfg := testConfig(t)
	return &fwtypes.ReconciliationRequest{
		Instance: obj,
		Release:  cfg.PlatformRelease(),
	}
}

func newTestRay() *componentApi.Ray {
	return &componentApi.Ray{
		ObjectMeta: metav1.ObjectMeta{
			Name: componentApi.RayInstanceName,
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
	g.Expect(m.variant.Kustomize).To(HaveLen(1))
	g.Expect(m.variant.Kustomize[0].ManifestInfo.ContextDir).To(Equal(componentName))
	g.Expect(m.variant.Kustomize[0].ManifestInfo.SourcePath).To(Equal("openshift"))
}

func TestNewModuleRhoaiVariant(t *testing.T) {
	g := NewWithT(t)

	cfg := testConfig(t)
	cfg.PlatformType = moduleconfig.PlatformTypeManagedRhoai

	m, err := NewModule(cfg)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(m.variant.Name).To(Equal(module.VariantRhoai))
	g.Expect(m.variant.Kustomize[0].ManifestInfo.SourcePath).To(Equal("openshift"))
}

func TestStageManifests(t *testing.T) {
	g := NewWithT(t)

	m := newTestModule(t)
	obj := newTestRay()
	rr := newTestRR(t, obj)

	g.Expect(m.stageManifests(context.Background(), rr)).To(Succeed())
	g.Expect(rr.Manifests).To(HaveLen(1))
	g.Expect(rr.Manifests[0].Path).To(Equal("manifests"))
	g.Expect(rr.Manifests[0].ContextDir).To(Equal(componentName))
	g.Expect(rr.Manifests[0].SourcePath).To(Equal("openshift"))
	g.Expect(rr.Templates).To(HaveLen(1))
	g.Expect(rr.Templates[0].Path).To(Equal("manifests/ext/openshift-config-grants.yaml.tmpl"))
}

func TestUpgradeIfNeededNoVersion(t *testing.T) {
	g := NewWithT(t)

	m := newTestModule(t)
	obj := newTestRay()
	rr := newTestRR(t, obj)

	g.Expect(m.upgradeIfNeeded(context.Background(), rr)).To(Succeed())
}

func TestUpgradeIfNeededSameVersion(t *testing.T) {
	g := NewWithT(t)

	m := newTestModule(t)
	obj := newTestRay()

	obj.Status.Releases = []common.ComponentRelease{
		{Name: moduleconfig.ReleasePlatform, Version: "1.0.0"},
	}
	rr := newTestRR(t, obj)

	g.Expect(m.upgradeIfNeeded(context.Background(), rr)).To(Succeed())
}

func TestReportStatus(t *testing.T) {
	g := NewWithT(t)

	m := newTestModule(t)
	g.Expect(m.Init()).To(Succeed())
	obj := newTestRay()
	rr := newTestRR(t, obj)

	g.Expect(m.stageManifests(context.Background(), rr)).To(Succeed())
	g.Expect(m.reportStatus(context.Background(), rr)).To(Succeed())

	g.Expect(obj.Status.Releases).To(Equal(m.releases))
}
