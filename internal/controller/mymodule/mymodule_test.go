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

package mymodule

import (
	"context"
	"testing"

	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	componentApi "github.com/lburgazzoli/opendatahub-module-operator/api/components/v1alpha1"
	moduleconfig "github.com/lburgazzoli/opendatahub-module-operator/pkg/config"
	"github.com/lburgazzoli/opendatahub-module-operator/pkg/version"
	"github.com/opendatahub-io/opendatahub-operator/v2/pkg/cluster"
	odhtypes "github.com/opendatahub-io/opendatahub-operator/v2/pkg/controller/types"
)

func newTestModule(t *testing.T, platformType string) *Module {
	t.Helper()

	cfg := &moduleconfig.Config{
		PlatformType:    platformType,
		PlatformVersion: "1.0.0",
	}

	m, err := NewModule(cfg)
	NewWithT(t).Expect(err).NotTo(HaveOccurred())

	return m
}

func newTestRR(obj *componentApi.MyModule) *odhtypes.ReconciliationRequest {
	return &odhtypes.ReconciliationRequest{
		Instance:          obj,
		ManifestsBasePath: "/manifests",
	}
}

func newTestMyModule() *componentApi.MyModule {
	return &componentApi.MyModule{
		ObjectMeta: metav1.ObjectMeta{
			Name: componentApi.MyModuleInstanceName,
		},
	}
}

func TestNewModule(t *testing.T) {
	g := NewWithT(t)

	cfg := &moduleconfig.Config{
		PlatformType:    string(cluster.OpenDataHub),
		PlatformVersion: "1.0.0",
	}

	m, err := NewModule(cfg)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(m.version.String()).To(Equal(version.Version))
	g.Expect(m.cfg).To(Equal(cfg))
	g.Expect(m.manifestInfo.ContextDir).To(Equal(componentName))
}

func TestNewModuleInvalidVersion(t *testing.T) {
	g := NewWithT(t)

	// Override version to something unparseable.
	orig := version.Version
	version.Version = "not-a-version"

	t.Cleanup(func() { version.Version = orig })

	_, err := NewModule(&moduleconfig.Config{})
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("invalid semver"))
}

func TestInitialize(t *testing.T) {
	g := NewWithT(t)

	m := newTestModule(t, string(cluster.OpenDataHub))
	obj := newTestMyModule()
	rr := newTestRR(obj)

	g.Expect(m.initialize(context.Background(), rr)).To(Succeed())
	g.Expect(rr.Manifests).To(HaveLen(1))
	g.Expect(rr.Manifests[0].Path).To(Equal("/manifests"))
	g.Expect(rr.Manifests[0].ContextDir).To(Equal(componentName))
	g.Expect(rr.Manifests[0].SourcePath).To(Equal(overlayODH))
}

func TestInitializeRHOAI(t *testing.T) {
	g := NewWithT(t)

	m := newTestModule(t, string(cluster.SelfManagedRhoai))
	obj := newTestMyModule()
	rr := newTestRR(obj)

	g.Expect(m.initialize(context.Background(), rr)).To(Succeed())
	g.Expect(rr.Manifests).To(HaveLen(1))
	g.Expect(rr.Manifests[0].SourcePath).To(Equal(overlayRhoai))
}

func TestInitializeUnknownPlatformFallsBackToODH(t *testing.T) {
	g := NewWithT(t)

	m := newTestModule(t, "unknown")
	obj := newTestMyModule()
	rr := newTestRR(obj)

	g.Expect(m.initialize(context.Background(), rr)).To(Succeed())
	g.Expect(rr.Manifests).To(HaveLen(1))
	g.Expect(rr.Manifests[0].SourcePath).To(Equal(overlayODH))
}

func TestValidateEnvironment(t *testing.T) {
	g := NewWithT(t)

	m := newTestModule(t, string(cluster.OpenDataHub))
	rr := newTestRR(newTestMyModule())

	g.Expect(m.validateEnvironment(context.Background(), rr)).To(Succeed())
}

func TestUpgradeIfNeededFreshInstall(t *testing.T) {
	g := NewWithT(t)

	m := newTestModule(t, string(cluster.OpenDataHub))
	obj := newTestMyModule()
	rr := newTestRR(obj)

	// Fresh install: status version is zero.
	g.Expect(m.upgradeIfNeeded(context.Background(), rr)).To(Succeed())
}

func TestUpgradeIfNeededSameVersion(t *testing.T) {
	g := NewWithT(t)

	m := newTestModule(t, string(cluster.OpenDataHub))
	obj := newTestMyModule()

	v, err := componentApi.NewSemVer(version.Version)
	g.Expect(err).NotTo(HaveOccurred())

	obj.Status.Module.Version = v
	rr := newTestRR(obj)

	// Same version: no upgrade.
	g.Expect(m.upgradeIfNeeded(context.Background(), rr)).To(Succeed())
}

func TestUpgradeIfNeededPlatformVersionChange(t *testing.T) {
	g := NewWithT(t)

	m := newTestModule(t, string(cluster.OpenDataHub))
	obj := newTestMyModule()

	v, err := componentApi.NewSemVer(version.Version)
	g.Expect(err).NotTo(HaveOccurred())

	prevPV, err := componentApi.NewSemVer("0.9.0")
	g.Expect(err).NotTo(HaveOccurred())

	// Same module version but older platform version in status.
	obj.Status.Module.Version = v
	obj.Status.Module.Platform.Version = prevPV
	rr := newTestRR(obj)

	// Platform version advanced: upgrade runs (no-op migrations, no error).
	g.Expect(m.upgradeIfNeeded(context.Background(), rr)).To(Succeed())
}

func TestUpgradeIfNeededVersionAdvance(t *testing.T) {
	g := NewWithT(t)

	orig := version.Version
	version.Version = "2.0.0"

	t.Cleanup(func() { version.Version = orig })

	m := newTestModule(t, string(cluster.OpenDataHub))
	obj := newTestMyModule()
	obj.Status.Module.Version = "1.0.0"
	rr := newTestRR(obj)

	// Version advanced: upgrade runs (no-op migrations, no error).
	g.Expect(m.upgradeIfNeeded(context.Background(), rr)).To(Succeed())
}

func TestReportStatus(t *testing.T) {
	g := NewWithT(t)

	m := newTestModule(t, string(cluster.OpenDataHub))
	obj := newTestMyModule()
	rr := newTestRR(obj)

	// Populate manifests as initialize would.
	g.Expect(m.initialize(context.Background(), rr)).To(Succeed())

	g.Expect(m.reportStatus(context.Background(), rr)).To(Succeed())

	g.Expect(obj.Status.Module.Version.String()).To(Equal(version.Version))
	g.Expect(obj.Status.Module.Platform.Name).To(Equal(string(cluster.OpenDataHub)))
	g.Expect(obj.Status.Module.Platform.Version.String()).To(Equal("1.0.0"))
	g.Expect(obj.Status.Module.Sources).To(HaveLen(1))
	g.Expect(obj.Status.Module.Sources[0].Renderer).To(Equal(componentApi.SourceRendererKustomize))

	g.Expect(obj.Status.ConfigValues).To(HaveKeyWithValue(moduleconfig.KeyPlatformType, string(cluster.OpenDataHub)))
	g.Expect(obj.Status.ConfigValues).To(HaveKeyWithValue(moduleconfig.KeyPlatformVersion, "1.0.0"))
}
