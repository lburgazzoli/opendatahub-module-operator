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

package ogx

import (
	"context"
	"testing"

	. "github.com/onsi/gomega"
	common "github.com/opendatahub-io/odh-platform-utilities/api/common"
	fwapi "github.com/opendatahub-io/odh-platform-utilities/framework/api"
	odhtypes "github.com/opendatahub-io/odh-platform-utilities/framework/controller/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	componentApi "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-ogx-operator/api/components/v1alpha1"
	moduleconfig "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-ogx-operator/pkg/config"
	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-ogx-operator/pkg/releases"
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

func newTestRR(t *testing.T, obj *componentApi.OGX) *odhtypes.ReconciliationRequest {
	t.Helper()

	rel := (&moduleconfig.Config{PlatformVersion: "1.0.0"}).Release()

	v, err := releases.ParseVersion(rel.Version)
	NewWithT(t).Expect(err).NotTo(HaveOccurred())

	return &odhtypes.ReconciliationRequest{
		Instance:          obj,
		ManifestsBasePath: "/manifests",
		Release: fwapi.Release{
			Name:    fwapi.Platform(rel.Name),
			Version: v,
		},
	}
}

func newTestOGX() *componentApi.OGX {
	return &componentApi.OGX{
		ObjectMeta: metav1.ObjectMeta{
			Name: componentApi.OGXInstanceName,
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
	g.Expect(m.manifestInfo.SourcePath).To(Equal(overlayODH))
}

func TestInitialize(t *testing.T) {
	g := NewWithT(t)

	m := newTestModule(t)
	obj := newTestOGX()
	rr := newTestRR(t, obj)

	g.Expect(m.initialize(context.Background(), rr)).To(Succeed())
	g.Expect(rr.Manifests).To(HaveLen(1))
	g.Expect(rr.Manifests[0].Path).To(Equal("/manifests"))
	g.Expect(rr.Manifests[0].ContextDir).To(Equal(componentName))
	g.Expect(rr.Manifests[0].SourcePath).To(Equal(overlayODH))
}

func TestUpgradeIfNeededNoVersion(t *testing.T) {
	g := NewWithT(t)

	m := newTestModule(t)
	obj := newTestOGX()
	rr := newTestRR(t, obj)

	g.Expect(m.upgradeIfNeeded(context.Background(), rr)).To(Succeed())
}

func TestUpgradeIfNeededSameVersion(t *testing.T) {
	g := NewWithT(t)

	m := newTestModule(t)
	obj := newTestOGX()

	obj.Status.Releases = []common.ComponentRelease{
		{Name: releases.Platform, Version: "1.0.0"},
	}
	rr := newTestRR(t, obj)

	g.Expect(m.upgradeIfNeeded(context.Background(), rr)).To(Succeed())
}

func TestReportStatus(t *testing.T) {
	g := NewWithT(t)

	m := newTestModule(t)
	obj := newTestOGX()
	rr := newTestRR(t, obj)

	g.Expect(m.initialize(context.Background(), rr)).To(Succeed())
	g.Expect(m.reportStatus(context.Background(), rr)).To(Succeed())

	g.Expect(obj.Status.Releases).To(ContainElement(
		common.ComponentRelease{Name: releases.Platform, Version: "1.0.0"},
	))
}
