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

package feastoperator

import (
	"context"
	"testing"

	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/blang/semver/v4"
	ofVersion "github.com/operator-framework/api/pkg/lib/version"

	componentApi "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-feast-operator/api/components/v1alpha1"
	moduleconfig "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-feast-operator/pkg/config"
	actionapi "github.com/opendatahub-io/odh-platform-utilities/framework/api"
	odhtypes "github.com/opendatahub-io/odh-platform-utilities/framework/controller/types"
	"github.com/opendatahub-io/odh-platform-utilities/pkg/cluster"
)

func newTestModule(t *testing.T) *Module {
	t.Helper()

	cfg := &moduleconfig.Config{
		PlatformName:          string(cluster.OpenDataHub),
		PlatformVersion:       "1.0.0",
		ManifestsPath:         "/manifests",
		ApplicationsNamespace: "test-ns",
	}

	m, err := NewModule(cfg)
	NewWithT(t).Expect(err).NotTo(HaveOccurred())

	return m
}

func newTestRR(obj *componentApi.FeastOperator) *odhtypes.ReconciliationRequest {
	rel := (&moduleconfig.Config{
		PlatformName:    string(cluster.OpenDataHub),
		PlatformVersion: "1.0.0",
	}).Release()

	return &odhtypes.ReconciliationRequest{
		Instance:          obj,
		ManifestsBasePath: "/manifests",
		Release: actionapi.Release{
			Name:    actionapi.Platform(rel.Name),
			Version: rel.Version.Version,
		},
	}
}

func newTestFeastOperator() *componentApi.FeastOperator {
	return &componentApi.FeastOperator{
		ObjectMeta: metav1.ObjectMeta{
			Name: componentApi.FeastOperatorInstanceName,
		},
	}
}

func TestNewModule(t *testing.T) {
	g := NewWithT(t)

	cfg := &moduleconfig.Config{
		PlatformName:    string(cluster.OpenDataHub),
		PlatformVersion: "1.0.0",
		ManifestsPath:   "/manifests",
	}

	m, err := NewModule(cfg)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(m.cfg).To(Equal(cfg))
	g.Expect(m.manifestInfo.Path).To(Equal(cfg.ManifestsPath))
	g.Expect(m.manifestInfo.ContextDir).To(Equal(componentName))
	g.Expect(m.manifestInfo.SourcePath).To(Equal(overlayODH))
}

func TestInitialize(t *testing.T) {
	g := NewWithT(t)

	m := newTestModule(t)
	obj := newTestFeastOperator()
	rr := newTestRR(obj)

	g.Expect(m.initialize(context.Background(), rr)).To(Succeed())
	g.Expect(rr.Manifests).To(HaveLen(1))
	g.Expect(rr.Manifests[0].Path).To(Equal("/manifests"))
	g.Expect(rr.Manifests[0].ContextDir).To(Equal(componentName))
	g.Expect(rr.Manifests[0].SourcePath).To(Equal(overlayODH))
}

func TestUpgradeIfNeededFreshInstall(t *testing.T) {
	g := NewWithT(t)

	m := newTestModule(t)
	obj := newTestFeastOperator()
	rr := newTestRR(obj)

	g.Expect(m.upgradeIfNeeded(context.Background(), rr)).To(Succeed())
}

func TestUpgradeIfNeededSameVersion(t *testing.T) {
	g := NewWithT(t)

	m := newTestModule(t)
	obj := newTestFeastOperator()
	obj.Status.Release.Version = ofVersion.OperatorVersion{Version: semver.MustParse("1.0.0")}
	rr := newTestRR(obj)

	g.Expect(m.upgradeIfNeeded(context.Background(), rr)).To(Succeed())
}

func TestReportStatus(t *testing.T) {
	g := NewWithT(t)

	m := newTestModule(t)
	obj := newTestFeastOperator()
	rr := newTestRR(obj)

	g.Expect(m.initialize(context.Background(), rr)).To(Succeed())
	g.Expect(m.reportStatus(context.Background(), rr)).To(Succeed())

	g.Expect(obj.Status.Release.Version.String()).To(Equal("1.0.0"))
	g.Expect(string(obj.Status.Release.Name)).To(Equal(string(cluster.OpenDataHub)))
}

func TestCustomizeManifestsNoOIDC(t *testing.T) {
	g := NewWithT(t)

	m := newTestModule(t)
	obj := newTestFeastOperator()
	// No OIDC set on the CR
	rr := newTestRR(obj)
	g.Expect(m.initialize(context.Background(), rr)).To(Succeed())

	// Should succeed and write empty OIDC_ISSUER_URL
	g.Expect(m.customizeManifests(context.Background(), rr)).To(Succeed())
}

func TestCustomizeManifestsWithOIDC(t *testing.T) {
	g := NewWithT(t)

	m := newTestModule(t)
	obj := newTestFeastOperator()
	obj.Spec.OIDC = &componentApi.GatewayOIDCSpec{IssuerURL: "https://issuer.example.com"}
	rr := newTestRR(obj)
	g.Expect(m.initialize(context.Background(), rr)).To(Succeed())

	g.Expect(m.customizeManifests(context.Background(), rr)).To(Succeed())
}

func TestCustomizeManifestsInvalidOIDC(t *testing.T) {
	g := NewWithT(t)

	m := newTestModule(t)
	obj := newTestFeastOperator()
	obj.Spec.OIDC = &componentApi.GatewayOIDCSpec{IssuerURL: "not-a-url"}
	rr := newTestRR(obj)
	g.Expect(m.initialize(context.Background(), rr)).To(Succeed())

	err := m.customizeManifests(context.Background(), rr)
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("invalid OIDC issuer URL"))
}

func TestParseAndValidateOIDCIssuerURL(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
		errMsg  string
	}{
		{"valid https", "https://issuer.example.com", false, ""},
		{"valid https with path", "https://issuer.example.com/path", false, ""},
		{"http not allowed", "http://issuer.example.com", true, "https scheme"},
		{"no host", "https://", true, "host"},
		{"not a url", "not-a-url", true, ""},
		{"empty", "", true, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			_, err := parseAndValidateOIDCIssuerURL(tt.input)
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
				if tt.errMsg != "" {
					g.Expect(err.Error()).To(ContainSubstring(tt.errMsg))
				}
			} else {
				g.Expect(err).NotTo(HaveOccurred())
			}
		})
	}
}
