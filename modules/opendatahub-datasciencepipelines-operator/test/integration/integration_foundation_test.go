package integration

import (
	"context"
	"fmt"
	"testing"

	. "github.com/onsi/gomega"

	"github.com/lburgazzoli/gomega-matchers/pkg/matchers/jq"
	k8sm "github.com/lburgazzoli/gomega-matchers/pkg/matchers/k8s"
	"github.com/lburgazzoli/gomega-matchers/pkg/matchers/k8s/condition"

	componentsv1alpha1 "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-datasciencepipelines-operator/api/components/v1alpha1"
	modulemeta "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-datasciencepipelines-operator/pkg/module"
	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-datasciencepipelines-operator/pkg/releases"
	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-datasciencepipelines-operator/test/support"
	common "github.com/opendatahub-io/odh-platform-utilities/api/common"
	"github.com/opendatahub-io/odh-platform-utilities/pkg/metadata/annotations"
	"github.com/opendatahub-io/odh-platform-utilities/pkg/metadata/labels"
	promv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type foundationTests struct {
	Client client.Client
}

func (ft *foundationTests) Execute(t *testing.T) {
	t.Run("should have module CRD installed", ft.testModuleCRDInstalled)
	t.Run("should fail when workflows CRD is not ODH-owned", ft.testForeignOwnedArgoWorkflowCRD)
	t.Run("should become ready when workflows CRD is ODH-owned", ft.testBecomesReady)
	t.Run("should report release version and platform", ft.testReleaseStatus)
	t.Run("should set platform labels and annotations", ft.testPlatformLabels)
	t.Run("should set owner references", ft.testOwnerReferences)
	t.Run("should fail when workflows CRD is missing and Argo is removed", ft.testMissingArgoWorkflowCRD)
}

func (ft *foundationTests) ensureReadyModule(t *testing.T) *componentsv1alpha1.DataSciencePipelines {
	t.Helper()

	ensureArgoWorkflowCRDOwnedByODH(t, ft.Client)

	g := NewWithT(t)
	module := &componentsv1alpha1.DataSciencePipelines{
		ObjectMeta: metav1.ObjectMeta{
			Name: componentsv1alpha1.DataSciencePipelinesInstanceName,
		},
	}
	workloadDeploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      support.ManagedDeploymentName,
			Namespace: support.IntegrationTestNamespace(),
		},
	}
	workloadConfigMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      workloadConfigMapName,
			Namespace: support.IntegrationTestNamespace(),
		},
	}
	workloadServiceMon := &promv1.ServiceMonitor{
		ObjectMeta: metav1.ObjectMeta{
			Name:      workloadServiceMonName,
			Namespace: support.IntegrationTestNamespace(),
		},
	}

	_ = ft.Client.Delete(t.Context(), module)
	g.Eventually(t.Context(), k8sm.NotFound(ft.Client, module)).Should(BeTrue())

	t.Cleanup(func() {
		_ = ft.Client.Delete(context.Background(), module)
	})

	g.Expect(ft.Client.Create(t.Context(), module)).To(Succeed())

	g.Eventually(t.Context(), k8sm.Get(ft.Client, module)).Should(
		WithTransform(k8sm.ConditionsOf[metav1.Condition](), SatisfyAll(
			ContainElement(condition.Is(string(common.ConditionTypeReady), metav1.ConditionTrue)),
			ContainElement(condition.Is(modulemeta.ConditionArgoWorkflowAvailable, metav1.ConditionTrue)),
			ContainElement(condition.Is(string(common.ConditionTypeProvisioningSucceeded), metav1.ConditionTrue)),
		)),
	)

	g.Eventually(t.Context(), k8sm.Get(ft.Client, workloadDeploy)).Should(
		jq.Match(`.status.readyReplicas >= 1`),
	)
	g.Eventually(t.Context(), k8sm.Get(ft.Client, workloadConfigMap)).Should(
		jq.Matchf(`.metadata.name == "%s"`, workloadConfigMapName),
	)
	g.Eventually(t.Context(), k8sm.Get(ft.Client, workloadServiceMon)).Should(
		jq.Matchf(`.metadata.name == "%s"`, workloadServiceMonName),
	)

	return module
}

func (ft *foundationTests) testModuleCRDInstalled(t *testing.T) {
	g := NewWithT(t)

	moduleCRD := &apiextensionsv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: componentsv1alpha1.DataSciencePipelinesCRDName},
	}
	g.Eventually(t.Context(), k8sm.Lookup(ft.Client, moduleCRD)).Should(Succeed())
}

func (ft *foundationTests) testMissingArgoWorkflowCRD(t *testing.T) {
	ensureArgoWorkflowCRDMissing(t, ft.Client)

	obj := &componentsv1alpha1.DataSciencePipelines{
		ObjectMeta: metav1.ObjectMeta{
			Name: componentsv1alpha1.DataSciencePipelinesInstanceName,
		},
	}
	obj.Spec.ArgoWorkflowsControllers = &componentsv1alpha1.ArgoWorkflowsControllersSpec{
		ManagementState: "Removed",
	}
	createModule(t, ft.Client, obj)

	g := NewWithT(t)
	g.Eventually(t.Context(), k8sm.Get(ft.Client, obj)).Should(
		WithTransform(k8sm.ConditionsOf[metav1.Condition](), SatisfyAll(
			ContainElement(SatisfyAll(
				condition.Is(modulemeta.ConditionArgoWorkflowAvailable, metav1.ConditionFalse),
				condition.HasReason(modulemeta.DataSciencePipelinesArgoWorkflowsCRDMissingReason),
			)),
			ContainElement(condition.Is(string(common.ConditionTypeReady), metav1.ConditionFalse)),
		)),
	)
}

func (ft *foundationTests) testForeignOwnedArgoWorkflowCRD(t *testing.T) {
	ensureArgoWorkflowCRDForeignOwned(t, ft.Client)
	obj := &componentsv1alpha1.DataSciencePipelines{
		ObjectMeta: metav1.ObjectMeta{
			Name: componentsv1alpha1.DataSciencePipelinesInstanceName,
		},
	}
	createModule(t, ft.Client, obj)

	g := NewWithT(t)
	g.Eventually(t.Context(), k8sm.Get(ft.Client, obj)).Should(
		WithTransform(k8sm.ConditionsOf[metav1.Condition](), SatisfyAll(
			ContainElement(SatisfyAll(
				condition.Is(modulemeta.ConditionArgoWorkflowAvailable, metav1.ConditionFalse),
				condition.HasReason(modulemeta.DataSciencePipelinesDoesntOwnArgoCRDReason),
			)),
		)),
	)
}

func (ft *foundationTests) testBecomesReady(t *testing.T) {
	ft.ensureReadyModule(t)
}

func (ft *foundationTests) testReleaseStatus(t *testing.T) {
	g := NewWithT(t)

	module := ft.ensureReadyModule(t)
	cfg, err := loadOperatorConfig()
	g.Expect(err).NotTo(HaveOccurred())

	expr := fmt.Sprintf(`.status.releases[] | select(.name == "%s") | .version == "%s"`,
		releases.Platform, cfg.Release().Version)
	g.Eventually(t.Context(), k8sm.Get(ft.Client, module)).Should(jq.Match(expr))
}

func (ft *foundationTests) testPlatformLabels(t *testing.T) {
	g := NewWithT(t)

	module := ft.ensureReadyModule(t)
	workloadDeploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      support.ManagedDeploymentName,
			Namespace: support.IntegrationTestNamespace(),
		},
	}
	cfg, err := loadOperatorConfig()
	g.Expect(err).NotTo(HaveOccurred())

	g.Eventually(t.Context(), k8sm.Get(ft.Client, workloadDeploy)).Should(And(
		k8sm.HasLabel(labels.PlatformPartOf, componentsv1alpha1.DataSciencePipelinesComponentName),
		k8sm.HasAnnotation(annotations.InstanceName, module.GetName()),
		k8sm.HasAnnotation(annotations.InstanceUID, string(module.GetUID())),
		k8sm.HasAnnotation(annotations.PlatformVersion, cfg.Release().Version),
	))
}

func (ft *foundationTests) testOwnerReferences(t *testing.T) {
	g := NewWithT(t)

	owner := ft.ensureReadyModule(t)
	workloadDeploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      support.ManagedDeploymentName,
			Namespace: support.IntegrationTestNamespace(),
		},
	}
	owner.TypeMeta = metav1.TypeMeta{
		APIVersion: componentsv1alpha1.GroupVersion.String(),
		Kind:       componentsv1alpha1.DataSciencePipelinesKind,
	}

	g.Eventually(t.Context(), k8sm.Get(ft.Client, workloadDeploy)).Should(
		k8sm.HasOwnerReference(owner),
	)
}
