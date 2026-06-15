package integration

import (
	"testing"
	"time"

	"github.com/blang/semver/v4"
	. "github.com/onsi/gomega"
	ofVersion "github.com/operator-framework/api/pkg/lib/version"

	"github.com/lburgazzoli/gomega-matchers/pkg/matchers/jq"
	k8sm "github.com/lburgazzoli/gomega-matchers/pkg/matchers/k8s"
	"github.com/lburgazzoli/gomega-matchers/pkg/matchers/k8s/condition"

	componentsv1alpha1 "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-mymodule-operator/api/components/v1alpha1"
	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-mymodule-operator/internal/controller/mymodule"
	moduleconfig "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-mymodule-operator/pkg/config"
	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-mymodule-operator/test/support"
	common "github.com/opendatahub-io/odh-platform-utilities/api/common"
	"github.com/opendatahub-io/odh-platform-utilities/framework/resources"
	"github.com/opendatahub-io/odh-platform-utilities/pkg/metadata/annotations"
	"github.com/opendatahub-io/odh-platform-utilities/pkg/metadata/labels"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type foundationTests struct {
	Client client.Client
}

func (ft *foundationTests) Execute(t *testing.T) {
	t.Run("should have module CRD installed", ft.testModuleCRDInstalled)
	t.Run("should block when Ingress is missing", ft.testIngressBlocks)
	t.Run("should recover when Ingress is created", ft.testIngressRecovers)
	t.Run("should expose config values", ft.testConfigValues)
	t.Run("should report release version and platform", ft.testReleaseStatus)
	t.Run("should set platform labels and annotations", ft.testPlatformLabels)
	t.Run("should set owner references", ft.testOwnerReferences)
	t.Run("should not annotate ingress on fresh install", ft.testUpgradeAnnotationAbsentOnFreshInstall)
	t.Run("should annotate ingress on upgrade", ft.testUpgradeAnnotatesIngress)
	t.Run("should not update version on upgrade fault", ft.testUpgradeFaultInjection)
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

func workloadService() *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "mymodule-workload",
			Namespace: support.IntegrationTestNamespace(),
		},
	}
}

func (ft *foundationTests) cleanupModuleAndIngress(t *testing.T) {
	t.Helper()

	g := NewWithT(t)
	module := &componentsv1alpha1.MyModule{
		ObjectMeta: metav1.ObjectMeta{
			Name: componentsv1alpha1.MyModuleInstanceName,
		},
	}
	ingress := testIngress(support.IntegrationTestNamespace())

	_ = ft.Client.Delete(t.Context(), module)
	_ = ft.Client.Delete(t.Context(), ingress)
	g.Eventually(t.Context(), k8sm.NotFound(ft.Client, module)).Should(BeTrue())
	g.Eventually(t.Context(), k8sm.NotFound(ft.Client, ingress)).Should(BeTrue())
}

func (ft *foundationTests) ensureReadyModule(t *testing.T) *componentsv1alpha1.MyModule {
	t.Helper()

	g := NewWithT(t)
	module := &componentsv1alpha1.MyModule{
		ObjectMeta: metav1.ObjectMeta{
			Name: componentsv1alpha1.MyModuleInstanceName,
		},
	}
	ingress := testIngress(support.IntegrationTestNamespace())
	workloadDeploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      support.ManagedDeploymentName,
			Namespace: support.IntegrationTestNamespace(),
		},
	}

	ft.cleanupModuleAndIngress(t)

	t.Cleanup(func() {
		_ = ft.Client.Delete(t.Context(), module)
		_ = ft.Client.Delete(t.Context(), ingress)
	})

	g.Expect(ft.Client.Create(t.Context(), ingress)).To(Succeed())
	g.Expect(ft.Client.Create(t.Context(), module)).To(Succeed())

	g.Eventually(t.Context(), k8sm.Get(ft.Client, module)).Should(
		WithTransform(k8sm.ConditionsOf[metav1.Condition](), SatisfyAll(
			ContainElement(condition.Is(common.ConditionTypeReady, metav1.ConditionTrue)),
			ContainElement(condition.Is(mymodule.ConditionIngressAvailable, metav1.ConditionTrue)),
			ContainElement(condition.Is(common.ConditionTypeProvisioningSucceeded, metav1.ConditionTrue)),
		)),
	)

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

func (ft *foundationTests) testIngressBlocks(t *testing.T) {
	g := NewWithT(t)
	module := &componentsv1alpha1.MyModule{
		ObjectMeta: metav1.ObjectMeta{
			Name: componentsv1alpha1.MyModuleInstanceName,
		},
	}

	ft.cleanupModuleAndIngress(t)

	t.Cleanup(func() {
		_ = ft.Client.Delete(t.Context(), module)
	})

	g.Expect(ft.Client.Create(t.Context(), module)).To(Succeed())

	g.Eventually(t.Context(), k8sm.Get(ft.Client, module)).Should(
		WithTransform(k8sm.ConditionsOf[metav1.Condition](), SatisfyAll(
			ContainElement(condition.Is(mymodule.ConditionIngressAvailable, metav1.ConditionFalse)),
		)),
	)
}

func (ft *foundationTests) testIngressRecovers(t *testing.T) {
	g := NewWithT(t)
	module := &componentsv1alpha1.MyModule{
		ObjectMeta: metav1.ObjectMeta{
			Name: componentsv1alpha1.MyModuleInstanceName,
		},
	}
	ingress := testIngress(support.IntegrationTestNamespace())
	workloadDeploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      support.ManagedDeploymentName,
			Namespace: support.IntegrationTestNamespace(),
		},
	}

	ft.cleanupModuleAndIngress(t)

	t.Cleanup(func() {
		_ = ft.Client.Delete(t.Context(), module)
		_ = ft.Client.Delete(t.Context(), ingress)
	})

	g.Expect(ft.Client.Create(t.Context(), module)).To(Succeed())
	g.Expect(ft.Client.Create(t.Context(), ingress)).To(Succeed())

	g.Eventually(t.Context(), k8sm.Get(ft.Client, module)).Should(
		WithTransform(k8sm.ConditionsOf[metav1.Condition](), SatisfyAll(
			ContainElement(condition.Is(common.ConditionTypeReady, metav1.ConditionTrue)),
			ContainElement(condition.Is(mymodule.ConditionIngressAvailable, metav1.ConditionTrue)),
			ContainElement(condition.Is(common.ConditionTypeProvisioningSucceeded, metav1.ConditionTrue)),
		)),
	)

	g.Eventually(t.Context(), k8sm.Get(ft.Client, workloadDeploy)).Should(
		jq.Match(`.status.readyReplicas >= 1`),
	)
}

func (ft *foundationTests) testConfigValues(t *testing.T) {
	g := NewWithT(t)

	module := ft.ensureReadyModule(t)
	cfg, err := loadOperatorConfig()
	g.Expect(err).NotTo(HaveOccurred())

	g.Eventually(t.Context(), k8sm.Get(ft.Client, module)).Should(And(
		jq.Matchf(`.status.configValues."%s" == "%s"`,
			moduleconfig.KeyPlatformName,
			cfg.PlatformName),
		jq.Matchf(`.status.configValues."%s" == "%s"`,
			moduleconfig.KeyPlatformVersion,
			cfg.PlatformVersion),
	))
}

func (ft *foundationTests) testReleaseStatus(t *testing.T) {
	g := NewWithT(t)

	module := ft.ensureReadyModule(t)
	cfg, err := loadOperatorConfig()
	g.Expect(err).NotTo(HaveOccurred())

	g.Eventually(t.Context(), k8sm.Get(ft.Client, module)).Should(And(
		jq.Matchf(`.status.release.version == "%s"`, cfg.Release().Version.String()),
		jq.Matchf(`.status.release.name == "%s"`, cfg.PlatformName),
	))
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
	workloadSvc := workloadService()
	cfg, err := loadOperatorConfig()
	g.Expect(err).NotTo(HaveOccurred())

	g.Eventually(t.Context(), k8sm.Get(ft.Client, workloadDeploy)).Should(And(
		k8sm.HasLabel(labels.PlatformPartOf, componentsv1alpha1.MyModuleComponentName),
		k8sm.HasAnnotation(annotations.InstanceName, module.GetName()),
		k8sm.HasAnnotation(annotations.InstanceUID, string(module.GetUID())),
		k8sm.HasAnnotation(annotations.PlatformType, cfg.PlatformName),
		k8sm.HasAnnotation(annotations.PlatformVersion, cfg.Release().Version.String()),
	))

	g.Eventually(t.Context(), k8sm.Get(ft.Client, workloadSvc)).Should(And(
		k8sm.HasLabel(labels.PlatformPartOf, componentsv1alpha1.MyModuleComponentName),
		k8sm.HasAnnotation(annotations.PlatformType, cfg.PlatformName),
		k8sm.HasAnnotation(annotations.PlatformVersion, cfg.Release().Version.String()),
	))
}

func (ft *foundationTests) testOwnerReferences(t *testing.T) {
	g := NewWithT(t)

	owner := ft.ensureReadyModule(t)
	owner.TypeMeta = metav1.TypeMeta{
		APIVersion: componentsv1alpha1.GroupVersion.String(),
		Kind:       componentsv1alpha1.MyModuleKind,
	}
	workloadDeploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      support.ManagedDeploymentName,
			Namespace: support.IntegrationTestNamespace(),
		},
	}
	workloadSvc := workloadService()

	g.Eventually(t.Context(), k8sm.Get(ft.Client, workloadDeploy)).Should(
		k8sm.HasOwnerReference(owner),
	)
	g.Eventually(t.Context(), k8sm.Get(ft.Client, workloadSvc)).Should(
		k8sm.HasOwnerReference(owner),
	)
}

func (ft *foundationTests) testUpgradeAnnotationAbsentOnFreshInstall(t *testing.T) {
	g := NewWithT(t)

	_ = ft.ensureReadyModule(t)
	ingress := testIngress(support.IntegrationTestNamespace())

	g.Eventually(t.Context(), k8sm.Get(ft.Client, ingress)).Should(And(
		jq.Matchf(`.metadata.annotations // {} | has("%s") | not`, mymodule.AnnotationManagedVersion),
		jq.Matchf(`.metadata.annotations // {} | has("%s") | not`, mymodule.AnnotationUpgradedFrom),
	))
}

func (ft *foundationTests) testUpgradeAnnotatesIngress(t *testing.T) {
	g := NewWithT(t)

	module := ft.ensureReadyModule(t)

	patch := client.MergeFrom(module.DeepCopy())
	module.Status.Release.Version = ofVersion.OperatorVersion{Version: semver.MustParse("0.0.0-0")}
	g.Expect(ft.Client.Status().Patch(t.Context(), module, patch)).To(Succeed())

	g.Eventually(t.Context(), k8sm.Lookup(ft.Client, module)).Should(Succeed())

	annPatch := client.MergeFrom(module.DeepCopy())
	resources.SetAnnotation(module, "test-trigger", time.Now().String())
	g.Expect(ft.Client.Patch(t.Context(), module, annPatch)).To(Succeed())

	ingress := testIngress(support.IntegrationTestNamespace())
	g.Eventually(t.Context(), k8sm.Get(ft.Client, ingress)).Should(And(
		jq.Matchf(`.metadata.annotations."%s" != ""`, mymodule.AnnotationManagedVersion),
		jq.Matchf(`.metadata.annotations."%s" == "0.0.0-0"`, mymodule.AnnotationUpgradedFrom),
	))
}

func (ft *foundationTests) testUpgradeFaultInjection(t *testing.T) {
	g := NewWithT(t)

	module := ft.ensureReadyModule(t)
	versionBefore := module.Status.Release.Version.String()

	statusPatch := client.MergeFrom(module.DeepCopy())
	module.Status.Release.Version = ofVersion.OperatorVersion{Version: semver.MustParse("0.0.0-0")}
	g.Expect(ft.Client.Status().Patch(t.Context(), module, statusPatch)).To(Succeed())

	ingress := testIngress(support.IntegrationTestNamespace())
	g.Eventually(t.Context(), k8sm.Lookup(ft.Client, ingress)).Should(Succeed())

	ingressPatch := client.MergeFrom(ingress.DeepCopy())
	resources.SetAnnotation(ingress, mymodule.AnnotationInjectUpgradeFault, "true")
	g.Expect(ft.Client.Patch(t.Context(), ingress, ingressPatch)).To(Succeed())

	g.Consistently(t.Context(), k8sm.Get(ft.Client, module)).WithTimeout(10 * time.Second).Should(
		jq.Match(`.status.release.version == "0.0.0-0"`),
	)

	g.Eventually(t.Context(), k8sm.Lookup(ft.Client, ingress)).Should(Succeed())

	cleanPatch := client.MergeFrom(ingress.DeepCopy())
	ann := ingress.GetAnnotations()
	delete(ann, mymodule.AnnotationInjectUpgradeFault)
	ingress.SetAnnotations(ann)
	g.Expect(ft.Client.Patch(t.Context(), ingress, cleanPatch)).To(Succeed())

	g.Eventually(t.Context(), k8sm.Get(ft.Client, module)).Should(
		jq.Matchf(`.status.release.version == "%s"`, versionBefore),
	)
}
