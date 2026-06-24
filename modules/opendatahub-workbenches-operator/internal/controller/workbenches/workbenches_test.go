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

	"github.com/blang/semver/v4"
	. "github.com/onsi/gomega"
	fwapi "github.com/opendatahub-io/odh-platform-utilities/framework/api"
	fwtypes "github.com/opendatahub-io/odh-platform-utilities/framework/controller/types"
	ofVersion "github.com/operator-framework/api/pkg/lib/version"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	componentApi "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-workbenches-operator/api/components/v1alpha1"
	moduleconfig "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-workbenches-operator/pkg/config"
)

func newTestModule(t *testing.T) *Module {
	t.Helper()
	cfg := &moduleconfig.Config{
		PlatformName:          "OpenDataHub",
		PlatformVersion:       "1.0.0",
		ManifestsPath:         "/manifests",
		ApplicationsNamespace: "test-ns",
	}
	m, err := NewModule(cfg)
	NewWithT(t).Expect(err).NotTo(HaveOccurred())
	return m
}

func seedTestAPIReader(t *testing.T, m *Module, objs ...client.Object) {
	t.Helper()
	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(corev1.AddToScheme(scheme))
	utilruntime.Must(apiextensionsv1.AddToScheme(scheme))
	utilruntime.Must(componentApi.AddToScheme(scheme))
	m.apiReader = fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(objs...).
		Build()
}

func newTestRR(obj *componentApi.Workbenches) *fwtypes.ReconciliationRequest {
	rel := (&moduleconfig.Config{
		PlatformName:    "OpenDataHub",
		PlatformVersion: "1.0.0",
	}).Release()

	return &fwtypes.ReconciliationRequest{
		Instance:          obj,
		ManifestsBasePath: "/manifests",
		Release: fwapi.Release{
			Name:    fwapi.Platform(rel.Name),
			Version: rel.Version.Version,
		},
	}
}

func newTestWorkbenches() *componentApi.Workbenches {
	return &componentApi.Workbenches{
		ObjectMeta: metav1.ObjectMeta{
			Name: componentApi.WorkbenchesInstanceName,
		},
	}
}

func TestInitialize(t *testing.T) {
	g := NewWithT(t)

	m := newTestModule(t)
	obj := newTestWorkbenches()
	rr := newTestRR(obj)

	g.Expect(m.initialize(context.Background(), rr)).To(Succeed())
	// initialize sets 3 manifests: odh-notebook-controller, kf-notebook-controller, notebooks
	g.Expect(rr.Manifests).To(HaveLen(3))
	g.Expect(rr.Manifests[0].ContextDir).To(Equal(notebookControllerContextDir))
	g.Expect(rr.Manifests[1].ContextDir).To(Equal(kfNotebookControllerContextDir))
	g.Expect(rr.Manifests[2].ContextDir).To(Equal(notebookContextDir))
}

func TestUpgradeIfNeededNoVersion(t *testing.T) {
	g := NewWithT(t)

	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(corev1.AddToScheme(scheme))
	utilruntime.Must(componentApi.AddToScheme(scheme))
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()

	m := newTestModule(t)
	obj := newTestWorkbenches()
	rr := newTestRR(obj)
	rr.Client = fakeClient
	seedTestAPIReader(t, m, obj.DeepCopy())

	g.Expect(m.upgradeIfNeeded(context.Background(), rr)).To(Succeed())

	events := &corev1.EventList{}
	g.Expect(fakeClient.List(context.Background(), events)).To(Succeed())
	g.Expect(events.Items).To(HaveLen(1))
	g.Expect(events.Items[0].Reason).To(Equal(upgradeEventReasonStarted))
}

func TestUpgradeIfNeededSameVersion(t *testing.T) {
	g := NewWithT(t)

	m := newTestModule(t)
	obj := newTestWorkbenches()

	obj.Status.Release.Version = ofVersion.OperatorVersion{Version: semver.MustParse("1.0.0")}
	rr := newTestRR(obj)
	seedTestAPIReader(t, m, obj.DeepCopy())

	g.Expect(m.upgradeIfNeeded(context.Background(), rr)).To(Succeed())
}

func TestReportStatus(t *testing.T) {
	g := NewWithT(t)

	m := newTestModule(t)
	obj := newTestWorkbenches()
	rr := newTestRR(obj)

	g.Expect(m.initialize(context.Background(), rr)).To(Succeed())
	g.Expect(m.reportStatus(context.Background(), rr)).To(Succeed())

	g.Expect(obj.Status.Release.Version.String()).To(Equal("1.0.0"))
	g.Expect(string(obj.Status.Release.Name)).To(Equal("OpenDataHub"))
}
