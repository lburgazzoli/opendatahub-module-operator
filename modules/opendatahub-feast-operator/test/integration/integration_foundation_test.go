package integration

import (
	"bytes"
	"fmt"
	"os"
	"testing"

	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/lburgazzoli/gomega-matchers/pkg/matchers/jq"
	k8sm "github.com/lburgazzoli/gomega-matchers/pkg/matchers/k8s"
	"github.com/lburgazzoli/gomega-matchers/pkg/matchers/k8s/condition"

	componentsv1alpha1 "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-feast-operator/api/components/v1alpha1"
	moduleconfig "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-feast-operator/pkg/config"
	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-feast-operator/test/support"
	common "github.com/opendatahub-io/odh-platform-utilities/api/common"
	kparams "github.com/opendatahub-io/odh-platform-utilities/framework/render/kustomize/params"
	"github.com/opendatahub-io/odh-platform-utilities/pkg/metadata/annotations"
	"github.com/opendatahub-io/odh-platform-utilities/pkg/metadata/labels"
)

type foundationTests struct {
	Client client.Client
}

func (ft *foundationTests) Execute(t *testing.T) {
	t.Run("should have module CRD installed", ft.testModuleCRDInstalled)
	t.Run("should become ready", ft.testBecomesReady)
	t.Run("should keep source params env unchanged while injecting runtime values", ft.testRuntimeParamsWithoutMutatingSource)
	t.Run("should report release version and platform", ft.testReleaseStatus)
	t.Run("should set platform labels and annotations", ft.testPlatformLabels)
	t.Run("should set owner references", ft.testOwnerReferences)
}

func (ft *foundationTests) ensureReadyModule(t *testing.T) *componentsv1alpha1.FeastOperator {
	return ft.ensureReadyModuleFor(t, &componentsv1alpha1.FeastOperator{
		ObjectMeta: metav1.ObjectMeta{
			Name: componentsv1alpha1.FeastOperatorInstanceName,
		},
	})
}

func (ft *foundationTests) ensureReadyModuleFor(
	t *testing.T,
	module *componentsv1alpha1.FeastOperator,
) *componentsv1alpha1.FeastOperator {
	t.Helper()

	g := NewWithT(t)
	workloadDeploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      support.ManagedDeploymentName,
			Namespace: support.IntegrationTestNamespace(),
		},
	}

	_ = ft.Client.Delete(t.Context(), module)
	g.Eventually(t.Context(), k8sm.NotFound(ft.Client, module)).Should(BeTrue())

	t.Cleanup(func() {
		_ = ft.Client.Delete(t.Context(), module)
	})

	g.Expect(ft.Client.Create(t.Context(), module)).To(Succeed())

	g.Eventually(t.Context(), k8sm.Get(ft.Client, module)).Should(
		WithTransform(k8sm.ConditionsOf[metav1.Condition](), SatisfyAll(
			ContainElement(condition.Is(string(common.ConditionTypeReady), metav1.ConditionTrue)),
			ContainElement(condition.Is(string(common.ConditionTypeProvisioningSucceeded), metav1.ConditionTrue)),
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
		ObjectMeta: metav1.ObjectMeta{Name: componentsv1alpha1.FeastOperatorCRDName},
	}
	g.Eventually(t.Context(), k8sm.Lookup(ft.Client, moduleCRD)).Should(Succeed())
}

func (ft *foundationTests) testBecomesReady(t *testing.T) {
	ft.ensureReadyModule(t)
}

func (ft *foundationTests) testRuntimeParamsWithoutMutatingSource(t *testing.T) {
	g := NewWithT(t)

	paramsPath := support.MustProjectFile(
		"config", "manifests", "feastoperator", "overlays", "odh", "params.env",
	)
	paramsBefore, err := os.ReadFile(paramsPath)
	g.Expect(err).NotTo(HaveOccurred())

	module := &componentsv1alpha1.FeastOperator{
		ObjectMeta: metav1.ObjectMeta{
			Name: componentsv1alpha1.FeastOperatorInstanceName,
		},
		Spec: componentsv1alpha1.FeastOperatorSpec{
			OIDC: &componentsv1alpha1.GatewayOIDCSpec{
				IssuerURL: "https://issuer.example.com",
			},
		},
	}
	ft.ensureReadyModuleFor(t, module)

	paramsAfter, err := os.ReadFile(paramsPath)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(bytes.Equal(paramsBefore, paramsAfter)).To(BeTrue())

	paramsEntries, err := kparams.Unmarshal(paramsAfter)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(paramsEntries).To(HaveKeyWithValue("OIDC_ISSUER_URL", ""))

	workloadDeploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      support.ManagedDeploymentName,
			Namespace: support.IntegrationTestNamespace(),
		},
	}

	g.Eventually(t.Context(), k8sm.Get(ft.Client, workloadDeploy)).Should(
		WithTransform(
			func(deploy *appsv1.Deployment) map[string]string {
				return deploymentContainerEnvMap(deploy, "manager")
			},
			HaveKeyWithValue("OIDC_ISSUER_URL", "https://issuer.example.com"),
		),
	)
}

func deploymentContainerEnvMap(
	deploy *appsv1.Deployment,
	containerName string,
) map[string]string {
	for _, container := range deploy.Spec.Template.Spec.Containers {
		if container.Name != containerName {
			continue
		}

		out := make(map[string]string, len(container.Env))
		for _, envVar := range container.Env {
			out[envVar.Name] = envVar.Value
		}

		return out
	}

	return nil
}

func (ft *foundationTests) testReleaseStatus(t *testing.T) {
	g := NewWithT(t)

	module := ft.ensureReadyModule(t)
	cfg, err := loadOperatorConfig()
	g.Expect(err).NotTo(HaveOccurred())

	expr := fmt.Sprintf(`.status.releases[] | select(.name == "%s") | .version == "%s"`,
		moduleconfig.ReleasePlatform, cfg.ComponentRelease().Version)
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
		k8sm.HasLabel(labels.PlatformPartOf, componentsv1alpha1.FeastOperatorComponentName),
		k8sm.HasAnnotation(annotations.InstanceName, module.GetName()),
		k8sm.HasAnnotation(annotations.InstanceUID, string(module.GetUID())),
		k8sm.HasAnnotation(annotations.PlatformVersion, cfg.ComponentRelease().Version),
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
		Kind:       componentsv1alpha1.FeastOperatorKind,
	}

	g.Eventually(t.Context(), k8sm.Get(ft.Client, workloadDeploy)).Should(
		k8sm.HasOwnerReference(owner),
	)
}
