//go:build e2e

package e2e

import (
	"testing"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/lburgazzoli/gomega-matchers/pkg/matchers/jq"

	componentsv1alpha1 "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-datasciencepipelines-operator/api/components/v1alpha1"
	moduleconfig "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-datasciencepipelines-operator/pkg/config"
	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-datasciencepipelines-operator/test/support"
	operatorstatus "github.com/opendatahub-io/opendatahub-operator/v2/pkg/controller/status"
)

type foundationTests struct {
	*dspE2ETest
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

func (ft *foundationTests) testModuleCRDInstalled(t *testing.T) {
	g := NewWithT(t)
	g.Eventually(k.Get(ft.moduleCRD)).WithContext(ctx).WithTimeout(timeout).WithPolling(interval).Should(
		jq.Match(`.metadata.name == "%s"`, moduleCRDName),
	)
}

func (ft *foundationTests) testOperatorConfigMap(t *testing.T) {
	g := NewWithT(t)
	g.Eventually(k.Get(ft.operatorCfgMap)).WithContext(ctx).WithTimeout(timeout).WithPolling(interval).Should(And(
		jq.Match(`.data."%s" != ""`, moduleconfig.KeyPlatformName),
		jq.Match(`.data."%s" != ""`, moduleconfig.KeyPlatformVersion),
	))
}

func (ft *foundationTests) testMissingArgoWorkflowCRD(t *testing.T) {
	module := ft.module.DeepCopy()
	withStoppedOperator(t, func() {
		ensureArgoWorkflowCRDMissing(t)

		module.Spec.ArgoWorkflowsControllers = &componentsv1alpha1.ArgoWorkflowsControllersSpec{
			ManagementState: "Removed",
		}
		createModule(t, module)
	})

	g := NewWithT(t)
	g.Eventually(k.Get(module)).WithContext(ctx).WithTimeout(timeout).WithPolling(interval).Should(And(
		jq.Match(`.status.conditions[]? | select(.type == "%s") | .status == "False"`,
			operatorstatus.ConditionArgoWorkflowAvailable),
		jq.Match(`.status.conditions[]? | select(.type == "%s") | .reason == "%s"`,
			operatorstatus.ConditionArgoWorkflowAvailable,
			operatorstatus.DataSciencePipelinesArgoWorkflowsCRDMissingReason),
	))
}

func (ft *foundationTests) testForeignOwnedArgoWorkflowCRD(t *testing.T) {
	module := ft.module.DeepCopy()
	withStoppedOperator(t, func() {
		ensureArgoWorkflowCRDForeignOwned(t)
		createModule(t, module)
	})

	g := NewWithT(t)
	g.Eventually(k.Get(module)).WithContext(ctx).WithTimeout(timeout).WithPolling(interval).Should(And(
		jq.Match(`.status.conditions[]? | select(.type == "%s") | .status == "False"`,
			operatorstatus.ConditionArgoWorkflowAvailable),
		jq.Match(`.status.conditions[]? | select(.type == "%s") | .reason == "%s"`,
			operatorstatus.ConditionArgoWorkflowAvailable,
			operatorstatus.DataSciencePipelinesDoesntOwnArgoCRDReason),
	))
}

func (ft *foundationTests) testBecomesReady(t *testing.T) {
	module := ft.module.DeepCopy()
	withStoppedOperator(t, func() {
		ensureArgoWorkflowCRDOwnedByODH(t)
		createModule(t, module)
	})

	g := NewWithT(t)
	g.Eventually(k.Get(module)).WithContext(ctx).WithTimeout(timeout).WithPolling(interval).Should(And(
		jq.Match(`.status.phase == "Ready"`),
		jq.Match(`.status.conditions[]? | select(.type == "%s") | .status == "True"`,
			operatorstatus.ConditionTypeReady),
		jq.Match(`.status.conditions[]? | select(.type == "%s") | .status == "True"`,
			operatorstatus.ConditionArgoWorkflowAvailable),
		jq.Match(`.status.conditions[]? | select(.type == "%s") | .status == "True"`,
			operatorstatus.ConditionTypeProvisioningSucceeded),
	))

	eventuallyDeploymentReady(t, ft.workloadDeploy)
	g.Eventually(k.Get(ft.workloadServiceMon)).WithContext(ctx).WithTimeout(timeout).WithPolling(interval).Should(
		jq.Match(`.metadata.name == "%s"`, workloadServiceMonName),
	)
}

func (ft *foundationTests) testReleaseStatus(t *testing.T) {
	module := ft.module.DeepCopy()
	operatorCfg := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      operatorConfigMapName,
			Namespace: support.OperatorNamespace(),
		},
	}
	withStoppedOperator(t, func() {
		ensureArgoWorkflowCRDOwnedByODH(t)
		createModule(t, module)
	})

	g := NewWithT(t)
	g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(operatorCfg), operatorCfg)).To(Succeed())

	cfg := &moduleconfig.Config{
		PlatformName:    operatorCfg.Data[moduleconfig.KeyPlatformName],
		PlatformVersion: operatorCfg.Data[moduleconfig.KeyPlatformVersion],
	}
	expectedRelease := cfg.Release()

	g.Eventually(k.Get(module)).WithContext(ctx).WithTimeout(timeout).WithPolling(interval).Should(And(
		jq.Match(`.status.release.version == "%s"`, expectedRelease.Version.String()),
		jq.Match(`.status.release.name == "%s"`, string(expectedRelease.Name)),
	))
}

func (ft *foundationTests) testPlatformLabels(t *testing.T) {
	module := ft.module.DeepCopy()
	operatorCfg := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      operatorConfigMapName,
			Namespace: support.OperatorNamespace(),
		},
	}
	withStoppedOperator(t, func() {
		ensureArgoWorkflowCRDOwnedByODH(t)
		createModule(t, module)
	})

	g := NewWithT(t)
	g.Eventually(k.Get(module)).WithContext(ctx).WithTimeout(timeout).WithPolling(interval).Should(
		jq.Match(`.status.release.version != ""`),
	)
	g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(module), module)).To(Succeed())
	g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(operatorCfg), operatorCfg)).To(Succeed())
	g.Eventually(k.Get(ft.workloadDeploy)).WithContext(ctx).WithTimeout(timeout).WithPolling(interval).Should(And(
		jq.Match(`.metadata.labels."%s" == "%s"`, labelPartOf, componentsv1alpha1.DataSciencePipelinesComponentName),
		jq.Match(`.metadata.annotations."%s" == "%s"`,
			annotationInstanceName,
			module.GetName()),
		jq.Match(`.metadata.annotations."%s" == "%s"`,
			annotationInstanceUID,
			string(module.GetUID())),
		jq.Match(`.metadata.annotations."%s" == "%s"`,
			annotationType,
			operatorCfg.Data[moduleconfig.KeyPlatformName]),
		jq.Match(`.metadata.annotations."%s" == "%s"`,
			annotationVersion,
			module.Status.Release.Version),
	))
}

func (ft *foundationTests) testOwnerReferences(t *testing.T) {
	module := ft.module.DeepCopy()
	withStoppedOperator(t, func() {
		ensureArgoWorkflowCRDOwnedByODH(t)
		createModule(t, module)
	})

	g := NewWithT(t)
	g.Eventually(k.Get(ft.workloadDeploy)).WithContext(ctx).WithTimeout(timeout).WithPolling(interval).Should(
		jq.Match(`.metadata.ownerReferences[] | select(.kind == "DataSciencePipelines") | .name == "%s"`,
			componentsv1alpha1.DataSciencePipelinesInstanceName),
	)
}
