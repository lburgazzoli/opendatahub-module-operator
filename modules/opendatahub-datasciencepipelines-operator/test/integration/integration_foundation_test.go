//go:build integration

package integration

import (
	"testing"

	. "github.com/onsi/gomega"

	"github.com/lburgazzoli/gomega-matchers/pkg/matchers/jq"

	componentsv1alpha1 "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-datasciencepipelines-operator/api/components/v1alpha1"
	moduleconfig "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-datasciencepipelines-operator/pkg/config"
	operatorstatus "github.com/opendatahub-io/opendatahub-operator/v2/pkg/controller/status"
)

type foundationTests struct {
	*dspIntegrationTest
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

func (ft *foundationTests) testModuleCRDInstalled(t *testing.T) {
	g := NewWithT(t)
	g.Eventually(k.Get(ft.moduleCRD)).WithContext(ctx).WithTimeout(timeout).WithPolling(interval).Should(
		jq.Match(`.metadata.name == "%s"`, moduleCRDName),
	)
}

func (ft *foundationTests) testMissingArgoWorkflowCRD(t *testing.T) {
	ensureArgoWorkflowCRDMissing(t)

	module := ft.module.DeepCopy()
	module.Spec.ArgoWorkflowsControllers = &componentsv1alpha1.ArgoWorkflowsControllersSpec{
		ManagementState: "Removed",
	}
	createModule(t, module)

	g := NewWithT(t)
	g.Eventually(k.Get(module)).WithContext(ctx).WithTimeout(timeout).WithPolling(interval).Should(And(
		jq.Match(`.status.conditions[]? | select(.type == "%s") | .status == "False"`,
			operatorstatus.ConditionArgoWorkflowAvailable),
		jq.Match(`.status.conditions[]? | select(.type == "%s") | .reason == "%s"`,
			operatorstatus.ConditionArgoWorkflowAvailable,
			operatorstatus.DataSciencePipelinesArgoWorkflowsCRDMissingReason),
		jq.Match(`.status.conditions[]? | select(.type == "%s") | .status == "False"`,
			operatorstatus.ConditionTypeReady),
	))
}

func (ft *foundationTests) testForeignOwnedArgoWorkflowCRD(t *testing.T) {
	ensureArgoWorkflowCRDForeignOwned(t)

	module := ft.module.DeepCopy()
	createModule(t, module)

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
	ensureArgoWorkflowCRDOwnedByODH(t)

	module := ft.module.DeepCopy()
	createModule(t, module)

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
	g.Eventually(k.Get(ft.workloadConfigMap)).WithContext(ctx).WithTimeout(timeout).WithPolling(interval).Should(
		jq.Match(`.metadata.name == "%s"`, workloadConfigMapName),
	)
	g.Eventually(k.Get(ft.workloadServiceMon)).WithContext(ctx).WithTimeout(timeout).WithPolling(interval).Should(
		jq.Match(`.metadata.name == "%s"`, workloadServiceMonName),
	)
}

func (ft *foundationTests) testReleaseStatus(t *testing.T) {
	ensureArgoWorkflowCRDOwnedByODH(t)

	module := ft.module.DeepCopy()
	createModule(t, module)

	g := NewWithT(t)
	g.Eventually(k.Get(module)).WithContext(ctx).WithTimeout(timeout).WithPolling(interval).Should(And(
		jq.Match(`.status.release.version == "%s"`, operatorReleaseVersion),
		jq.Match(`.status.release.name == "%s"`,
			operatorCfgData[moduleconfig.KeyPlatformName]),
	))
}

func (ft *foundationTests) testPlatformLabels(t *testing.T) {
	ensureArgoWorkflowCRDOwnedByODH(t)

	module := ft.module.DeepCopy()
	createModule(t, module)

	g := NewWithT(t)
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
			operatorCfgData[moduleconfig.KeyPlatformName]),
		jq.Match(`.metadata.annotations."%s" == "%s"`,
			annotationVersion,
			operatorReleaseVersion),
	))
}

func (ft *foundationTests) testOwnerReferences(t *testing.T) {
	ensureArgoWorkflowCRDOwnedByODH(t)

	module := ft.module.DeepCopy()
	createModule(t, module)

	g := NewWithT(t)
	g.Eventually(k.Get(ft.workloadDeploy)).WithContext(ctx).WithTimeout(timeout).WithPolling(interval).Should(
		jq.Match(`.metadata.ownerReferences[] | select(.kind == "DataSciencePipelines") | .name == "%s"`,
			componentsv1alpha1.DataSciencePipelinesInstanceName),
	)
}
