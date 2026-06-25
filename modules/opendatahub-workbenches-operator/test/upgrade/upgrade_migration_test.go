//go:build upgrade

package upgrade

import (
	"testing"

	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gstruct"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	k8serr "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/lburgazzoli/gomega-matchers/pkg/matchers/jq"
	k8sm "github.com/lburgazzoli/gomega-matchers/pkg/matchers/k8s"

	componentsv1alpha1 "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-workbenches-operator/api/components/v1alpha1"
	workbenches "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-workbenches-operator/internal/controller/workbenches"
	moduleconfig "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-workbenches-operator/pkg/config"
	gvk "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-workbenches-operator/pkg/resources/gvk"
	common "github.com/opendatahub-io/odh-platform-utilities/api/common"
	infrav1 "github.com/opendatahub-io/opendatahub-operator/v2/api/infrastructure/v1"
)

const (
	notebookSizeAnnotation = "notebooks.opendatahub.io/last-size-selection"
	hwpNameAnnotation      = "opendatahub.io/hardware-profile-name"
	hwpNamespaceAnnotation = "opendatahub.io/hardware-profile-namespace"
)

type migrationTests struct {
	s                  suite
	module             *componentsv1alpha1.Workbenches
	moduleCRD          *apiextensionsv1.CustomResourceDefinition
	notebook           *unstructured.Unstructured
	odhDashboardConfig *unstructured.Unstructured
	hardwareProfile    *infrav1.HardwareProfile
}

func (mt *migrationTests) Execute(t *testing.T) {
	mt.notebook = newNotebook(mt.s.operatorNamespace(), "upgrade-notebook")
	mt.odhDashboardConfig = newOdhDashboardConfig(mt.s.operatorNamespace())
	mt.hardwareProfile = &infrav1.HardwareProfile{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "containersize-small-notebooks",
			Namespace: mt.s.operatorNamespace(),
		},
	}

	t.Run("should migrate container size annotations to hardware profiles", mt.testContainerSizeMigration)
}

func (mt *migrationTests) testContainerSizeMigration(t *testing.T) {
	ctx := t.Context()
	g := NewWithT(t)

	deleteIfExists(t, mt.s, mt.notebook)
	deleteIfExists(t, mt.s, mt.odhDashboardConfig)
	deleteIfExists(t, mt.s, mt.hardwareProfile)
	deleteIfExists(t, mt.s, mt.module)
	g.Eventually(ctx, k8sm.NotFound(mt.s.directClient, mt.module)).Should(BeTrue())
	mt.module.SetResourceVersion("")
	mt.module.SetUID("")

	t.Cleanup(func() {
		deleteIfExists(t, mt.s, mt.notebook)
		deleteIfExists(t, mt.s, mt.odhDashboardConfig)
		deleteIfExists(t, mt.s, mt.hardwareProfile)
		deleteIfExists(t, mt.s, mt.module)
	})

	g.Expect(mt.s.directClient.Create(ctx, mt.module)).To(Succeed())

	seededModule := &componentsv1alpha1.Workbenches{
		ObjectMeta: metav1.ObjectMeta{Name: componentsv1alpha1.WorkbenchesInstanceName},
	}
	g.Eventually(ctx, k8sm.Lookup(mt.s.directClient, seededModule)).Should(Succeed())
	workbenches.UpsertRelease(seededModule.GetReleaseStatus(), common.ComponentRelease{
		Name:    moduleconfig.ReleasePlatform,
		Version: appliedUpgradeVersion,
	})
	g.Expect(mt.s.directClient.Status().Update(ctx, seededModule)).To(Succeed())
	g.Expect(mt.s.directClient.Create(ctx, mt.odhDashboardConfig)).To(Succeed())
	g.Expect(mt.s.directClient.Create(ctx, mt.notebook)).To(Succeed())

	g.Eventually(ctx, k8sm.Lookup(mt.s.directClient, seededModule)).Should(Succeed())
	prev, _ := workbenches.GetRelease(seededModule.GetReleaseStatus(), moduleconfig.ReleasePlatform)
	g.Expect(prev.Version).To(Equal(appliedUpgradeVersion))
	g.Eventually(ctx, k8sm.Get(mt.s.directClient, mt.odhDashboardConfig)).Should(And(
		jq.Match(`.metadata.name == "odh-dashboard-config"`),
		jq.Matchf(`.metadata.namespace == "%s"`, mt.s.operatorNamespace()),
	))
	g.Eventually(ctx, k8sm.Get(mt.s.directClient, mt.notebook)).Should(And(
		jq.Match(`.metadata.name == "upgrade-notebook"`),
		jq.Matchf(`.metadata.namespace == "%s"`, mt.s.operatorNamespace()),
	))

	mt.s = startManager(t, mt.s)

	g.Eventually(ctx, k8sm.Lookup(mt.s.directClient, mt.moduleCRD)).Should(Succeed())

	g.Eventually(ctx, k8sm.Get(mt.s.directClient, mt.hardwareProfile)).Should(And(
		jq.Match(`.metadata.name == "containersize-small-notebooks"`),
		jq.Matchf(`.metadata.namespace == "%s"`, mt.s.operatorNamespace()),
	))
	g.Eventually(ctx, k8sm.Get(mt.s.directClient, mt.notebook)).Should(And(
		jq.Matchf(`.metadata.annotations."%s" == "Small"`, notebookSizeAnnotation),
		jq.Matchf(`.metadata.annotations."%s" == "containersize-small-notebooks"`, hwpNameAnnotation),
		jq.Matchf(`.metadata.annotations."%s" == "%s"`, hwpNamespaceAnnotation, mt.s.operatorNamespace()),
	))

	g.Eventually(ctx, k8sm.Get(mt.s.directClient, mt.module)).Should(
		jq.Matchf(`.status.release.version == "%s"`, desiredUpgradeVersion),
	)
	g.Eventually(ctx, k8sm.Get(mt.s.directClient, mt.notebook)).Should(And(
		jq.Matchf(`.metadata.annotations."%s" == "containersize-small-notebooks"`, hwpNameAnnotation),
		jq.Matchf(`.metadata.annotations."%s" == "%s"`, hwpNamespaceAnnotation, mt.s.operatorNamespace()),
	))
	g.Eventually(ctx, k8sm.Events(
		mt.s.directClient,
		k8sm.InNamespace(mt.s.operatorNamespace()),
		k8sm.ForObject(corev1.ObjectReference{
			Kind: componentsv1alpha1.WorkbenchesKind,
			Name: mt.module.GetName(),
		}),
	)).Should(ContainElement(gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
		"Reason": Equal("UpgradeStarted"),
	})))
}

func newNotebook(namespace string, name string) *unstructured.Unstructured {
	notebook := &unstructured.Unstructured{}
	notebook.SetGroupVersionKind(gvk.Notebook)
	notebook.SetName(name)
	notebook.SetNamespace(namespace)
	notebook.SetAnnotations(map[string]string{
		notebookSizeAnnotation: "Small",
	})
	notebook.Object["spec"] = map[string]any{
		"template": map[string]any{
			"spec": map[string]any{
				"containers": []any{
					map[string]any{
						"name":            name,
						"image":           "quay.io/thoth-station/s2i-minimal-notebook:v0.2.2",
						"imagePullPolicy": "Always",
						"workingDir":      "/opt/app-root/src",
						"env": []any{
							map[string]any{"name": "JUPYTER_NOTEBOOK_PORT", "value": "8888"},
							map[string]any{"name": "NOTEBOOK_ARGS", "value": "--NotebookApp.token='' --NotebookApp.password=''"},
						},
					},
				},
			},
		},
	}

	return notebook
}

func newOdhDashboardConfig(namespace string) *unstructured.Unstructured {
	cfg := &unstructured.Unstructured{}
	cfg.SetGroupVersionKind(gvk.OdhDashboardConfig)
	cfg.SetName("odh-dashboard-config")
	cfg.SetNamespace(namespace)
	cfg.Object["spec"] = map[string]any{
		"notebookSizes": []any{
			map[string]any{
				"name": "Small",
				"resources": map[string]any{
					"requests": map[string]any{
						"cpu":    "1",
						"memory": "8Gi",
					},
					"limits": map[string]any{
						"cpu":    "2",
						"memory": "8Gi",
					},
				},
			},
		},
		"notebookController": map[string]any{
			"enabled": true,
		},
	}

	return cfg
}

func deleteIfExists(t *testing.T, s suite, obj client.Object) {
	t.Helper()
	ctx := t.Context()

	g := NewWithT(t)
	err := s.directClient.Delete(ctx, obj)
	g.Expect(err).To(SatisfyAny(
		BeNil(),
		MatchError(k8serr.IsNotFound, "IsNotFound"),
		MatchError(meta.IsNoMatchError, "IsNoMatchError"),
	))
}
