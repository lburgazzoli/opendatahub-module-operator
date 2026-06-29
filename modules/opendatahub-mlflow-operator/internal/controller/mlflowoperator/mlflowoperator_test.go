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

package mlflowoperator

import (
	"context"
	"testing"
	"testing/fstest"

	. "github.com/onsi/gomega"
	common "github.com/opendatahub-io/odh-platform-utilities/api/common"
	odhtypes "github.com/opendatahub-io/odh-platform-utilities/framework/controller/types"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"

	componentApi "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-mlflow-operator/api/components/v1alpha1"
	moduleconfig "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-mlflow-operator/pkg/config"
)

type staticErrReader struct {
	err error
}

type staticErrSubResourceClient struct {
	err error
}

type staticErrClient struct {
	staticErrReader
}

func (r staticErrReader) Get(
	_ context.Context,
	_ client.ObjectKey,
	_ client.Object,
	_ ...client.GetOption,
) error {
	return r.err
}

func (r staticErrReader) List(
	_ context.Context,
	_ client.ObjectList,
	_ ...client.ListOption,
) error {
	return r.err
}

func (c staticErrSubResourceClient) Get(
	_ context.Context,
	_ client.Object,
	_ client.Object,
	_ ...client.SubResourceGetOption,
) error {
	return c.err
}

func (c staticErrSubResourceClient) Create(
	_ context.Context,
	_ client.Object,
	_ client.Object,
	_ ...client.SubResourceCreateOption,
) error {
	return c.err
}

func (c staticErrSubResourceClient) Update(
	_ context.Context,
	_ client.Object,
	_ ...client.SubResourceUpdateOption,
) error {
	return c.err
}

func (c staticErrSubResourceClient) Patch(
	_ context.Context,
	_ client.Object,
	_ client.Patch,
	_ ...client.SubResourcePatchOption,
) error {
	return c.err
}

func (c staticErrSubResourceClient) Apply(
	_ context.Context,
	_ runtime.ApplyConfiguration,
	_ ...client.SubResourceApplyOption,
) error {
	return c.err
}

func (c staticErrClient) Apply(
	_ context.Context,
	_ runtime.ApplyConfiguration,
	_ ...client.ApplyOption,
) error {
	return c.err
}

func (c staticErrClient) Create(
	_ context.Context,
	_ client.Object,
	_ ...client.CreateOption,
) error {
	return c.err
}

func (c staticErrClient) Delete(
	_ context.Context,
	_ client.Object,
	_ ...client.DeleteOption,
) error {
	return c.err
}

func (c staticErrClient) Update(
	_ context.Context,
	_ client.Object,
	_ ...client.UpdateOption,
) error {
	return c.err
}

func (c staticErrClient) Patch(
	_ context.Context,
	_ client.Object,
	_ client.Patch,
	_ ...client.PatchOption,
) error {
	return c.err
}

func (c staticErrClient) DeleteAllOf(
	_ context.Context,
	_ client.Object,
	_ ...client.DeleteAllOfOption,
) error {
	return c.err
}

func (c staticErrClient) Status() client.SubResourceWriter {
	return staticErrSubResourceClient{err: c.err}
}

func (c staticErrClient) SubResource(_ string) client.SubResourceClient {
	return staticErrSubResourceClient{err: c.err}
}

func (c staticErrClient) Scheme() *runtime.Scheme {
	return nil
}

func (c staticErrClient) RESTMapper() meta.RESTMapper {
	return nil
}

func (c staticErrClient) GroupVersionKindFor(_ runtime.Object) (schema.GroupVersionKind, error) {
	return schema.GroupVersionKind{}, c.err
}

func (c staticErrClient) IsObjectNamespaced(_ runtime.Object) (bool, error) {
	return false, c.err
}

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

func newTestRR(t *testing.T, obj *componentApi.MLflowOperator) *odhtypes.ReconciliationRequest {
	t.Helper()

	cfg := testConfig(t)
	return &odhtypes.ReconciliationRequest{
		Instance: obj,
		Release:  cfg.PlatformRelease(),
	}
}

func newTestMLflowOperator() *componentApi.MLflowOperator {
	return &componentApi.MLflowOperator{
		ObjectMeta: metav1.ObjectMeta{
			Name: componentApi.MLflowOperatorInstanceName,
		},
	}
}

func TestNewModule(t *testing.T) {
	g := NewWithT(t)

	cfg := testConfig(t)

	m, err := NewModule(cfg)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(m.cfg).To(Equal(cfg))
	g.Expect(m.manifestInfo.ContextDir).To(Equal(componentName))
	g.Expect(m.manifestInfo.Path).To(Equal(manifestsRoot))
	g.Expect(m.manifestInfo.SourcePath).To(Equal(overlayODH))
}

func TestInitialize(t *testing.T) {
	g := NewWithT(t)

	m := newTestModule(t)
	obj := newTestMLflowOperator()
	rr := newTestRR(t, obj)

	g.Expect(m.initialize(context.Background(), rr)).To(Succeed())
	g.Expect(rr.Manifests).To(HaveLen(1))
	g.Expect(rr.Templates).To(HaveLen(1))
	g.Expect(rr.Manifests[0].Path).To(Equal(manifestsRoot))
	g.Expect(rr.Manifests[0].ContextDir).To(Equal(componentName))
	g.Expect(rr.Manifests[0].SourcePath).To(Equal(overlayODH))
	g.Expect(rr.Templates[0].Path).To(Equal(openShiftConfigGrantsTemplatePath))
}

func TestInitLoadsReleases(t *testing.T) {
	g := NewWithT(t)

	m := newTestModule(t)

	g.Expect(m.Init()).To(Succeed())
	g.Expect(m.releases).To(ContainElement(
		common.ComponentRelease{
			Name:    "MLflow",
			Version: "v3.12.0",
			RepoURL: "https://github.com/mlflow/mlflow",
		},
	))
	g.Expect(m.releases).To(ContainElement(
		common.ComponentRelease{Name: moduleconfig.ReleasePlatform, Version: "1.0.0"},
	))
}

func TestUpgradeIfNeededNoVersion(t *testing.T) {
	g := NewWithT(t)

	m := newTestModule(t)
	obj := newTestMLflowOperator()
	rr := newTestRR(t, obj)

	g.Expect(m.upgradeIfNeeded(context.Background(), rr)).To(Succeed())
}

func TestUpgradeIfNeededSameVersion(t *testing.T) {
	g := NewWithT(t)

	m := newTestModule(t)
	obj := newTestMLflowOperator()

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

	obj := newTestMLflowOperator()
	obj.Status.Releases = []common.ComponentRelease{
		{Name: "stale", Version: "0.0.1"},
	}
	rr := newTestRR(t, obj)

	g.Expect(m.initialize(context.Background(), rr)).To(Succeed())
	g.Expect(m.reportStatus(context.Background(), rr)).To(Succeed())

	g.Expect(obj.Status.Releases).NotTo(ContainElement(
		common.ComponentRelease{Name: "stale", Version: "0.0.1"},
	))
	g.Expect(obj.Status.Releases).To(ContainElement(
		common.ComponentRelease{
			Name:    "MLflow",
			Version: "v3.12.0",
			RepoURL: "https://github.com/mlflow/mlflow",
		},
	))
	g.Expect(obj.Status.Releases).To(Equal(m.releases))
}

func TestCustomizeManifestsIgnoresMissingGatewayConfigAPI(t *testing.T) {
	g := NewWithT(t)

	m := newTestModule(t)
	rr := newTestRR(t, newTestMLflowOperator())
	rr.Client = staticErrClient{
		staticErrReader: staticErrReader{
			err: &meta.NoResourceMatchError{
				PartialResource: schema.GroupVersionResource{
					Group:    "services.platform.opendatahub.io",
					Version:  "v1alpha1",
					Resource: "gatewayconfigs",
				},
			},
		},
	}
	g.Expect(m.customizeManifests(context.Background(), rr)).To(Succeed())
}
