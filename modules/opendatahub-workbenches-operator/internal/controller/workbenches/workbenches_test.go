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

package workbenches

import (
	"context"
	"testing"
	"testing/fstest"

	. "github.com/onsi/gomega"
	common "github.com/opendatahub-io/odh-platform-utilities/api/common"
	fwtypes "github.com/opendatahub-io/odh-platform-utilities/framework/controller/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	componentApi "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-workbenches-operator/api/components/v1alpha1"
	moduleconfig "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-workbenches-operator/pkg/config"
	modulemeta "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-workbenches-operator/pkg/module"
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
	cfg.ManifestsPath = "/manifests"
	cfg.ApplicationsNamespace = "test-ns"
	m, err := NewModule(cfg)
	NewWithT(t).Expect(err).NotTo(HaveOccurred())
	return m
}

func newTestRR(t *testing.T, obj *componentApi.Workbenches) *fwtypes.ReconciliationRequest {
	t.Helper()

	rel := testConfig(t).PlatformRelease()

	return &fwtypes.ReconciliationRequest{
		Instance:          obj,
		ManifestsBasePath: "/manifests",
		Release:           rel,
	}
}

func newTestWorkbenches() *componentApi.Workbenches {
	return &componentApi.Workbenches{
		ObjectMeta: metav1.ObjectMeta{
			Name: componentApi.WorkbenchesInstanceName,
		},
	}
}

func TestStageManifests(t *testing.T) {
	g := NewWithT(t)

	m := newTestModule(t)
	obj := newTestWorkbenches()
	rr := newTestRR(t, obj)

	g.Expect(m.stageManifests(context.Background(), rr)).To(Succeed())
	// stageManifests sets 3 manifests: odh-notebook-controller, kf-notebook-controller, notebooks
	g.Expect(rr.Manifests).To(HaveLen(3))
	g.Expect(rr.Templates).To(HaveLen(1))
	g.Expect(rr.Manifests[0].Path).To(Equal(manifestsRoot))
	g.Expect(rr.Manifests[0].ContextDir).To(Equal(notebookControllerContextDir))
	g.Expect(rr.Manifests[1].ContextDir).To(Equal(kfNotebookControllerContextDir))
	g.Expect(rr.Manifests[2].ContextDir).To(Equal(notebookContextDir))
	g.Expect(rr.Manifests[2].SourcePath).To(Equal("odh/overlays/additional"))
	g.Expect(rr.Templates[0].Path).To(Equal("manifests/ext/openshift-config-grants.yaml.tmpl"))
}

func TestNewModuleUsesResolvedVariant(t *testing.T) {
	g := NewWithT(t)

	cfg := testConfig(t)

	m, err := NewModule(cfg)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(m.variant.Name).To(Equal(modulemeta.VariantODH))
	g.Expect(m.variant.Kustomize).To(HaveLen(4))
	g.Expect(m.variant.Kustomize[0].ManifestInfo.ContextDir).To(Equal(notebookControllerContextDir))
	g.Expect(m.variant.Kustomize[1].ManifestInfo.ContextDir).To(Equal(kfNotebookControllerContextDir))
	g.Expect(m.variant.Kustomize[2].ManifestInfo.ContextDir).To(Equal(notebookContextDir))
	g.Expect(m.variant.Kustomize[2].ManifestInfo.SourcePath).To(Equal("odh/overlays/additional"))
	g.Expect(m.variant.Kustomize[3].SkipRender).To(BeTrue())
	g.Expect(m.variant.Kustomize[3].ManifestInfo.SourcePath).To(Equal("odh/base"))
}

func TestNewModuleRhoaiVariant(t *testing.T) {
	g := NewWithT(t)

	cfg := testConfig(t)
	cfg.PlatformType = moduleconfig.PlatformTypeManagedRhoai

	m, err := NewModule(cfg)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(m.variant.Name).To(Equal(modulemeta.VariantRhoai))
	g.Expect(m.variant.Kustomize).To(HaveLen(4))
	g.Expect(m.variant.Kustomize[2].ManifestInfo.SourcePath).To(Equal("odh/overlays/additional"))
}

func TestMetadataFilePathUsesEmbeddedManifests(t *testing.T) {
	g := NewWithT(t)

	g.Expect(metadataFilePath(nil)).To(Equal(
		"manifests/workbenches/kf-notebook-controller/component_metadata.yaml",
	))
}

func TestReportStatus(t *testing.T) {
	g := NewWithT(t)

	m := newTestModule(t)
	g.Expect(m.Init()).To(Succeed())
	obj := newTestWorkbenches()
	rr := newTestRR(t, obj)

	g.Expect(m.stageManifests(context.Background(), rr)).To(Succeed())
	g.Expect(m.reportStatus(context.Background(), rr)).To(Succeed())

	g.Expect(obj.Status.Releases).To(ContainElement(
		common.ComponentRelease{Name: moduleconfig.ReleasePlatform, Version: "1.0.0"},
	))
	g.Expect(len(obj.Status.Releases)).To(BeNumerically(">", 1))
}
