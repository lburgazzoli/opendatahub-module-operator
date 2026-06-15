//go:build e2e

package e2e

import (
	"testing"
	"testing/fstest"

	. "github.com/onsi/gomega"

	"github.com/lburgazzoli/gomega-matchers/pkg/matchers/jq"
	k8sm "github.com/lburgazzoli/gomega-matchers/pkg/matchers/k8s"
	"github.com/lburgazzoli/gomega-matchers/pkg/matchers/k8s/condition"

	componentsv1alpha1 "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-datasciencepipelines-operator/api/components/v1alpha1"
	moduleconfig "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-datasciencepipelines-operator/pkg/config"
	modulemeta "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-datasciencepipelines-operator/pkg/module"
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
	t.Run("should have operator ConfigMap deployed", ft.testOperatorConfigMap)
	t.Run("should fail when workflows CRD is missing and Argo is removed", ft.testMissingArgoWorkflowCRD)
	t.Run("should fail when workflows CRD is not ODH-owned", ft.testForeignOwnedArgoWorkflowCRD)
	t.Run("should become ready when workflows CRD is ODH-owned", ft.testBecomesReady)
	t.Run("should report release version and platform", ft.testReleaseStatus)
	t.Run("should set platform labels and annotations", ft.testPlatformLabels)
	t.Run("should set owner references", ft.testOwnerReferences)
}

func moduleObject() *componentsv1alpha1.DataSciencePipelines {
	return &componentsv1alpha1.DataSciencePipelines{
		ObjectMeta: metav1.ObjectMeta{
			Name: componentsv1alpha1.DataSciencePipelinesInstanceName,
		},
	}
}

func workloadDeployment() *appsv1.Deployment {
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      workloadDeploymentName,
			Namespace: support.OperatorNamespace(),
		},
	}
}

func (ft *foundationTests) ensureReadyModule(t *testing.T) *componentsv1alpha1.DataSciencePipelines {
	t.Helper()

	withStoppedOperator(t, ft.Client, func() {
		ensureArgoWorkflowCRDOwnedByODH(t, ft.Client)
	})

	g := NewWithT(t)
	module := moduleObject()
	workloadDeploy := workloadDeployment()
	workloadServiceMon := &promv1.ServiceMonitor{
		ObjectMeta: metav1.ObjectMeta{
			Name:      workloadServiceMonName,
			Namespace: support.OperatorNamespace(),
		},
	}

	withStoppedOperator(t, ft.Client, func() {
		createModule(t, ft.Client, module)
	})

	g.Eventually(t.Context(), k8sm.Get(ft.Client, module)).Should(And(
		jq.Match(`.status.phase == "Ready"`),
		WithTransform(k8sm.ConditionsOf[metav1.Condition](), SatisfyAll(
			ContainElement(condition.Is(common.ConditionTypeReady, metav1.ConditionTrue)),
			ContainElement(condition.Is(modulemeta.ConditionArgoWorkflowAvailable, metav1.ConditionTrue)),
			ContainElement(condition.Is(common.ConditionTypeProvisioningSucceeded, metav1.ConditionTrue)),
		)),
	))

	g.Eventually(t.Context(), k8sm.Get(ft.Client, workloadDeploy)).Should(
		jq.Match(`.status.readyReplicas >= 1`),
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

func (ft *foundationTests) testOperatorConfigMap(t *testing.T) {
	g := NewWithT(t)

	operatorCfgMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      modulemeta.OperatorConfigName,
			Namespace: support.OperatorNamespace(),
		},
	}
	g.Eventually(t.Context(), k8sm.Get(ft.Client, operatorCfgMap)).Should(
		WithTransform(k8sm.Data(), SatisfyAll(
			HaveKeyWithValue(moduleconfig.KeyPlatformName, Not(BeEmpty())),
			HaveKeyWithValue(moduleconfig.KeyPlatformVersion, Not(BeEmpty())),
		)),
	)
}

func (ft *foundationTests) testMissingArgoWorkflowCRD(t *testing.T) {
	obj := moduleObject()
	withStoppedOperator(t, ft.Client, func() {
		ensureArgoWorkflowCRDMissing(t, ft.Client)

		obj.Spec.ArgoWorkflowsControllers = &componentsv1alpha1.ArgoWorkflowsControllersSpec{
			ManagementState: "Removed",
		}
		createModule(t, ft.Client, obj)
	})

	g := NewWithT(t)
	g.Eventually(t.Context(), k8sm.Get(ft.Client, obj)).Should(
		WithTransform(k8sm.ConditionsOf[metav1.Condition](), SatisfyAll(
			ContainElement(SatisfyAll(
				condition.Is(modulemeta.ConditionArgoWorkflowAvailable, metav1.ConditionFalse),
				condition.HasReason(modulemeta.DataSciencePipelinesArgoWorkflowsCRDMissingReason),
			)),
		)),
	)
}

func (ft *foundationTests) testForeignOwnedArgoWorkflowCRD(t *testing.T) {
	obj := moduleObject()
	withStoppedOperator(t, ft.Client, func() {
		ensureArgoWorkflowCRDForeignOwned(t, ft.Client)
		createModule(t, ft.Client, obj)
	})

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

	operatorCfg := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      modulemeta.OperatorConfigName,
			Namespace: support.OperatorNamespace(),
		},
	}
	module := ft.ensureReadyModule(t)

	g.Eventually(t.Context(), k8sm.Lookup(ft.Client, operatorCfg)).Should(Succeed())

	cfg, err := moduleconfig.LoadFromFS(fstest.MapFS{
		moduleconfig.KeyPlatformName: {
			Data: []byte(operatorCfg.Data[moduleconfig.KeyPlatformName]),
		},
		moduleconfig.KeyPlatformVersion: {
			Data: []byte(operatorCfg.Data[moduleconfig.KeyPlatformVersion]),
		},
	})
	g.Expect(err).NotTo(HaveOccurred())
	expectedRelease := cfg.Release()

	g.Eventually(t.Context(), k8sm.Get(ft.Client, module)).Should(And(
		jq.Matchf(`.status.release.version == "%s"`, expectedRelease.Version.String()),
		jq.Matchf(`.status.release.name == "%s"`, string(expectedRelease.Name)),
	))
}

func (ft *foundationTests) testPlatformLabels(t *testing.T) {
	g := NewWithT(t)

	module := ft.ensureReadyModule(t)
	operatorCfg := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      modulemeta.OperatorConfigName,
			Namespace: support.OperatorNamespace(),
		},
	}
	workloadDeploy := workloadDeployment()

	g.Eventually(t.Context(), k8sm.Lookup(ft.Client, operatorCfg)).Should(Succeed())

	g.Eventually(t.Context(), k8sm.Get(ft.Client, workloadDeploy)).Should(And(
		k8sm.HasLabel(labels.PlatformPartOf, componentsv1alpha1.DataSciencePipelinesComponentName),
		k8sm.HasAnnotation(annotations.InstanceName, module.GetName()),
		k8sm.HasAnnotation(annotations.InstanceUID, string(module.GetUID())),
		k8sm.HasAnnotation(annotations.PlatformType, operatorCfg.Data[moduleconfig.KeyPlatformName]),
		k8sm.HasAnnotation(annotations.PlatformVersion, module.Status.Release.Version.String()),
	))
}

func (ft *foundationTests) testOwnerReferences(t *testing.T) {
	g := NewWithT(t)

	owner := ft.ensureReadyModule(t)
	workloadDeploy := workloadDeployment()
	owner.TypeMeta = metav1.TypeMeta{
		APIVersion: componentsv1alpha1.GroupVersion.String(),
		Kind:       componentsv1alpha1.DataSciencePipelinesKind,
	}

	g.Eventually(t.Context(), k8sm.Get(ft.Client, workloadDeploy)).Should(
		k8sm.HasOwnerReference(owner),
	)
}
