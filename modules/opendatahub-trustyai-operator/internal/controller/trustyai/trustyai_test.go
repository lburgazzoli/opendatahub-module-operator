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

package trustyai

import (
	"context"
	"testing"
	"testing/fstest"

	componentApi "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-trustyai-operator/api/components/v1alpha1"
	moduleconfig "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-trustyai-operator/pkg/config"
	modulemeta "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-trustyai-operator/pkg/module"
	. "github.com/onsi/gomega"
	common "github.com/opendatahub-io/odh-platform-utilities/api/common"
	odhtypes "github.com/opendatahub-io/odh-platform-utilities/framework/controller/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

func newTestRR(t *testing.T, obj *componentApi.TrustyAI) *odhtypes.ReconciliationRequest {
	t.Helper()

	cfg := testConfig(t)
	return &odhtypes.ReconciliationRequest{
		Instance: obj,
		Release:  cfg.PlatformRelease(),
	}
}

func newTestTrustyAI() *componentApi.TrustyAI {
	return &componentApi.TrustyAI{
		ObjectMeta: metav1.ObjectMeta{
			Name: componentApi.TrustyAIInstanceName,
		},
	}
}

func TestNewModule(t *testing.T) {
	g := NewWithT(t)

	cfg := testConfig(t)

	m, err := NewModule(cfg)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(m.cfg).To(Equal(cfg))
	g.Expect(m.variant.Name).To(Equal(modulemeta.VariantODH))
	g.Expect(m.variant.Kustomize).To(HaveLen(1))
	g.Expect(m.variant.Kustomize[0].ManifestInfo.ContextDir).To(Equal(componentName))
	g.Expect(m.variant.Kustomize[0].ManifestInfo.SourcePath).To(Equal("overlays/odh"))
	g.Expect(m.variant.Kustomize[0].ManifestInfo.Path).To(Equal("manifests"))
	g.Expect(m.mcpVariant.Name).To(Equal(modulemeta.VariantMCPGuardrails))
	g.Expect(m.mcpVariant.Kustomize[0].ManifestInfo.SourcePath).To(Equal("overlays/mcp-guardrails"))
}

func TestNewModuleRhoaiVariant(t *testing.T) {
	g := NewWithT(t)

	cfg := testConfig(t)
	cfg.PlatformType = moduleconfig.PlatformTypeManagedRhoai

	m, err := NewModule(cfg)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(m.variant.Name).To(Equal(modulemeta.VariantRhoai))
	g.Expect(m.variant.Kustomize[0].ManifestInfo.SourcePath).To(Equal("overlays/rhoai"))
}

func TestStageManifests(t *testing.T) {
	g := NewWithT(t)

	m := newTestModule(t)
	obj := newTestTrustyAI()
	rr := newTestRR(t, obj)

	g.Expect(m.stageManifests(context.Background(), rr)).To(Succeed())
	g.Expect(rr.Manifests).To(HaveLen(1))
	g.Expect(rr.Manifests[0].Path).To(Equal("manifests"))
	g.Expect(rr.Manifests[0].ContextDir).To(Equal(componentName))
	g.Expect(rr.Manifests[0].SourcePath).To(Equal("overlays/odh"))
	g.Expect(rr.Templates).To(HaveLen(1))
	g.Expect(rr.Templates[0].Path).To(Equal("manifests/ext/openshift-config-grants.yaml.tmpl"))
}

func TestStageManifestsMCPGuardrailsMode(t *testing.T) {
	g := NewWithT(t)

	m := newTestModule(t)
	obj := newTestTrustyAI()
	obj.Spec.MCPGuardrailsMode = true
	rr := newTestRR(t, obj)

	g.Expect(m.stageManifests(context.Background(), rr)).To(Succeed())
	g.Expect(rr.Manifests).To(HaveLen(1))
	g.Expect(rr.Manifests[0].SourcePath).To(Equal("overlays/mcp-guardrails"))
	g.Expect(rr.Templates).To(HaveLen(1))
	g.Expect(rr.Templates[0].Path).To(Equal("manifests/ext/openshift-config-grants.yaml.tmpl"))
}

func TestInitLoadsReleases(t *testing.T) {
	g := NewWithT(t)

	m := newTestModule(t)

	g.Expect(m.Init()).To(Succeed())
	g.Expect(m.releases).ToNot(BeEmpty())
	g.Expect(m.releases).To(ContainElement(common.ComponentRelease{
		Name:    "TrustyAI operator",
		Version: "v1.37.0",
		RepoURL: "https://github.com/trustyai-explainability/trustyai-service-operator",
	}))
	g.Expect(m.releases).To(ContainElement(common.ComponentRelease{
		Name:    moduleconfig.ReleasePlatform,
		Version: "1.0.0",
	}))
}

func TestUpgradeIfNeededNoVersion(t *testing.T) {
	g := NewWithT(t)

	m := newTestModule(t)
	obj := newTestTrustyAI()
	rr := newTestRR(t, obj)

	g.Expect(m.upgradeIfNeeded(context.Background(), rr)).To(Succeed())
}

func TestUpgradeIfNeededSameVersion(t *testing.T) {
	g := NewWithT(t)

	m := newTestModule(t)
	obj := newTestTrustyAI()

	obj.Status.Releases = []common.ComponentRelease{
		{Name: moduleconfig.ReleasePlatform, Version: "1.0.0"},
	}
	rr := newTestRR(t, obj)

	g.Expect(m.upgradeIfNeeded(context.Background(), rr)).To(Succeed())
}

func TestReportStatus(t *testing.T) {
	g := NewWithT(t)

	m := newTestModule(t)
	obj := newTestTrustyAI()
	rr := newTestRR(t, obj)

	g.Expect(m.Init()).To(Succeed())
	g.Expect(m.stageManifests(context.Background(), rr)).To(Succeed())
	g.Expect(m.reportStatus(context.Background(), rr)).To(Succeed())

	g.Expect(obj.Status.Releases).To(ContainElement(common.ComponentRelease{
		Name:    "TrustyAI operator",
		Version: "v1.37.0",
		RepoURL: "https://github.com/trustyai-explainability/trustyai-service-operator",
	}))
	g.Expect(obj.Status.Releases).To(ContainElement(common.ComponentRelease{
		Name:    moduleconfig.ReleasePlatform,
		Version: "1.0.0",
	}))
	g.Expect(obj.Status.Releases).To(Equal(m.releases))
}
