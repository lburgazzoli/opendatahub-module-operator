//go:build integration

package integration

import (
	"testing"
	"time"

	. "github.com/onsi/gomega"

	"github.com/lburgazzoli/gomega-matchers/pkg/matchers/jq"

	componentsv1alpha1 "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-mymodule-operator/api/components/v1alpha1"
	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-mymodule-operator/internal/controller/mymodule"
	moduleconfig "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-mymodule-operator/pkg/config"
	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-mymodule-operator/pkg/version"
	"github.com/opendatahub-io/operator-actions-framework/resources"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type foundationTests struct {
	*myModuleTest
}

func (ft *foundationTests) Execute(t *testing.T) {
	t.Run("should have module CRD installed", ft.testModuleCRDInstalled)
	t.Run("should block when Ingress is missing", ft.testIngressBlocks)
	t.Run("should recover when Ingress is created", ft.testIngressRecovers)
	t.Run("should expose config values", ft.testConfigValues)
	t.Run("should report module version and platform", ft.testModuleStatus)
	t.Run("should set platform labels and annotations", ft.testPlatformLabels)
	t.Run("should set owner references", ft.testOwnerReferences)
	t.Run("should not annotate ingress on fresh install", ft.testUpgradeAnnotationAbsentOnFreshInstall)
	t.Run("should annotate ingress on upgrade", ft.testUpgradeAnnotatesIngress)
	t.Run("should not update version on upgrade fault", ft.testUpgradeFaultInjection)
}

func (ft *foundationTests) testModuleCRDInstalled(t *testing.T) {
	g := NewWithT(t)

	g.Eventually(k.Get(ft.moduleCRD)).WithContext(ctx).WithTimeout(timeout).WithPolling(interval).Should(
		jq.Match(`.metadata.name == "%s"`, moduleCRDName),
	)
}

func (ft *foundationTests) testIngressBlocks(t *testing.T) {
	g := NewWithT(t)

	_ = k8sClient.Delete(ctx, ft.module)
	_ = k8sClient.Delete(ctx, ft.ingress)

	ft.module.ResourceVersion = ""
	g.Expect(k8sClient.Create(ctx, ft.module)).To(Succeed())

	g.Eventually(k.Get(ft.module)).WithContext(ctx).WithTimeout(timeout).WithPolling(interval).Should(And(
		jq.Match(`.status.phase == "Not Ready"`),
		jq.Match(`[(.status.conditions // [])[] | select(.type == "%s" and .status == "False")] | length > 0`,
			mymodule.ConditionIngressAvailable),
	))
}

func (ft *foundationTests) testIngressRecovers(t *testing.T) {
	g := NewWithT(t)

	ft.ingress.ResourceVersion = ""
	g.Expect(k8sClient.Create(ctx, ft.ingress)).To(Succeed())

	g.Eventually(k.Get(ft.module)).WithContext(ctx).WithTimeout(timeout).WithPolling(interval).Should(And(
		jq.Match(`.status.phase == "Ready"`),
		jq.Match(`.status.conditions[] | select(.type == "Ready") | .status == "True"`),
		jq.Match(`.status.conditions[] | select(.type == "%s") | .status == "True"`,
			mymodule.ConditionIngressAvailable),
		jq.Match(`.status.conditions[] | select(.type == "ProvisioningSucceeded") | .status == "True"`),
	))

	eventuallyDeploymentReady(t, ft.workloadDeploy)
}

func (ft *foundationTests) testConfigValues(t *testing.T) {
	g := NewWithT(t)

	g.Eventually(k.Get(ft.module)).WithContext(ctx).WithTimeout(timeout).WithPolling(interval).Should(And(
		jq.Match(`.status.configValues."%s" == "%s"`,
			moduleconfig.KeyPlatformType,
			operatorCfgData[moduleconfig.KeyPlatformType]),
		jq.Match(`.status.configValues."%s" == "%s"`,
			moduleconfig.KeyPlatformVersion,
			operatorCfgData[moduleconfig.KeyPlatformVersion]),
	))
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
		jq.Match(`.metadata.labels."%s" == "mymodule"`, labelPartOf),
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

	g.Eventually(k.Get(ft.workloadService)).WithContext(ctx).WithTimeout(timeout).WithPolling(interval).Should(And(
		jq.Match(`.metadata.labels."%s" == "mymodule"`, labelPartOf),
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
		jq.Match(`.metadata.ownerReferences[] | select(.kind == "MyModule") | .name == "%s"`,
			componentsv1alpha1.MyModuleInstanceName),
	)

	g.Eventually(k.Get(ft.workloadService)).WithContext(ctx).WithTimeout(timeout).WithPolling(interval).Should(
		jq.Match(`.metadata.ownerReferences[] | select(.kind == "MyModule") | .name == "%s"`,
			componentsv1alpha1.MyModuleInstanceName),
	)
}

func (ft *foundationTests) testUpgradeAnnotationAbsentOnFreshInstall(t *testing.T) {
	g := NewWithT(t)

	g.Eventually(k.Get(ft.ingress)).WithContext(ctx).WithTimeout(timeout).WithPolling(interval).Should(And(
		jq.Match(`.metadata.annotations // {} | has("%s") | not`, mymodule.AnnotationManagedVersion),
		jq.Match(`.metadata.annotations // {} | has("%s") | not`, mymodule.AnnotationUpgradedFrom),
	))
}

func (ft *foundationTests) testUpgradeAnnotatesIngress(t *testing.T) {
	g := NewWithT(t)

	g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(ft.module), ft.module)).To(Succeed())

	patch := client.MergeFrom(ft.module.DeepCopy())
	ft.module.Status.Module.Version = "0.0.0-0"
	g.Expect(k8sClient.Status().Patch(ctx, ft.module, patch)).To(Succeed())

	g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(ft.module), ft.module)).To(Succeed())

	annPatch := client.MergeFrom(ft.module.DeepCopy())
	resources.SetAnnotation(ft.module, "test-trigger", time.Now().String())
	g.Expect(k8sClient.Patch(ctx, ft.module, annPatch)).To(Succeed())

	g.Eventually(k.Get(ft.ingress)).WithContext(ctx).WithTimeout(timeout).WithPolling(interval).Should(And(
		jq.Match(`.metadata.annotations."%s" != ""`, mymodule.AnnotationManagedVersion),
		jq.Match(`.metadata.annotations."%s" == "0.0.0-0"`, mymodule.AnnotationUpgradedFrom),
	))
}

func (ft *foundationTests) testUpgradeFaultInjection(t *testing.T) {
	g := NewWithT(t)

	g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(ft.module), ft.module)).To(Succeed())
	versionBefore := ft.module.Status.Module.Version.String()

	g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(ft.module), ft.module)).To(Succeed())

	statusPatch := client.MergeFrom(ft.module.DeepCopy())
	ft.module.Status.Module.Version = "0.0.0-0"
	g.Expect(k8sClient.Status().Patch(ctx, ft.module, statusPatch)).To(Succeed())

	g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(ft.ingress), ft.ingress)).To(Succeed())

	ingressPatch := client.MergeFrom(ft.ingress.DeepCopy())
	resources.SetAnnotation(ft.ingress, mymodule.AnnotationInjectUpgradeFault, "true")
	g.Expect(k8sClient.Patch(ctx, ft.ingress, ingressPatch)).To(Succeed())

	g.Consistently(k.Get(ft.module)).WithContext(ctx).WithTimeout(10 * time.Second).WithPolling(interval).Should(
		jq.Match(`.status.module.version == "0.0.0-0"`),
	)

	g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(ft.ingress), ft.ingress)).To(Succeed())

	cleanPatch := client.MergeFrom(ft.ingress.DeepCopy())
	ann := ft.ingress.GetAnnotations()
	delete(ann, mymodule.AnnotationInjectUpgradeFault)
	ft.ingress.SetAnnotations(ann)
	g.Expect(k8sClient.Patch(ctx, ft.ingress, cleanPatch)).To(Succeed())

	g.Eventually(k.Get(ft.module)).WithContext(ctx).WithTimeout(timeout).WithPolling(interval).Should(
		jq.Match(`.status.module.version == "%s"`, versionBefore),
	)
}
