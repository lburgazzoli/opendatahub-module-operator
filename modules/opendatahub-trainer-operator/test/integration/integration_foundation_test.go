//go:build integration

package integration

import (
	"testing"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"

	"github.com/lburgazzoli/gomega-matchers/pkg/matchers/jq"

	componentsv1alpha1 "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-trainer-operator/api/components/v1alpha1"
	moduleconfig "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-trainer-operator/pkg/config"
	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-trainer-operator/pkg/version"
	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-trainer-operator/test/support"
	"github.com/opendatahub-io/opendatahub-operator/v2/pkg/controller/status"
)

type foundationTests struct {
	*trainerTest
}

func (ft *foundationTests) cleanupModuleWorkload(t *testing.T) {
	t.Helper()

	serviceAccount := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      workloadDeployName,
			Namespace: ft.workloadDeploy.Namespace,
		},
	}

	_ = k8sClient.Delete(ctx, ft.module)
	waitForSingletonDeleted(t, ft.module)
	waitForDeleted(t, ft.workloadDeploy)
	waitForDeleted(t, serviceAccount)
}

func (ft *foundationTests) Execute(t *testing.T) {
	t.Run("should have module CRD installed", ft.testModuleCRDInstalled)
	t.Run("should become ready", ft.testBecomesReady)
	t.Run("should report module version and platform", ft.testModuleStatus)
	t.Run("should set platform labels and annotations", ft.testPlatformLabels)
	t.Run("should set owner references", ft.testOwnerReferences)
	t.Run("should report not ready when JobSet operator CRD is missing", ft.testJobSetOperatorCRDMissing)
	t.Run("should report not ready when JobSet operator CR is missing", ft.testJobSetOperatorCRMissing)
	t.Run("should report not ready when JobSet CRD is missing", ft.testJobSetCRDMissing)
}

func (ft *foundationTests) testModuleCRDInstalled(t *testing.T) {
	g := NewWithT(t)

	g.Eventually(k.Get(ft.moduleCRD)).WithContext(ctx).WithTimeout(timeout).WithPolling(interval).Should(
		jq.Match(`.metadata.name == "%s"`, moduleCRDName),
	)
}

func (ft *foundationTests) expectDependenciesUnavailable(
	t *testing.T,
	module *componentsv1alpha1.Trainer,
	expectedMessage string,
) {
	t.Helper()

	g := NewWithT(t)
	g.Eventually(k.Get(module)).WithContext(ctx).WithTimeout(timeout).WithPolling(interval).Should(And(
		jq.Match(`(.status.conditions // []) | any(.[]; .type == "%s" and .status == "%s")`,
			status.ConditionDependenciesAvailable, metav1.ConditionFalse),
		jq.Match(`(.status.conditions // []) | any(.[]; .type == "%s" and .reason == "%s")`,
			status.ConditionDependenciesAvailable, "PreConditionFailed"),
		jq.Match(`(.status.conditions // []) | any(.[]; .type == "%s" and .message == "%s")`,
			status.ConditionDependenciesAvailable, expectedMessage),
		jq.Match(`(.status.conditions // []) | any(.[]; .type == "%s" and .status == "%s")`,
			status.ConditionTypeReady, metav1.ConditionFalse),
		jq.Match(`(.status.conditions // []) | any(.[]; .type == "%s" and .status == "%s")`,
			status.ConditionTypeProvisioningSucceeded, metav1.ConditionFalse),
	))
}

func (ft *foundationTests) testJobSetOperatorCRDMissing(t *testing.T) {
	g := NewWithT(t)
	module := ft.module.DeepCopy()
	ft.cleanupModuleWorkload(t)

	jobSetOperatorCRD := &apiextensionsv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: jobSetOperatorCRDName},
	}
	g.Expect(directClient.Delete(ctx, jobSetOperatorCRD)).To(Succeed())
	waitForDeleted(t, jobSetOperatorCRD)
	t.Cleanup(func() {
		_, _ = support.EnsureStubCRDIfMissing(
			ctx,
			directClient,
			jobSetOperatorCRDName,
			"operator.openshift.io",
			"v1",
			"JobSetOperator",
			"jobsetoperators",
		)
		_, _ = support.EnsureStubJobSetOperatorCRIfMissing(ctx, directClient)
		_ = k8sClient.Delete(ctx, module)
		waitForSingletonDeleted(t, module)
		waitForDeleted(t, ft.workloadDeploy)
	})

	module.ResourceVersion = ""
	g.Expect(k8sClient.Create(ctx, module)).To(Succeed())
	ft.expectDependenciesUnavailable(t, module, status.JobSetOperatorNotInstalledMessage)
}

func (ft *foundationTests) testJobSetOperatorCRMissing(t *testing.T) {
	g := NewWithT(t)
	module := ft.module.DeepCopy()
	ft.cleanupModuleWorkload(t)

	jobSetOperatorCR := support.NewStubJobSetOperatorCR()
	g.Expect(directClient.Delete(ctx, jobSetOperatorCR)).To(Succeed())
	waitForDeleted(t, jobSetOperatorCR)
	t.Cleanup(func() {
		_, _ = support.EnsureStubJobSetOperatorCRIfMissing(ctx, directClient)
		_ = k8sClient.Delete(ctx, module)
		waitForSingletonDeleted(t, module)
		waitForDeleted(t, ft.workloadDeploy)
	})

	module.ResourceVersion = ""
	g.Expect(k8sClient.Create(ctx, module)).To(Succeed())
	ft.expectDependenciesUnavailable(t, module, status.JobSetOperatorCRNotFoundMessage)
}

func (ft *foundationTests) testJobSetCRDMissing(t *testing.T) {
	g := NewWithT(t)
	module := ft.module.DeepCopy()
	ft.cleanupModuleWorkload(t)

	jobSetCRD := &apiextensionsv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: jobSetCRDName},
	}
	g.Expect(directClient.Delete(ctx, jobSetCRD)).To(Succeed())
	waitForDeleted(t, jobSetCRD)
	t.Cleanup(func() {
		_, _ = support.EnsureStubCRDIfMissing(
			ctx,
			directClient,
			jobSetCRDName,
			"jobset.x-k8s.io",
			"v1alpha2",
			"JobSet",
			"jobsets",
		)
		_ = k8sClient.Delete(ctx, module)
		waitForSingletonDeleted(t, module)
		waitForDeleted(t, ft.workloadDeploy)
	})

	module.ResourceVersion = ""
	g.Expect(k8sClient.Create(ctx, module)).To(Succeed())
	ft.expectDependenciesUnavailable(t, module, status.JobSetCRDMissingMessage)
}

func (ft *foundationTests) testBecomesReady(t *testing.T) {
	g := NewWithT(t)

	ft.module.ResourceVersion = ""
	g.Expect(k8sClient.Create(ctx, ft.module)).To(Succeed())

	g.Eventually(k.Get(ft.module)).WithContext(ctx).WithTimeout(timeout).WithPolling(interval).Should(And(
		jq.Match(`.status.phase == "Ready"`),
		jq.Match(`.status.conditions[] | select(.type == "Ready") | .status == "True"`),
		jq.Match(`.status.conditions[] | select(.type == "ProvisioningSucceeded") | .status == "True"`),
	))

	eventuallyDeploymentReady(t, ft.workloadDeploy)
}

func (ft *foundationTests) testModuleStatus(t *testing.T) {
	g := NewWithT(t)

	g.Eventually(k.Get(ft.module)).WithContext(ctx).WithTimeout(timeout).WithPolling(interval).Should(And(
		jq.Match(`.status.module.version == "%s"`, version.Version),
		jq.Match(`.status.module.buildSource == "%s"`,
			version.BuildSource()),
		jq.Match(`.status.module.platform.name == "%s"`,
			operatorCfgData[moduleconfig.KeyPlatformType]),
		jq.Match(`.status.module.platform.version == "%s"`,
			operatorReleaseVersion),
		jq.Match(`.status.module.sources | length > 0`),
		jq.Match(`.status.module.sources[0].path != ""`),
		jq.Match(`.status.module.sources[0].renderer == "kustomize"`),
	))
}

func (ft *foundationTests) testPlatformLabels(t *testing.T) {
	g := NewWithT(t)

	g.Eventually(k.Get(ft.workloadDeploy)).WithContext(ctx).WithTimeout(timeout).WithPolling(interval).Should(And(
		jq.Match(`.metadata.labels."%s" == "trainer"`, labelPartOf),
		jq.Match(`.metadata.annotations."%s" == "%s"`,
			annotationInstanceName,
			ft.module.GetName()),
		jq.Match(`.metadata.annotations."%s" == "%s"`,
			annotationInstanceUID,
			string(ft.module.GetUID())),
		jq.Match(`.metadata.annotations."%s" == "%s"`,
			annotationType,
			operatorCfgData[moduleconfig.KeyPlatformType]),
		jq.Match(`.metadata.annotations."%s" == "%s"`,
			annotationVersion,
			operatorReleaseVersion),
	))
}

func (ft *foundationTests) testOwnerReferences(t *testing.T) {
	g := NewWithT(t)

	g.Eventually(k.Get(ft.workloadDeploy)).WithContext(ctx).WithTimeout(timeout).WithPolling(interval).Should(
		jq.Match(`.metadata.ownerReferences[] | select(.kind == "Trainer") | .name == "%s"`,
			componentsv1alpha1.TrainerInstanceName),
	)
}
