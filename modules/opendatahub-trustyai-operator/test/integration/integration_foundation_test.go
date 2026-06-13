package integration

import (
	"testing"

	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/lburgazzoli/gomega-matchers/pkg/matchers/jq"
	k8sm "github.com/lburgazzoli/gomega-matchers/pkg/matchers/k8s"

	componentsv1alpha1 "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-trustyai-operator/api/components/v1alpha1"
	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-trustyai-operator/test/support"
	"github.com/opendatahub-io/odh-platform-utilities/pkg/metadata/annotations"
	"github.com/opendatahub-io/odh-platform-utilities/pkg/metadata/labels"
)

type foundationTests struct {
	Client client.Client
}

func (ft *foundationTests) Execute(t *testing.T) {
	t.Run("should have module CRD installed", ft.testModuleCRDInstalled)
	t.Run("should report not ready when precondition CR is missing", ft.testPreconditionCRMissing)
	t.Run("should become ready", ft.testBecomesReady)
	t.Run("should report release version and platform", ft.testReleaseStatus)
	t.Run("should set platform labels and annotations", ft.testPlatformLabels)
	t.Run("should set owner references", ft.testOwnerReferences)
}

func moduleObject() *componentsv1alpha1.TrustyAI {
	return &componentsv1alpha1.TrustyAI{
		ObjectMeta: metav1.ObjectMeta{
			Name: componentsv1alpha1.TrustyAIInstanceName,
		},
	}
}

func workloadDeployment() *appsv1.Deployment {
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "trustyai-service-operator-controller-manager",
			Namespace: support.IntegrationTestNamespace(),
		},
	}
}

func (ft *foundationTests) ensureReadyModule(t *testing.T) *componentsv1alpha1.TrustyAI {
	t.Helper()

	g := NewWithT(t)
	module := moduleObject()
	workloadDeploy := workloadDeployment()

	_ = ft.Client.Delete(t.Context(), module)
	g.Eventually(t.Context(), k8sm.NotFound(ft.Client, module)).Should(BeTrue())

	t.Cleanup(func() {
		_ = ft.Client.Delete(t.Context(), module)
	})

	g.Expect(ft.Client.Create(t.Context(), module)).To(Succeed())

	g.Eventually(t.Context(), k8sm.Get(ft.Client, module)).Should(And(
		jq.Match(`.status.phase == "Ready"`),
		WithTransform(k8sm.Conditions(), SatisfyAll(
			ContainElement(SatisfyAll(
				HaveKeyWithValue("type", "Ready"),
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
		ObjectMeta: metav1.ObjectMeta{Name: componentsv1alpha1.TrustyAICRDName},
	}
	g.Eventually(t.Context(), k8sm.Lookup(ft.Client, moduleCRD)).Should(Succeed())
}

func (ft *foundationTests) testPreconditionCRMissing(t *testing.T) {
	g := NewWithT(t)
	module := moduleObject()

	kserveCR := support.NewStubKserveCR()
	g.Expect(ft.Client.Delete(t.Context(), kserveCR)).To(Succeed())
	g.Eventually(t.Context(), k8sm.NotFound(ft.Client, kserveCR)).Should(BeTrue())
	t.Cleanup(func() {
		_, _ = support.EnsureStubKserveCRIfMissing(t.Context(), ft.Client)
		_ = ft.Client.Delete(t.Context(), module)
		g := NewWithT(t)
		g.Eventually(t.Context(), k8sm.NotFound(ft.Client, module)).Should(BeTrue())
	})

	g.Expect(ft.Client.Create(t.Context(), module)).To(Succeed())

	g.Eventually(t.Context(), k8sm.Get(ft.Client, module)).Should(
		jq.Match(`(.status.conditions // []) | any(.[]; .type == "ProvisioningSucceeded" and .status == "False")`),
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

	g.Eventually(t.Context(), k8sm.Get(ft.Client, module)).Should(And(
		jq.Matchf(`.status.release.version == "%s"`, cfg.Release().Version.String()),
		jq.Matchf(`.status.release.name == "%s"`, cfg.PlatformName),
	))
}

func (ft *foundationTests) testPlatformLabels(t *testing.T) {
	g := NewWithT(t)

	module := ft.ensureReadyModule(t)
	workloadDeploy := workloadDeployment()
	cfg, err := loadOperatorConfig()
	g.Expect(err).NotTo(HaveOccurred())

	g.Eventually(t.Context(), k8sm.Get(ft.Client, workloadDeploy)).Should(And(
		k8sm.HasLabel(labels.PlatformPartOf, componentsv1alpha1.TrustyAIComponentName),
		k8sm.HasAnnotation(annotations.InstanceName, module.GetName()),
		k8sm.HasAnnotation(annotations.InstanceUID, string(module.GetUID())),
		k8sm.HasAnnotation(annotations.PlatformType, cfg.PlatformName),
		k8sm.HasAnnotation(annotations.PlatformVersion, cfg.Release().Version.String()),
	))
}

func (ft *foundationTests) testOwnerReferences(t *testing.T) {
	g := NewWithT(t)

	owner := ft.ensureReadyModule(t)
	workloadDeploy := workloadDeployment()
	owner.TypeMeta = metav1.TypeMeta{
		APIVersion: componentsv1alpha1.GroupVersion.String(),
		Kind:       componentsv1alpha1.TrustyAIKind,
	}

	g.Eventually(t.Context(), k8sm.Get(ft.Client, workloadDeploy)).Should(
		k8sm.HasOwnerReference(owner),
	)
}
