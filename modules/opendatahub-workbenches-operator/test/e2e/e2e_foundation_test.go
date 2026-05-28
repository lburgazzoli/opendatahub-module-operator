//go:build e2e

package e2e

import (
	"testing"

	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/lburgazzoli/gomega-matchers/pkg/matchers/jq"
	"github.com/opendatahub-io/opendatahub-operator/v2/api/common"

	componentsv1alpha1 "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-workbenches-operator/api/components/v1alpha1"
	moduleconfig "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-workbenches-operator/pkg/config"
	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-workbenches-operator/pkg/version"
	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-workbenches-operator/test/support"
)

type foundationTests struct {
	*workbenchesE2ETest
	moduleCRD      *apiextensionsv1.CustomResourceDefinition
	operatorDeploy *appsv1.Deployment
	operatorCfgMap *corev1.ConfigMap
	nbcDeploy      *appsv1.Deployment
}

func newFoundationTests(suite *workbenchesE2ETest) *foundationTests {
	return &foundationTests{
		workbenchesE2ETest: suite,
		moduleCRD: &apiextensionsv1.CustomResourceDefinition{
			ObjectMeta: metav1.ObjectMeta{Name: moduleCRDName},
		},
		operatorDeploy: &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "opendatahub-workbenches-operator",
				Namespace: suite.operatorNamespace,
			},
		},
		operatorCfgMap: &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      operatorConfigMapName,
				Namespace: suite.operatorNamespace,
			},
		},
		nbcDeploy: &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "odh-notebook-controller-manager",
				Namespace: suite.operatorNamespace,
			},
		},
	}
}

func (ft *foundationTests) Execute(t *testing.T) {
	t.Run("should have module CRD installed", ft.testModuleCRDInstalled)
	t.Run("should have operator ConfigMap deployed", ft.testOperatorConfigMap)
	t.Run("should become ready", ft.testBecomesReady)
	t.Run("should report module version and platform", ft.testModuleStatus)
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
		jq.Match(`.data."%s" != ""`, moduleconfig.KeyPlatformType),
		jq.Match(`.data."%s" != ""`, moduleconfig.KeyPlatformVersion),
	))
}

func (ft *foundationTests) testBecomesReady(t *testing.T) {
	g := NewWithT(t)

	ensureSingletonCreated(t, ft.module)

	g.Eventually(func() bool {
		current := &componentsv1alpha1.Workbenches{
			ObjectMeta: metav1.ObjectMeta{Name: ft.module.GetName()},
		}
		if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(ft.module), current); err != nil {
			return false
		}

		return string(current.Status.Phase) == "Ready" &&
			hasConditionStatus(current.Status.Conditions, "Ready", "True") &&
			hasConditionStatus(current.Status.Conditions, "ProvisioningSucceeded", "True")
	}).WithContext(ctx).WithTimeout(timeout).WithPolling(interval).Should(BeTrue())

	eventuallyDeploymentReady(t, ft.nbcDeploy)
}

func (ft *foundationTests) testModuleStatus(t *testing.T) {
	g := NewWithT(t)
	operatorCfg := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      operatorConfigMapName,
			Namespace: support.OperatorNamespace(),
		},
	}
	workloadDeploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "odh-notebook-controller-manager",
			Namespace: support.OperatorNamespace(),
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
			Namespace: support.OperatorNamespace(),
		},
	}

	g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(module), module)).To(Succeed())
	g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(operatorCfg), operatorCfg)).To(Succeed())

	g.Eventually(k.Get(ft.nbcDeploy)).WithContext(ctx).WithTimeout(timeout).WithPolling(interval).Should(And(
		jq.Match(`.metadata.labels."%s" == "workbenches"`, labelPartOf),
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
}

func (ft *foundationTests) testOwnerReferences(t *testing.T) {
	g := NewWithT(t)

	g.Eventually(k.Get(ft.nbcDeploy)).WithContext(ctx).WithTimeout(timeout).WithPolling(interval).Should(
		jq.Match(`.metadata.ownerReferences[] | select(.kind == "Workbenches") | .name == "%s"`,
			componentsv1alpha1.WorkbenchesInstanceName),
	)
}

func hasConditionStatus(
	conditions []common.Condition,
	conditionType string,
	status string,
) bool {
	for _, condition := range conditions {
		if condition.Type == conditionType && string(condition.Status) == status {
			return true
		}
	}

	return false
}
