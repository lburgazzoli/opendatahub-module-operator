//go:build upgrade

package upgrade

import (
	"testing"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	k8serr "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/lburgazzoli/gomega-matchers/pkg/matchers/jq"

	componentsv1alpha1 "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-workbenches-operator/api/components/v1alpha1"
	gvk "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-workbenches-operator/pkg/resources/gvk"
	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-workbenches-operator/test/support"
	infrav1 "github.com/opendatahub-io/opendatahub-operator/v2/api/infrastructure/v1"
)

const (
	notebookSizeAnnotation   = "notebooks.opendatahub.io/last-size-selection"
	hwpNameAnnotation        = "opendatahub.io/hardware-profile-name"
	hwpNamespaceAnnotation   = "opendatahub.io/hardware-profile-namespace"
)

type migrationTests struct {
	*workbenchesUpgradeTest
	notebook           *unstructured.Unstructured
	odhDashboardConfig *unstructured.Unstructured
	hardwareProfile    *infrav1.HardwareProfile
}

func newMigrationTests(suite *workbenchesUpgradeTest) *migrationTests {
	return &migrationTests{
		workbenchesUpgradeTest: suite,
		notebook:               newNotebook(suite.operatorNamespace, "upgrade-notebook"),
		odhDashboardConfig:     newOdhDashboardConfig(suite.operatorNamespace),
		hardwareProfile: &infrav1.HardwareProfile{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "containersize-small-notebooks",
				Namespace: suite.operatorNamespace,
			},
		},
	}
}

func (mt *migrationTests) Execute(t *testing.T) {
	t.Run("should migrate container size annotations to hardware profiles", mt.testContainerSizeMigration)
}

func (mt *migrationTests) testContainerSizeMigration(t *testing.T) {
	g := NewWithT(t)

	deleteIfExists(t, mt.notebook)
	deleteIfExists(t, mt.odhDashboardConfig)
	deleteIfExists(t, mt.hardwareProfile)
	deleteIfExists(t, mt.module)
	waitForSingletonDeleted(t, mt.module)

	t.Cleanup(func() {
		deleteIfExists(t, mt.notebook)
		deleteIfExists(t, mt.odhDashboardConfig)
		deleteIfExists(t, mt.hardwareProfile)
		deleteIfExists(t, mt.module)
	})

	g.Expect(directClient.Create(ctx, mt.module)).To(Succeed())

	seededModule := &componentsv1alpha1.Workbenches{
		ObjectMeta: metav1.ObjectMeta{Name: componentsv1alpha1.WorkbenchesInstanceName},
	}
	g.Expect(directClient.Get(ctx, client.ObjectKeyFromObject(seededModule), seededModule)).To(Succeed())
	seededModule.Status.Module.Version = componentsv1alpha1.SemVer(appliedUpgradeVersion)
	seededModule.Status.Module.Platform.Version = componentsv1alpha1.SemVer(appliedUpgradeVersion)
	g.Expect(directClient.Status().Update(ctx, seededModule)).To(Succeed())
	g.Expect(directClient.Create(ctx, mt.odhDashboardConfig)).To(Succeed())
	g.Expect(directClient.Create(ctx, mt.notebook)).To(Succeed())

	g.Expect(directClient.Get(ctx, client.ObjectKeyFromObject(seededModule), seededModule)).To(Succeed())
	g.Expect(seededModule.Status.Module.Version).To(Equal(componentsv1alpha1.SemVer(appliedUpgradeVersion)))
	g.Expect(seededModule.Status.Module.Platform.Version).To(Equal(componentsv1alpha1.SemVer(appliedUpgradeVersion)))
	g.Eventually(directK.Get(mt.odhDashboardConfig)).WithContext(ctx).WithTimeout(timeout).WithPolling(interval).Should(And(
		jq.Match(`.metadata.name == "odh-dashboard-config"`),
		jq.Match(`.metadata.namespace == "%s"`, mt.operatorNamespace),
	))
	g.Eventually(directK.Get(mt.notebook)).WithContext(ctx).WithTimeout(timeout).WithPolling(interval).Should(And(
		jq.Match(`.metadata.name == "upgrade-notebook"`),
		jq.Match(`.metadata.namespace == "%s"`, mt.operatorNamespace),
	))

	startManager(t)

	g.Eventually(k.Get(mt.moduleCRD)).WithContext(ctx).WithTimeout(timeout).WithPolling(interval).Should(
		jq.Match(`.metadata.name == "%s"`, moduleCRDName),
	)

	g.Eventually(directK.Get(mt.hardwareProfile)).WithContext(ctx).WithTimeout(timeout).WithPolling(interval).Should(And(
		jq.Match(`.metadata.name == "containersize-small-notebooks"`),
		jq.Match(`.metadata.namespace == "%s"`, mt.operatorNamespace),
	))
	g.Eventually(directK.Get(mt.notebook)).WithContext(ctx).WithTimeout(timeout).WithPolling(interval).Should(And(
		jq.Match(`.metadata.annotations."%s" == "Small"`, notebookSizeAnnotation),
		jq.Match(`.metadata.annotations."%s" == "containersize-small-notebooks"`, hwpNameAnnotation),
		jq.Match(`.metadata.annotations."%s" == "%s"`, hwpNamespaceAnnotation, mt.operatorNamespace),
	))

	g.Eventually(directK.Get(mt.module)).WithContext(ctx).WithTimeout(timeout).WithPolling(interval).Should(
		jq.Match(`.status.module.version == "%s"`, desiredUpgradeVersion),
	)
	g.Eventually(directK.Get(mt.module)).WithContext(ctx).WithTimeout(timeout).WithPolling(interval).Should(
		jq.Match(`.status.module.platform.version == "%s"`, desiredUpgradeVersion),
	)
	g.Eventually(directK.Get(mt.notebook)).WithContext(ctx).WithTimeout(timeout).WithPolling(interval).Should(And(
		jq.Match(`.metadata.annotations."%s" == "containersize-small-notebooks"`, hwpNameAnnotation),
		jq.Match(`.metadata.annotations."%s" == "%s"`, hwpNamespaceAnnotation, mt.operatorNamespace),
	))
	g.Eventually(moduleHasEventReason(mt.module.GetName(), "UpgradeStarted")).
		WithContext(ctx).
		WithTimeout(timeout).
		WithPolling(interval).
		Should(BeTrue())
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

func moduleHasEventReason(moduleName string, reason string) func() bool {
	return func() bool {
		events := &corev1.EventList{}
		if err := directClient.List(ctx, events, client.InNamespace(support.OperatorNamespace())); err != nil {
			return false
		}
		for i := range events.Items {
			event := &events.Items[i]
			if event.Reason != reason {
				continue
			}
			if event.InvolvedObject.Kind != componentsv1alpha1.WorkbenchesKind {
				continue
			}
			if event.InvolvedObject.Name != moduleName {
				continue
			}

			return true
		}

		return false
	}
}

func deleteIfExists(t *testing.T, obj client.Object) {
	t.Helper()

	g := NewWithT(t)
	err := directClient.Delete(ctx, obj)
	g.Expect(err).To(SatisfyAny(
		BeNil(),
		MatchError(k8serr.IsNotFound, "IsNotFound"),
		MatchError(meta.IsNoMatchError, "IsNoMatchError"),
	))
}
