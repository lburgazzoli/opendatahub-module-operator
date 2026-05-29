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
	"testing"

	. "github.com/onsi/gomega"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	componentApi "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-mymodule-operator/api/components/v1alpha1"
	moduleconfig "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-mymodule-operator/pkg/config"
	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-mymodule-operator/pkg/version"
	"github.com/opendatahub-io/opendatahub-operator/v2/pkg/cluster"
	actionapi "github.com/opendatahub-io/operator-actions-framework/api"
	"github.com/opendatahub-io/operator-actions-framework/controller/types"
)

const (
	testVersionOld = "1.0.0"
	testVersionNew = "2.0.0"
)

func newTestScheme(t *testing.T) *runtime.Scheme {
	t.Helper()

	s := runtime.NewScheme()

	g := NewWithT(t)
	g.Expect(networkingv1.AddToScheme(s)).To(Succeed())
	g.Expect(componentApi.AddToScheme(s)).To(Succeed())

	return s
}

func newTestModule(t *testing.T, platformType string) *Module {
	t.Helper()

	cfg := &moduleconfig.Config{
		PlatformType:          platformType,
		PlatformVersion:       "1.0.0",
		ManifestsPath:         "/manifests",
		ApplicationsNamespace: "test-ns",
	}

	m, err := NewModule(cfg)
	NewWithT(t).Expect(err).NotTo(HaveOccurred())

	return m
}

func newTestRR(obj *componentApi.MyModule) *types.ReconciliationRequest {
	rel := (&moduleconfig.Config{
		PlatformType:    string(cluster.OpenDataHub),
		PlatformVersion: "1.0.0",
	}).Release()

	return &types.ReconciliationRequest{
		Instance:          obj,
		ManifestsBasePath: "/manifests",
		Release:           actionapi.Release{Name: rel.Name, Version: rel.Version.Version},
	}
}

func newTestRRWithClient(obj *componentApi.MyModule, cl client.Client) *types.ReconciliationRequest {
	rel := (&moduleconfig.Config{
		PlatformType:    string(cluster.OpenDataHub),
		PlatformVersion: "1.0.0",
	}).Release()

	return &types.ReconciliationRequest{
		Client:            cl,
		Instance:          obj,
		ManifestsBasePath: "/manifests",
		Release:           actionapi.Release{Name: rel.Name, Version: rel.Version.Version},
	}
}

func newTestIngress(namespace string) *networkingv1.Ingress {
	return &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      IngressName,
			Namespace: namespace,
		},
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
	g.Expect(m.manifestInfo.Path).To(Equal(cfg.ManifestsPath))
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

	g.Expect(m.initialize(t.Context(), rr)).To(Succeed())
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

	g.Expect(m.initialize(t.Context(), rr)).To(Succeed())
	g.Expect(rr.Manifests).To(HaveLen(1))
	g.Expect(rr.Manifests[0].SourcePath).To(Equal(overlayRhoai))
}

func TestInitializeUnknownPlatformFallsBackToODH(t *testing.T) {
	g := NewWithT(t)

	m := newTestModule(t, "unknown")
	obj := newTestMyModule()
	rr := newTestRR(obj)

	g.Expect(m.initialize(t.Context(), rr)).To(Succeed())
	g.Expect(rr.Manifests).To(HaveLen(1))
	g.Expect(rr.Manifests[0].SourcePath).To(Equal(overlayODH))
}

func TestUpgradeIfNeededFreshInstall(t *testing.T) {
	g := NewWithT(t)

	m := newTestModule(t, string(cluster.OpenDataHub))
	obj := newTestMyModule()
	ingress := newTestIngress(m.cfg.ApplicationsNamespace)
	cl := fake.NewClientBuilder().WithScheme(newTestScheme(t)).WithObjects(ingress).Build()
	rr := newTestRRWithClient(obj, cl)

	// Fresh install: status version is zero, upgrade skipped.
	g.Expect(m.upgradeIfNeeded(t.Context(), rr)).To(Succeed())

	// Ingress must not have upgrade annotations.
	got := &networkingv1.Ingress{}
	g.Expect(cl.Get(t.Context(), client.ObjectKeyFromObject(ingress), got)).To(Succeed())
	g.Expect(got.Annotations).NotTo(HaveKey(AnnotationManagedVersion))
	g.Expect(got.Annotations).NotTo(HaveKey(AnnotationUpgradedFrom))
}

func TestUpgradeIfNeededSameVersion(t *testing.T) {
	g := NewWithT(t)

	m := newTestModule(t, string(cluster.OpenDataHub))
	obj := newTestMyModule()

	v, err := componentApi.NewSemVer(version.Version)
	g.Expect(err).NotTo(HaveOccurred())

	obj.Status.Module.Version = v

	ingress := newTestIngress(m.cfg.ApplicationsNamespace)
	cl := fake.NewClientBuilder().WithScheme(newTestScheme(t)).WithObjects(ingress).Build()
	rr := newTestRRWithClient(obj, cl)

	// Same version: no upgrade.
	g.Expect(m.upgradeIfNeeded(t.Context(), rr)).To(Succeed())

	// Ingress must not have upgrade annotations.
	got := &networkingv1.Ingress{}
	g.Expect(cl.Get(t.Context(), client.ObjectKeyFromObject(ingress), got)).To(Succeed())
	g.Expect(got.Annotations).NotTo(HaveKey(AnnotationManagedVersion))
	g.Expect(got.Annotations).NotTo(HaveKey(AnnotationUpgradedFrom))
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

	ingress := newTestIngress(m.cfg.ApplicationsNamespace)
	cl := fake.NewClientBuilder().WithScheme(newTestScheme(t)).WithObjects(ingress).Build()
	rr := newTestRRWithClient(obj, cl)

	// Platform version advanced: upgrade runs.
	g.Expect(m.upgradeIfNeeded(t.Context(), rr)).To(Succeed())

	// Ingress must have both upgrade annotations.
	// upgraded-from records the previous module version, not the platform version.
	got := &networkingv1.Ingress{}
	g.Expect(cl.Get(t.Context(), client.ObjectKeyFromObject(ingress), got)).To(Succeed())
	g.Expect(got.Annotations).To(HaveKeyWithValue(AnnotationManagedVersion, version.Version))
	g.Expect(got.Annotations).To(HaveKeyWithValue(AnnotationUpgradedFrom, version.Version))
}

func TestUpgradeIfNeededVersionAdvance(t *testing.T) {
	g := NewWithT(t)

	orig := version.Version
	version.Version = testVersionNew

	t.Cleanup(func() { version.Version = orig })

	m := newTestModule(t, string(cluster.OpenDataHub))
	obj := newTestMyModule()
	obj.Status.Module.Version = testVersionOld

	ingress := newTestIngress(m.cfg.ApplicationsNamespace)
	cl := fake.NewClientBuilder().WithScheme(newTestScheme(t)).WithObjects(ingress).Build()
	rr := newTestRRWithClient(obj, cl)

	// Version advanced: upgrade runs.
	g.Expect(m.upgradeIfNeeded(t.Context(), rr)).To(Succeed())

	// Ingress must have both upgrade annotations.
	got := &networkingv1.Ingress{}
	g.Expect(cl.Get(t.Context(), client.ObjectKeyFromObject(ingress), got)).To(Succeed())
	g.Expect(got.Annotations).To(HaveKeyWithValue(AnnotationManagedVersion, testVersionNew))
	g.Expect(got.Annotations).To(HaveKeyWithValue(AnnotationUpgradedFrom, testVersionOld))
}

func TestUpgradeIngressNotFound(t *testing.T) {
	g := NewWithT(t)

	orig := version.Version
	version.Version = testVersionNew

	t.Cleanup(func() { version.Version = orig })

	m := newTestModule(t, string(cluster.OpenDataHub))
	obj := newTestMyModule()
	obj.Status.Module.Version = testVersionOld

	// No Ingress in the fake client.
	cl := fake.NewClientBuilder().WithScheme(newTestScheme(t)).Build()
	rr := newTestRRWithClient(obj, cl)

	g.Expect(m.upgradeIfNeeded(t.Context(), rr)).To(Succeed())
}

func TestUpgradeFaultInjection(t *testing.T) {
	g := NewWithT(t)

	orig := version.Version
	version.Version = testVersionNew

	t.Cleanup(func() { version.Version = orig })

	m := newTestModule(t, string(cluster.OpenDataHub))
	obj := newTestMyModule()
	obj.Status.Module.Version = testVersionOld

	ingress := newTestIngress(m.cfg.ApplicationsNamespace)
	ingress.Annotations = map[string]string{
		AnnotationInjectUpgradeFault: "true",
	}

	cl := fake.NewClientBuilder().WithScheme(newTestScheme(t)).WithObjects(ingress).Build()
	rr := newTestRRWithClient(obj, cl)

	err := m.upgradeIfNeeded(t.Context(), rr)
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("upgrade fault injected"))
}

func TestReportStatus(t *testing.T) {
	g := NewWithT(t)

	m := newTestModule(t, string(cluster.OpenDataHub))
	obj := newTestMyModule()
	rr := newTestRR(obj)

	// Populate manifests as initialize would.
	g.Expect(m.initialize(t.Context(), rr)).To(Succeed())

	g.Expect(m.reportStatus(t.Context(), rr)).To(Succeed())

	g.Expect(obj.Status.Module.Version.String()).To(Equal(version.Version))
	g.Expect(obj.Status.Module.Platform.Name).To(Equal(string(cluster.OpenDataHub)))
	g.Expect(obj.Status.Module.Platform.Version.String()).To(Equal("1.0.0"))
	g.Expect(obj.Status.Module.Sources).To(HaveLen(1))
	g.Expect(obj.Status.Module.Sources[0].Renderer).To(Equal(componentApi.SourceRendererKustomize))

	g.Expect(obj.Status.ConfigValues).To(HaveKeyWithValue(moduleconfig.KeyPlatformType, string(cluster.OpenDataHub)))
	g.Expect(obj.Status.ConfigValues).To(HaveKeyWithValue(moduleconfig.KeyPlatformVersion, "1.0.0"))
}
