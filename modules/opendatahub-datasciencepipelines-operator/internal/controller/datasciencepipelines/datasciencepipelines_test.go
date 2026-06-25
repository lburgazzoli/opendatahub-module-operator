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

package datasciencepipelines

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"

	. "github.com/onsi/gomega"
	common "github.com/opendatahub-io/odh-platform-utilities/api/common"
	fwapi "github.com/opendatahub-io/odh-platform-utilities/framework/api"
	fwtypes "github.com/opendatahub-io/odh-platform-utilities/framework/controller/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	componentApi "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-datasciencepipelines-operator/api/components/v1alpha1"
	moduleconfig "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-datasciencepipelines-operator/pkg/config"
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

func writeTestParamsEnv(t *testing.T) string {
	t.Helper()

	root := t.TempDir()
	dir := filepath.Join(root, componentName, "base")
	err := os.MkdirAll(dir, 0o755)
	NewWithT(t).Expect(err).NotTo(HaveOccurred())
	err = os.WriteFile(filepath.Join(dir, "params.env"), []byte(""), 0o600)
	NewWithT(t).Expect(err).NotTo(HaveOccurred())

	return root
}

func newTestModule(t *testing.T) *Module {
	t.Helper()

	cfg := testConfig(t)
	cfg.ManifestsPath = writeTestParamsEnv(t)
	cfg.ApplicationsNamespace = "test-ns"

	m, err := NewModule(cfg)
	NewWithT(t).Expect(err).NotTo(HaveOccurred())

	return m
}

func newTestRR(
	t *testing.T,
	obj *componentApi.DataSciencePipelines,
	manifestsBasePath string,
) *fwtypes.ReconciliationRequest {
	t.Helper()

	rel := testConfig(t).PlatformRelease()

	return &fwtypes.ReconciliationRequest{
		Instance:          obj,
		ManifestsBasePath: manifestsBasePath,
		Release: fwapi.Release{
			Name:    rel.Name,
			Version: rel.Version,
		},
	}
}

func newTestDataSciencePipelines() *componentApi.DataSciencePipelines {
	return &componentApi.DataSciencePipelines{
		ObjectMeta: metav1.ObjectMeta{
			Name: componentApi.DataSciencePipelinesInstanceName,
		},
	}
}

func TestNewModule(t *testing.T) {
	g := NewWithT(t)

	manifestsPath := writeTestParamsEnv(t)
	cfg := testConfig(t)
	cfg.ManifestsPath = manifestsPath

	m, err := NewModule(cfg)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(m.cfg).To(Equal(cfg))
	g.Expect(m.manifestInfo.Path).To(Equal(manifestsPath))
	g.Expect(m.manifestInfo.ContextDir).To(Equal(componentName))
	g.Expect(m.manifestInfo.SourcePath).To(Equal(overlayODH))
}

func TestInitialize(t *testing.T) {
	g := NewWithT(t)

	m := newTestModule(t)
	obj := newTestDataSciencePipelines()
	rr := newTestRR(t, obj, m.cfg.ManifestsPath)

	g.Expect(m.initialize(context.Background(), rr)).To(Succeed())
	g.Expect(rr.Manifests).To(HaveLen(1))
	g.Expect(rr.Manifests[0].Path).To(Equal(m.cfg.ManifestsPath))
	g.Expect(rr.Manifests[0].ContextDir).To(Equal(componentName))
	g.Expect(rr.Manifests[0].SourcePath).To(Equal(overlayODH))
}

func TestUpgradeIfNeededNoVersion(t *testing.T) {
	g := NewWithT(t)

	m := newTestModule(t)
	obj := newTestDataSciencePipelines()
	rr := newTestRR(t, obj, m.cfg.ManifestsPath)

	g.Expect(m.upgradeIfNeeded(context.Background(), rr)).To(Succeed())
}

func TestUpgradeIfNeededSameVersion(t *testing.T) {
	g := NewWithT(t)

	m := newTestModule(t)
	obj := newTestDataSciencePipelines()

	obj.Status.Releases = []common.ComponentRelease{
		{Name: moduleconfig.ReleasePlatform, Version: "1.0.0"},
	}
	rr := newTestRR(t, obj, m.cfg.ManifestsPath)

	g.Expect(m.upgradeIfNeeded(context.Background(), rr)).To(Succeed())
}

func TestReportStatus(t *testing.T) {
	g := NewWithT(t)

	m := newTestModule(t)
	obj := newTestDataSciencePipelines()
	rr := newTestRR(t, obj, m.cfg.ManifestsPath)

	g.Expect(m.initialize(context.Background(), rr)).To(Succeed())
	g.Expect(m.reportStatus(context.Background(), rr)).To(Succeed())

	g.Expect(obj.Status.Releases).To(ContainElement(
		common.ComponentRelease{Name: moduleconfig.ReleasePlatform, Version: "1.0.0"},
	))
}
