//go:build e2e

package e2e

import (
	"testing"
	"testing/fstest"
	"time"

	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/lburgazzoli/gomega-matchers/pkg/matchers/jq"
	k8sm "github.com/lburgazzoli/gomega-matchers/pkg/matchers/k8s"

	componentsv1alpha1 "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-mymodule-operator/api/components/v1alpha1"
	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-mymodule-operator/internal/controller/mymodule"
	moduleconfig "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-mymodule-operator/pkg/config"
	modulemeta "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-mymodule-operator/pkg/module"
	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-mymodule-operator/test/support"
	"github.com/opendatahub-io/odh-platform-utilities/pkg/metadata/annotations"
	"github.com/opendatahub-io/odh-platform-utilities/pkg/metadata/labels"
)

type foundationTests struct {
	Client client.Client
}

func (ft *foundationTests) Execute(t *testing.T) {
	t.Run("should have module CRD installed", ft.testModuleCRDInstalled)
	t.Run("should have ConfigMap volume mounted", ft.testConfigMapVolume)
	t.Run("should have operator ConfigMap deployed", ft.testOperatorConfigMap)
	t.Run("should block when Ingress is missing", ft.testIngressBlocks)
	t.Run("should recover when Ingress is created", ft.testIngressRecovers)
	t.Run("should expose config values", ft.testConfigValues)
	t.Run("should report release version and platform", ft.testReleaseStatus)
	t.Run("should set platform labels and annotations", ft.testPlatformLabels)
	t.Run("should have webhook-injected labels", ft.testWebhookLabels)
	t.Run("should set owner references", ft.testOwnerReferences)
	t.Run("should not annotate ingress on fresh deploy", ft.testUpgradeAnnotationAbsentOnFreshDeploy)
	t.Run("should annotate ingress on upgrade via configmap restart", ft.testUpgradeViaConfigMapRestart)
	t.Run("should not update version on upgrade fault", ft.testUpgradeFaultInjection)
}

func moduleObject() *componentsv1alpha1.MyModule {
	return &componentsv1alpha1.MyModule{
		ObjectMeta: metav1.ObjectMeta{
			Name: componentsv1alpha1.MyModuleInstanceName,
		},
	}
}

func testIngress(namespace string) *networkingv1.Ingress {
	return &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      mymodule.IngressName,
			Namespace: namespace,
		},
		Spec: networkingv1.IngressSpec{
			DefaultBackend: &networkingv1.IngressBackend{
				Service: &networkingv1.IngressServiceBackend{
					Name: "mymodule-workload",
					Port: networkingv1.ServiceBackendPort{Number: 8080},
				},
			},
		},
	}
}

func workloadDeployment() *appsv1.Deployment {
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "mymodule-workload",
			Namespace: support.OperatorNamespace(),
		},
	}
}

func workloadService() *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "mymodule-workload",
			Namespace: support.OperatorNamespace(),
		},
	}
}

func (ft *foundationTests) cleanupModuleAndIngress(t *testing.T) {
	t.Helper()

	g := NewWithT(t)
	module := moduleObject()
	ingress := testIngress(support.OperatorNamespace())

	_ = ft.Client.Delete(t.Context(), module)
	_ = ft.Client.Delete(t.Context(), ingress)
	g.Eventually(t.Context(), k8sm.NotFound(ft.Client, module)).Should(BeTrue())
	g.Eventually(t.Context(), k8sm.NotFound(ft.Client, ingress)).Should(BeTrue())
}

func (ft *foundationTests) ensureReadyModule(t *testing.T) *componentsv1alpha1.MyModule {
	t.Helper()

	g := NewWithT(t)
	module := moduleObject()
	ingress := testIngress(support.OperatorNamespace())
	workloadDeploy := workloadDeployment()

	ft.cleanupModuleAndIngress(t)

	t.Cleanup(func() {
		_ = ft.Client.Delete(t.Context(), module)
		_ = ft.Client.Delete(t.Context(), ingress)
	})

	g.Expect(ft.Client.Create(t.Context(), ingress)).To(Succeed())
	g.Expect(ft.Client.Create(t.Context(), module)).To(Succeed())

	g.Eventually(t.Context(), k8sm.Get(ft.Client, module)).Should(And(
		jq.Match(`.status.phase == "Ready"`),
		WithTransform(k8sm.Conditions(), SatisfyAll(
			ContainElement(SatisfyAll(
				HaveKeyWithValue("type", "Ready"),
				HaveKeyWithValue("status", string(metav1.ConditionTrue)),
			)),
			ContainElement(SatisfyAll(
				HaveKeyWithValue("type", mymodule.ConditionIngressAvailable),
				HaveKeyWithValue("status", string(metav1.ConditionTrue)),
			)),
			ContainElement(SatisfyAll(
				HaveKeyWithValue("type", "ProvisioningSucceeded"),
				HaveKeyWithValue("status", string(metav1.ConditionTrue)),
			)),
		)),
	))

	g.Eventually(t.Context(), k8sm.Get(ft.Client, workloadDeploy)).Should(
		jq.Match(`.status.readyReplicas >= 1`),
	)

	return module
}

func (ft *foundationTests) testModuleCRDInstalled(t *testing.T) {
	g := NewWithT(t)

	moduleCRD := &apiextensionsv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: componentsv1alpha1.MyModuleCRDName},
	}
	g.Eventually(t.Context(), k8sm.Lookup(ft.Client, moduleCRD)).Should(Succeed())
}

func (ft *foundationTests) testConfigMapVolume(t *testing.T) {
	g := NewWithT(t)
	operatorDeploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "opendatahub-mymodule-operator",
			Namespace: support.OperatorNamespace(),
		},
	}

	g.Eventually(t.Context(), k8sm.Get(ft.Client, operatorDeploy)).Should(
		jq.Match(`.spec.template.spec.volumes[] | select(.name == "config") | .configMap.name != ""`),
	)
	g.Eventually(t.Context(), k8sm.Get(ft.Client, operatorDeploy)).Should(
		jq.Match(`.spec.template.spec.containers[0].volumeMounts[]` +
			` | select(.name == "config") | .mountPath == "/etc/controller/config"`),
	)
	g.Eventually(t.Context(), k8sm.Get(ft.Client, operatorDeploy)).Should(
		jq.Match(`.spec.template.spec.containers[0].env[]` +
			` | select(.name == "ODH_MODULE_OPERATOR_CONFIGURATION_PATH")` +
			` | .value == "/etc/controller/config"`),
	)
	g.Eventually(t.Context(), k8sm.Get(ft.Client, operatorDeploy)).Should(
		jq.Match(`.spec.template.spec.containers[0].env[]` +
			` | select(.name == "ODH_MODULE_OPERATOR_NAMESPACE")` +
			` | .valueFrom.fieldRef.fieldPath == "metadata.namespace"`),
	)
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

func (ft *foundationTests) testIngressBlocks(t *testing.T) {
	g := NewWithT(t)
	module := moduleObject()

	ft.cleanupModuleAndIngress(t)

	t.Cleanup(func() {
		_ = ft.Client.Delete(t.Context(), module)
	})

	g.Expect(ft.Client.Create(t.Context(), module)).To(Succeed())

	g.Eventually(t.Context(), k8sm.Get(ft.Client, module)).Should(And(
		jq.Match(`.status.phase == "Not Ready"`),
		WithTransform(k8sm.Conditions(), SatisfyAll(
			ContainElement(SatisfyAll(
				HaveKeyWithValue("type", mymodule.ConditionIngressAvailable),
				HaveKeyWithValue("status", "False"),
			)),
		)),
	))
}

func (ft *foundationTests) testIngressRecovers(t *testing.T) {
	g := NewWithT(t)
	module := moduleObject()
	ingress := testIngress(support.OperatorNamespace())
	workloadDeploy := workloadDeployment()

	ft.cleanupModuleAndIngress(t)

	t.Cleanup(func() {
		_ = ft.Client.Delete(t.Context(), module)
		_ = ft.Client.Delete(t.Context(), ingress)
	})

	g.Expect(ft.Client.Create(t.Context(), module)).To(Succeed())
	g.Expect(ft.Client.Create(t.Context(), ingress)).To(Succeed())

	g.Eventually(t.Context(), k8sm.Get(ft.Client, module)).Should(And(
		jq.Match(`.status.phase == "Ready"`),
		WithTransform(k8sm.Conditions(), SatisfyAll(
			ContainElement(SatisfyAll(
				HaveKeyWithValue("type", "Ready"),
				HaveKeyWithValue("status", string(metav1.ConditionTrue)),
			)),
			ContainElement(SatisfyAll(
				HaveKeyWithValue("type", mymodule.ConditionIngressAvailable),
				HaveKeyWithValue("status", string(metav1.ConditionTrue)),
			)),
			ContainElement(SatisfyAll(
				HaveKeyWithValue("type", "ProvisioningSucceeded"),
				HaveKeyWithValue("status", string(metav1.ConditionTrue)),
			)),
		)),
	))

	g.Eventually(t.Context(), k8sm.Get(ft.Client, workloadDeploy)).Should(
		jq.Match(`.status.readyReplicas >= 1`),
	)
}

func (ft *foundationTests) testConfigValues(t *testing.T) {
	g := NewWithT(t)

	module := ft.ensureReadyModule(t)

	g.Eventually(t.Context(), k8sm.Get(ft.Client, module)).Should(And(
		jq.Matchf(`.status.configValues."%s" != ""`, moduleconfig.KeyPlatformName),
		jq.Matchf(`.status.configValues."%s" != ""`, moduleconfig.KeyPlatformVersion),
	))
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
	workloadSvc := workloadService()

	g.Eventually(t.Context(), k8sm.Lookup(ft.Client, operatorCfg)).Should(Succeed())

	g.Eventually(t.Context(), k8sm.Get(ft.Client, workloadDeploy)).Should(And(
		k8sm.HasLabel(labels.PlatformPartOf, componentsv1alpha1.MyModuleComponentName),
		k8sm.HasAnnotation(annotations.InstanceName, module.GetName()),
		k8sm.HasAnnotation(annotations.InstanceUID, string(module.GetUID())),
		k8sm.HasAnnotation(annotations.PlatformType, operatorCfg.Data[moduleconfig.KeyPlatformName]),
		k8sm.HasAnnotation(annotations.PlatformVersion, module.Status.Release.Version.String()),
	))

	g.Eventually(t.Context(), k8sm.Get(ft.Client, workloadSvc)).Should(And(
		k8sm.HasLabel(labels.PlatformPartOf, componentsv1alpha1.MyModuleComponentName),
		k8sm.HasAnnotation(annotations.PlatformType, operatorCfg.Data[moduleconfig.KeyPlatformName]),
		k8sm.HasAnnotation(annotations.PlatformVersion, module.Status.Release.Version.String()),
	))
}

func (ft *foundationTests) testWebhookLabels(t *testing.T) {
	g := NewWithT(t)

	_ = ft.ensureReadyModule(t)
	workloadDeploy := workloadDeployment()

	g.Eventually(t.Context(), k8sm.Get(ft.Client, workloadDeploy)).Should(And(
		jq.Match(`.metadata.labels."mymodule.opendatahub.io/version" != ""`),
		jq.Match(`.metadata.labels."mymodule.opendatahub.io/platform" != ""`),
	))
}

func (ft *foundationTests) testOwnerReferences(t *testing.T) {
	g := NewWithT(t)

	owner := ft.ensureReadyModule(t)
	owner.TypeMeta = metav1.TypeMeta{
		APIVersion: componentsv1alpha1.GroupVersion.String(),
		Kind:       componentsv1alpha1.MyModuleKind,
	}
	workloadDeploy := workloadDeployment()
	workloadSvc := workloadService()

	g.Eventually(t.Context(), k8sm.Get(ft.Client, workloadDeploy)).Should(
		k8sm.HasOwnerReference(owner),
	)
	g.Eventually(t.Context(), k8sm.Get(ft.Client, workloadSvc)).Should(
		k8sm.HasOwnerReference(owner),
	)
}

func (ft *foundationTests) testUpgradeAnnotationAbsentOnFreshDeploy(t *testing.T) {
	g := NewWithT(t)

	_ = ft.ensureReadyModule(t)
	ingress := testIngress(support.OperatorNamespace())

	g.Eventually(t.Context(), k8sm.Get(ft.Client, ingress)).Should(And(
		jq.Matchf(`.metadata.annotations // {} | has("%s") | not`, mymodule.AnnotationManagedVersion),
		jq.Matchf(`.metadata.annotations // {} | has("%s") | not`, mymodule.AnnotationUpgradedFrom),
	))
}

func (ft *foundationTests) testUpgradeViaConfigMapRestart(t *testing.T) {
	g := NewWithT(t)

	_ = ft.ensureReadyModule(t)

	operatorCfgMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      modulemeta.OperatorConfigName,
			Namespace: support.OperatorNamespace(),
		},
	}
	g.Eventually(t.Context(), k8sm.Lookup(ft.Client, operatorCfgMap)).Should(Succeed())

	patch := client.MergeFrom(operatorCfgMap.DeepCopy())
	operatorCfgMap.Data[moduleconfig.KeyPlatformVersion] = "1.0.0"
	g.Expect(ft.Client.Patch(t.Context(), operatorCfgMap, patch)).To(Succeed())

	g.Expect(ft.Client.DeleteAllOf(t.Context(), &corev1.Pod{},
		client.InNamespace(support.OperatorNamespace()),
		client.MatchingLabels{"app.kubernetes.io/name": "opendatahub-mymodule-operator"},
	)).To(Succeed())

	operatorDeploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "opendatahub-mymodule-operator",
			Namespace: support.OperatorNamespace(),
		},
	}
	g.Eventually(t.Context(), k8sm.Get(ft.Client, operatorDeploy)).Should(
		jq.Match(`.status.readyReplicas >= 1`),
	)

	ingress := testIngress(support.OperatorNamespace())
	g.Eventually(t.Context(), k8sm.Get(ft.Client, ingress)).Should(And(
		jq.Matchf(`.metadata.annotations."%s" != ""`, mymodule.AnnotationManagedVersion),
		jq.Matchf(`.metadata.annotations."%s" != ""`, mymodule.AnnotationUpgradedFrom),
	))
}

func (ft *foundationTests) testUpgradeFaultInjection(t *testing.T) {
	g := NewWithT(t)

	module := ft.ensureReadyModule(t)
	platformVersionBefore := module.Status.Release.Version

	ingress := testIngress(support.OperatorNamespace())
	g.Eventually(t.Context(), k8sm.Lookup(ft.Client, ingress)).Should(Succeed())

	ingressPatch := client.MergeFrom(ingress.DeepCopy())
	if ingress.Annotations == nil {
		ingress.Annotations = map[string]string{}
	}
	ingress.Annotations[mymodule.AnnotationInjectUpgradeFault] = "true"
	g.Expect(ft.Client.Patch(t.Context(), ingress, ingressPatch)).To(Succeed())

	operatorCfgMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      modulemeta.OperatorConfigName,
			Namespace: support.OperatorNamespace(),
		},
	}
	g.Eventually(t.Context(), k8sm.Lookup(ft.Client, operatorCfgMap)).Should(Succeed())

	cfgPatch := client.MergeFrom(operatorCfgMap.DeepCopy())
	operatorCfgMap.Data[moduleconfig.KeyPlatformVersion] = "2.0.0"
	g.Expect(ft.Client.Patch(t.Context(), operatorCfgMap, cfgPatch)).To(Succeed())

	pods := &corev1.PodList{}
	g.Expect(ft.Client.List(t.Context(), pods,
		client.InNamespace(support.OperatorNamespace()),
		client.MatchingLabels{"app.kubernetes.io/name": "opendatahub-mymodule-operator"},
	)).To(Succeed())
	for i := range pods.Items {
		g.Expect(ft.Client.Delete(t.Context(), &pods.Items[i])).To(Succeed())
	}

	operatorDeploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "opendatahub-mymodule-operator",
			Namespace: support.OperatorNamespace(),
		},
	}
	g.Eventually(t.Context(), k8sm.Get(ft.Client, operatorDeploy)).Should(
		jq.Match(`.status.readyReplicas >= 1`),
	)

	g.Consistently(t.Context(), k8sm.Get(ft.Client, module)).WithTimeout(10 * time.Second).Should(
		jq.Matchf(`.status.release.version == "%s"`, platformVersionBefore),
	)

	g.Eventually(t.Context(), k8sm.Lookup(ft.Client, ingress)).Should(Succeed())

	cleanPatch := client.MergeFrom(ingress.DeepCopy())
	ann := ingress.GetAnnotations()
	delete(ann, mymodule.AnnotationInjectUpgradeFault)
	ingress.SetAnnotations(ann)
	g.Expect(ft.Client.Patch(t.Context(), ingress, cleanPatch)).To(Succeed())

	g.Eventually(t.Context(), k8sm.Get(ft.Client, module)).Should(
		jq.Match(`.status.release.version == "2.0.0"`),
	)
}
