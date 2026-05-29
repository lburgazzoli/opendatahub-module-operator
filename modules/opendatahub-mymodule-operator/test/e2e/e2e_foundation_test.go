//go:build e2e

package e2e

import (
	"testing"
	"time"

	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/lburgazzoli/gomega-matchers/pkg/matchers/jq"

	componentsv1alpha1 "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-mymodule-operator/api/components/v1alpha1"
	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-mymodule-operator/internal/controller/mymodule"
	moduleconfig "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-mymodule-operator/pkg/config"
	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-mymodule-operator/pkg/version"
)

type foundationTests struct {
	*myModuleE2ETest
}

func (ft *foundationTests) Execute(t *testing.T) {
	t.Run("should have module CRD installed", ft.testModuleCRDInstalled)
	t.Run("should have ConfigMap volume mounted", ft.testConfigMapVolume)
	t.Run("should have operator ConfigMap deployed", ft.testOperatorConfigMap)
	t.Run("should block when Ingress is missing", ft.testIngressBlocks)
	t.Run("should recover when Ingress is created", ft.testIngressRecovers)
	t.Run("should expose config values", ft.testConfigValues)
	t.Run("should report module version and platform", ft.testModuleStatus)
	t.Run("should set platform labels and annotations", ft.testPlatformLabels)
	t.Run("should have webhook-injected labels", ft.testWebhookLabels)
	t.Run("should set owner references", ft.testOwnerReferences)
	t.Run("should not annotate ingress on fresh deploy", ft.testUpgradeAnnotationAbsentOnFreshDeploy)
	t.Run("should annotate ingress on upgrade via configmap restart", ft.testUpgradeViaConfigMapRestart)
	t.Run("should not update version on upgrade fault", ft.testUpgradeFaultInjection)
}

func (ft *foundationTests) testModuleCRDInstalled(t *testing.T) {
	g := NewWithT(t)

	g.Eventually(k.Get(ft.moduleCRD)).WithContext(ctx).WithTimeout(timeout).WithPolling(interval).Should(
		jq.Match(`.metadata.name == "%s"`, moduleCRDName),
	)
}

func (ft *foundationTests) testConfigMapVolume(t *testing.T) {
	g := NewWithT(t)

	g.Eventually(k.Get(ft.operatorDeploy)).WithContext(ctx).WithTimeout(timeout).WithPolling(interval).Should(
		jq.Match(`.spec.template.spec.volumes[] | select(.name == "config") | .configMap.name != ""`),
	)

	g.Eventually(k.Get(ft.operatorDeploy)).WithContext(ctx).WithTimeout(timeout).WithPolling(interval).Should(
		jq.Match(`.spec.template.spec.containers[0].volumeMounts[] | select(.name == "config") | .mountPath == "/etc/controller/config"`),
	)

	g.Eventually(k.Get(ft.operatorDeploy)).WithContext(ctx).WithTimeout(timeout).WithPolling(interval).Should(
		jq.Match(`.spec.template.spec.containers[0].env[] | select(.name == "ODH_MODULE_OPERATOR_CONFIGURATION_PATH") | .value == "/etc/controller/config"`),
	)

	g.Eventually(k.Get(ft.operatorDeploy)).WithContext(ctx).WithTimeout(timeout).WithPolling(interval).Should(
		jq.Match(`.spec.template.spec.containers[0].env[] | select(.name == "ODH_MODULE_OPERATOR_NAMESPACE") | .valueFrom.fieldRef.fieldPath == "metadata.namespace"`),
	)
}

func (ft *foundationTests) testOperatorConfigMap(t *testing.T) {
	g := NewWithT(t)

	g.Eventually(k.Get(ft.operatorCfgMap)).WithContext(ctx).WithTimeout(timeout).WithPolling(interval).Should(And(
		jq.Match(`.data."%s" != ""`, moduleconfig.KeyPlatformType),
		jq.Match(`.data."%s" != ""`, moduleconfig.KeyPlatformVersion),
	))
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
		jq.Match(`.status.configValues."%s" != ""`, moduleconfig.KeyPlatformType),
		jq.Match(`.status.configValues."%s" != ""`, moduleconfig.KeyPlatformVersion),
	))
}

func (ft *foundationTests) testModuleStatus(t *testing.T) {
	g := NewWithT(t)
	operatorCfg := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      operatorConfigMapName,
			Namespace: defaultOperatorNamespace,
		},
	}
	workloadDeploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "mymodule-workload",
			Namespace: defaultOperatorNamespace,
		},
	}

	g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(operatorCfg), operatorCfg)).To(Succeed())
	g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(workloadDeploy), workloadDeploy)).To(Succeed())

	platformType := operatorCfg.Data[moduleconfig.KeyPlatformType]
	workloadVersion := workloadDeploy.Annotations[annotationVersion]

	g.Eventually(k.Get(ft.module)).WithContext(ctx).WithTimeout(timeout).WithPolling(interval).Should(And(
		jq.Match(`.status.module.version == "%s"`, version.Version),
		jq.Match(`.status.module.buildSource == "%s"`,
			version.BuildSource()),
		jq.Match(`.status.module.platform.name == "%s"`, platformType),
		jq.Match(`.status.module.platform.version == "%s"`, workloadVersion),
		jq.Match(`.status.module.sources | length > 0`),
		jq.Match(`.status.module.sources[0].path != ""`),
		jq.Match(`.status.module.sources[0].renderer == "kustomize"`),
	))
}

func (ft *foundationTests) testPlatformLabels(t *testing.T) {
	g := NewWithT(t)
	module := ft.module.DeepCopy()
	operatorCfg := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      operatorConfigMapName,
			Namespace: defaultOperatorNamespace,
		},
	}

	g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(module), module)).To(Succeed())
	g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(operatorCfg), operatorCfg)).To(Succeed())

	g.Eventually(k.Get(ft.workloadDeploy)).WithContext(ctx).WithTimeout(timeout).WithPolling(interval).Should(And(
		jq.Match(`.metadata.labels."%s" == "mymodule"`, labelPartOf),
		jq.Match(`.metadata.annotations."%s" == "%s"`,
			annotationInstanceName,
			module.GetName()),
		jq.Match(`.metadata.annotations."%s" == "%s"`,
			annotationInstanceUID,
			string(module.GetUID())),
		jq.Match(`.metadata.annotations."%s" == "%s"`,
			annotationType,
			operatorCfg.Data[moduleconfig.KeyPlatformType]),
		jq.Match(`.metadata.annotations."%s" == "%s"`,
			annotationVersion,
			module.Status.Module.Platform.Version.String()),
	))

	g.Eventually(k.Get(ft.workloadService)).WithContext(ctx).WithTimeout(timeout).WithPolling(interval).Should(And(
		jq.Match(`.metadata.labels."%s" == "mymodule"`, labelPartOf),
		jq.Match(`.metadata.annotations."%s" == "%s"`,
			annotationType,
			operatorCfg.Data[moduleconfig.KeyPlatformType]),
		jq.Match(`.metadata.annotations."%s" == "%s"`,
			annotationVersion,
			module.Status.Module.Platform.Version.String()),
	))
}

func (ft *foundationTests) testWebhookLabels(t *testing.T) {
	g := NewWithT(t)

	g.Eventually(k.Get(ft.workloadDeploy)).WithContext(ctx).WithTimeout(timeout).WithPolling(interval).Should(And(
		jq.Match(`.metadata.labels."mymodule.opendatahub.io/version" != ""`),
		jq.Match(`.metadata.labels."mymodule.opendatahub.io/platform" != ""`),
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

func (ft *foundationTests) testUpgradeAnnotationAbsentOnFreshDeploy(t *testing.T) {
	g := NewWithT(t)

	g.Eventually(k.Get(ft.ingress)).WithContext(ctx).WithTimeout(timeout).WithPolling(interval).Should(And(
		jq.Match(`.metadata.annotations // {} | has("%s") | not`, mymodule.AnnotationManagedVersion),
		jq.Match(`.metadata.annotations // {} | has("%s") | not`, mymodule.AnnotationUpgradedFrom),
	))
}

func (ft *foundationTests) testUpgradeViaConfigMapRestart(t *testing.T) {
	g := NewWithT(t)

	g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(ft.operatorCfgMap), ft.operatorCfgMap)).To(Succeed())

	patch := client.MergeFrom(ft.operatorCfgMap.DeepCopy())
	ft.operatorCfgMap.Data[moduleconfig.KeyPlatformVersion] = "1.0.0"
	g.Expect(k8sClient.Patch(ctx, ft.operatorCfgMap, patch)).To(Succeed())

	g.Expect(k8sClient.DeleteAllOf(ctx, &corev1.Pod{},
		client.InNamespace(operatorNamespace),
		client.MatchingLabels{"app.kubernetes.io/name": "opendatahub-mymodule-operator"},
	)).To(Succeed())

	eventuallyDeploymentReady(t, ft.operatorDeploy)

	g.Eventually(k.Get(ft.ingress)).WithContext(ctx).WithTimeout(timeout).WithPolling(interval).Should(And(
		jq.Match(`.metadata.annotations."%s" != ""`, mymodule.AnnotationManagedVersion),
		jq.Match(`.metadata.annotations."%s" != ""`, mymodule.AnnotationUpgradedFrom),
	))
}

func (ft *foundationTests) testUpgradeFaultInjection(t *testing.T) {
	g := NewWithT(t)

	g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(ft.module), ft.module)).To(Succeed())
	platformVersionBefore := ft.module.Status.Module.Platform.Version.String()

	g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(ft.ingress), ft.ingress)).To(Succeed())

	ingressPatch := client.MergeFrom(ft.ingress.DeepCopy())
	if ft.ingress.Annotations == nil {
		ft.ingress.Annotations = map[string]string{}
	}
	ft.ingress.Annotations[mymodule.AnnotationInjectUpgradeFault] = "true"
	g.Expect(k8sClient.Patch(ctx, ft.ingress, ingressPatch)).To(Succeed())

	g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(ft.operatorCfgMap), ft.operatorCfgMap)).To(Succeed())

	cfgPatch := client.MergeFrom(ft.operatorCfgMap.DeepCopy())
	ft.operatorCfgMap.Data[moduleconfig.KeyPlatformVersion] = "2.0.0"
	g.Expect(k8sClient.Patch(ctx, ft.operatorCfgMap, cfgPatch)).To(Succeed())

	pods := &corev1.PodList{}
	g.Expect(k8sClient.List(ctx, pods,
		client.InNamespace(operatorNamespace),
		client.MatchingLabels{"app.kubernetes.io/name": "opendatahub-mymodule-operator"},
	)).To(Succeed())

	for i := range pods.Items {
		g.Expect(k8sClient.Delete(ctx, &pods.Items[i])).To(Succeed())
	}

	eventuallyDeploymentReady(t, ft.operatorDeploy)

	g.Consistently(k.Get(ft.module)).WithContext(ctx).WithTimeout(10 * time.Second).WithPolling(interval).Should(
		jq.Match(`.status.module.platform.version == "%s"`, platformVersionBefore),
	)

	g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(ft.ingress), ft.ingress)).To(Succeed())

	cleanPatch := client.MergeFrom(ft.ingress.DeepCopy())
	ann := ft.ingress.GetAnnotations()
	delete(ann, mymodule.AnnotationInjectUpgradeFault)
	ft.ingress.SetAnnotations(ann)
	g.Expect(k8sClient.Patch(ctx, ft.ingress, cleanPatch)).To(Succeed())

	g.Eventually(k.Get(ft.module)).WithContext(ctx).WithTimeout(timeout).WithPolling(interval).Should(
		jq.Match(`.status.module.platform.version == "2.0.0"`),
	)
}
