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

	"github.com/blang/semver/v4"
	fwapi "github.com/opendatahub-io/odh-platform-utilities/framework/api"
	fwtypes "github.com/opendatahub-io/odh-platform-utilities/framework/controller/types"
	ofVersion "github.com/operator-framework/api/pkg/lib/version"

	componentApi "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-datasciencepipelines-operator/api/components/v1alpha1"
	moduleconfig "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-datasciencepipelines-operator/pkg/config"
	. "github.com/onsi/gomega"
	"github.com/opendatahub-io/odh-platform-utilities/pkg/cluster"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

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

	cfg := &moduleconfig.Config{
		PlatformName:          "OpenDataHub",
		PlatformVersion:       "1.0.0",
		ManifestsPath:         writeTestParamsEnv(t),
		ApplicationsNamespace: "test-ns",
	}

	m, err := NewModule(cfg)
	NewWithT(t).Expect(err).NotTo(HaveOccurred())

	return m
}

func newTestRR(
	obj *componentApi.DataSciencePipelines,
	manifestsBasePath string,
) *fwtypes.ReconciliationRequest {
	rel := (&moduleconfig.Config{
		PlatformName:    string(cluster.OpenDataHub),
		PlatformVersion: "1.0.0",
	}).Release()

	return &fwtypes.ReconciliationRequest{
		Instance:          obj,
		ManifestsBasePath: manifestsBasePath,
		Release: fwapi.Release{
			Name:    fwapi.Platform(rel.Name),
			Version: rel.Version.Version,
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
	cfg := &moduleconfig.Config{
		PlatformName:    string(cluster.OpenDataHub),
		PlatformVersion: "1.0.0",
		ManifestsPath:   manifestsPath,
	}

	m, err := NewModule(cfg)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(m.cfg).To(Equal(cfg))
	g.Expect(m.manifestInfo.Path).To(Equal(manifestsPath))
	g.Expect(m.manifestInfo.ContextDir).To(Equal(componentName))
	g.Expect(m.manifestInfo.SourcePath).To(Equal(overlayODH))
}

func TestNewModuleSelectsRhoaiOverlay(t *testing.T) {
	g := NewWithT(t)

	cfg := &moduleconfig.Config{
		PlatformName:    string(cluster.SelfManagedRhoai),
		PlatformVersion: "1.0.0",
		ManifestsPath:   writeTestParamsEnv(t),
	}

	m, err := NewModule(cfg)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(m.manifestInfo.SourcePath).To(Equal(overlayRhoai))
}

func TestInitialize(t *testing.T) {
	g := NewWithT(t)

	m := newTestModule(t)
	obj := newTestDataSciencePipelines()
	rr := newTestRR(obj, m.cfg.ManifestsPath)

	g.Expect(m.initialize(context.Background(), rr)).To(Succeed())
	g.Expect(rr.Manifests).To(HaveLen(1))
	g.Expect(rr.Manifests[0].Path).To(Equal(m.cfg.ManifestsPath))
	g.Expect(rr.Manifests[0].ContextDir).To(Equal(componentName))
	g.Expect(rr.Manifests[0].SourcePath).To(Equal(overlayODH))
}

func TestUpgradeIfNeededFreshInstall(t *testing.T) {
	g := NewWithT(t)

	m := newTestModule(t)
	obj := newTestDataSciencePipelines()
	rr := newTestRR(obj, m.cfg.ManifestsPath)

	g.Expect(m.upgradeIfNeeded(context.Background(), rr)).To(Succeed())
}

func TestUpgradeIfNeededSameVersion(t *testing.T) {
	g := NewWithT(t)

	m := newTestModule(t)
	obj := newTestDataSciencePipelines()
	obj.Status.Release.Version = ofVersion.OperatorVersion{Version: semver.MustParse("1.0.0")}
	rr := newTestRR(obj, m.cfg.ManifestsPath)

	g.Expect(m.upgradeIfNeeded(context.Background(), rr)).To(Succeed())
}

func TestReportStatus(t *testing.T) {
	g := NewWithT(t)

	m := newTestModule(t)
	obj := newTestDataSciencePipelines()
	rr := newTestRR(obj, m.cfg.ManifestsPath)

	g.Expect(m.initialize(context.Background(), rr)).To(Succeed())
	g.Expect(m.reportStatus(context.Background(), rr)).To(Succeed())

	g.Expect(obj.Status.Release.Version.String()).To(Equal("1.0.0"))
	g.Expect(string(obj.Status.Release.Name)).To(Equal(string(cluster.OpenDataHub)))
}
