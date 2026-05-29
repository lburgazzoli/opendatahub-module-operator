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

	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"

	componentApi "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-mlflow-operator/api/components/v1alpha1"
	moduleconfig "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-mlflow-operator/pkg/config"
	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-mlflow-operator/pkg/version"
	odhtypes "github.com/opendatahub-io/opendatahub-operator/v2/pkg/controller/types"
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

func newTestModule(t *testing.T) *Module {
	t.Helper()

	cfg := &moduleconfig.Config{
		PlatformType:          "OpenDataHub",
		PlatformVersion:       "1.0.0",
		ManifestsPath:         "/manifests",
		ApplicationsNamespace: "test-ns",
	}

	m, err := NewModule(cfg)
	NewWithT(t).Expect(err).NotTo(HaveOccurred())

	return m
}

func newTestRR(obj *componentApi.MLflowOperator) *odhtypes.ReconciliationRequest {
	return &odhtypes.ReconciliationRequest{
		Instance:          obj,
		ManifestsBasePath: "/manifests",
		Release: (&moduleconfig.Config{
			PlatformType:    "OpenDataHub",
			PlatformVersion: "1.0.0",
		}).Release(),
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

	cfg := &moduleconfig.Config{
		PlatformType:    "OpenDataHub",
		PlatformVersion: "1.0.0",
		ManifestsPath:   "/manifests",
	}

	m, err := NewModule(cfg)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(m.version.String()).To(Equal(version.Version))
	g.Expect(m.cfg).To(Equal(cfg))
	g.Expect(m.manifestInfo.ContextDir).To(Equal(componentName))
	g.Expect(m.manifestInfo.SourcePath).To(Equal(overlayODH))
}

func TestNewModuleInvalidVersion(t *testing.T) {
	g := NewWithT(t)

	orig := version.Version
	version.Version = "not-a-version"

	t.Cleanup(func() { version.Version = orig })

	_, err := NewModule(&moduleconfig.Config{})
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("invalid semver"))
}

func TestInitialize(t *testing.T) {
	g := NewWithT(t)

	m := newTestModule(t)
	obj := newTestMLflowOperator()
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
	obj := newTestMLflowOperator()
	rr := newTestRR(obj)

	g.Expect(m.upgradeIfNeeded(context.Background(), rr)).To(Succeed())
}

func TestUpgradeIfNeededSameVersion(t *testing.T) {
	g := NewWithT(t)

	m := newTestModule(t)
	obj := newTestMLflowOperator()

	v, err := componentApi.NewSemVer(version.Version)
	g.Expect(err).NotTo(HaveOccurred())

	obj.Status.Module.Version = v
	rr := newTestRR(obj)

	g.Expect(m.upgradeIfNeeded(context.Background(), rr)).To(Succeed())
}

func TestReportStatus(t *testing.T) {
	g := NewWithT(t)

	m := newTestModule(t)
	obj := newTestMLflowOperator()
	rr := newTestRR(obj)

	g.Expect(m.initialize(context.Background(), rr)).To(Succeed())
	g.Expect(m.reportStatus(context.Background(), rr)).To(Succeed())

	g.Expect(obj.Status.Module.Version.String()).To(Equal(version.Version))
	g.Expect(obj.Status.Module.Platform.Name).To(Equal("OpenDataHub"))
	g.Expect(obj.Status.Module.Platform.Version.String()).To(Equal("1.0.0"))
	g.Expect(obj.Status.Module.Sources).To(HaveLen(1))
	g.Expect(obj.Status.Module.Sources[0].Renderer).To(Equal(componentApi.SourceRendererKustomize))
}

func TestSetKustomizedParamsIgnoresMissingGatewayConfigAPI(t *testing.T) {
	g := NewWithT(t)

	m := newTestModule(t)
	rr := newTestRR(newTestMLflowOperator())
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
	g.Expect(m.setKustomizedParams(context.Background(), rr)).To(Succeed())
}
