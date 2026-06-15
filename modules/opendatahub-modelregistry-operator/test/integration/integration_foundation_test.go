package integration

import (
	"testing"

	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/lburgazzoli/gomega-matchers/pkg/matchers/jq"
	k8sm "github.com/lburgazzoli/gomega-matchers/pkg/matchers/k8s"
	"github.com/lburgazzoli/gomega-matchers/pkg/matchers/k8s/condition"

	componentsv1alpha1 "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-modelregistry-operator/api/components/v1alpha1"
	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-modelregistry-operator/test/support"
	common "github.com/opendatahub-io/odh-platform-utilities/api/common"
	"github.com/opendatahub-io/odh-platform-utilities/pkg/metadata/annotations"
	"github.com/opendatahub-io/odh-platform-utilities/pkg/metadata/labels"
)

type foundationTests struct {
	Client client.Client
}

func (ft *foundationTests) Execute(t *testing.T) {
	t.Run("should have module CRD installed", ft.testModuleCRDInstalled)
	t.Run("should become ready", ft.testBecomesReady)
	t.Run("should report release version and platform", ft.testReleaseStatus)
	t.Run("should set platform labels and annotations", ft.testPlatformLabels)
	t.Run("should set owner references", ft.testOwnerReferences)
	t.Run("should set registries namespace in status", ft.testRegistriesNamespaceStatus)
}

func (ft *foundationTests) ensureReadyModule(t *testing.T) *componentsv1alpha1.ModelRegistry {
	t.Helper()

	g := NewWithT(t)
	module := &componentsv1alpha1.ModelRegistry{
		ObjectMeta: metav1.ObjectMeta{
			Name: componentsv1alpha1.ModelRegistryInstanceName,
		},
		Spec: componentsv1alpha1.ModelRegistrySpec{
			Gateway: &componentsv1alpha1.GatewaySpec{
				Domain: testGatewayDomain,
			},
		},
	}
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

	g.Eventually(t.Context(), support.WrapGetEventually(
		t,
		"ensureReadyModule/modelregistry",
		k8sm.Get(ft.Client, module),
	)).Should(
		WithTransform(k8sm.ConditionsOf[metav1.Condition](), SatisfyAll(
			ContainElement(condition.Is(string(common.ConditionTypeReady), metav1.ConditionTrue)),
			ContainElement(condition.Is(string(common.ConditionTypeProvisioningSucceeded), metav1.ConditionTrue)),
		)),
	)

	g.Eventually(t.Context(), ft.wrapDeploymentDebugEventually(
		t,
		"ensureReadyModule/deployment",
		k8sm.Get(ft.Client, workloadDeploy),
	)).Should(
		jq.Match(`.status.readyReplicas >= 1`),
	)

	return module
}

func (ft *foundationTests) testModuleCRDInstalled(t *testing.T) {
	g := NewWithT(t)

	moduleCRD := &apiextensionsv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: componentsv1alpha1.ModelRegistryCRDName},
	}
	g.Eventually(t.Context(), k8sm.Lookup(ft.Client, moduleCRD)).Should(Succeed())
}

func (ft *foundationTests) testBecomesReady(t *testing.T) {
	ft.ensureReadyModule(t)
}

func (ft *foundationTests) testReleaseStatus(t *testing.T) {
	g := NewWithT(t)

	module := ft.ensureReadyModule(t)
	cfg, err := loadOperatorConfig()
	g.Expect(err).NotTo(HaveOccurred())

	g.Eventually(t.Context(), support.WrapGetEventually(
		t,
		"testReleaseStatus/modelregistry",
		k8sm.Get(ft.Client, module),
	)).Should(And(
		jq.Matchf(`.status.release.version == "%s"`, cfg.Release().Version.String()),
		jq.Matchf(`.status.release.name == "%s"`,
			cfg.PlatformName),
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
	cfg, err := loadOperatorConfig()
	g.Expect(err).NotTo(HaveOccurred())

	g.Eventually(t.Context(), ft.wrapDeploymentDebugEventually(
		t,
		"testPlatformLabels/deployment",
		k8sm.Get(ft.Client, workloadDeploy),
	)).Should(And(
		k8sm.HasLabel(labels.PlatformPartOf, componentsv1alpha1.ModelRegistryComponentName),
		k8sm.HasAnnotation(annotations.InstanceName, module.GetName()),
		k8sm.HasAnnotation(annotations.InstanceUID, string(module.GetUID())),
		k8sm.HasAnnotation(annotations.PlatformType, cfg.PlatformName),
		k8sm.HasAnnotation(annotations.PlatformVersion, cfg.Release().Version.String()),
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
		Kind:       componentsv1alpha1.ModelRegistryKind,
	}

	g.Eventually(t.Context(), ft.wrapDeploymentDebugEventually(
		t,
		"testOwnerReferences/deployment",
		k8sm.Get(ft.Client, workloadDeploy),
	)).Should(
		k8sm.HasOwnerReference(owner),
	)
}

func (ft *foundationTests) testRegistriesNamespaceStatus(t *testing.T) {
	g := NewWithT(t)

	module := ft.ensureReadyModule(t)

	g.Eventually(t.Context(), support.WrapGetEventually(
		t,
		"testRegistriesNamespaceStatus/modelregistry",
		k8sm.Get(ft.Client, module),
	)).Should(
		jq.Match(`.status.registriesNamespace != ""`),
	)
}

func (ft *foundationTests) wrapDeploymentDebugEventually(
	t *testing.T,
	label string,
	poll support.ContextPollFunc[*appsv1.Deployment],
) support.ContextPollFunc[*appsv1.Deployment] {
	t.Helper()

	return support.WrapEventually(t, label, poll, func(deployment *appsv1.Deployment) any {
		return ft.snapshotDeploymentWithPods(t, deployment)
	})
}

func (ft *foundationTests) snapshotDeploymentWithPods(t *testing.T, deployment *appsv1.Deployment) any {
	t.Helper()

	if deployment == nil {
		return nil
	}

	snapshot := map[string]any{
		"name":      deployment.GetName(),
		"namespace": deployment.GetNamespace(),
		"status":    deployment.Status,
	}

	if deployment.Spec.Selector == nil {
		snapshot["podsError"] = "deployment selector is nil"
		return snapshot
	}

	selector, err := metav1.LabelSelectorAsSelector(deployment.Spec.Selector)
	if err != nil {
		snapshot["podsError"] = err.Error()
		return snapshot
	}

	podList := &corev1.PodList{}
	if err := ft.Client.List(
		t.Context(),
		podList,
		client.InNamespace(deployment.GetNamespace()),
		client.MatchingLabelsSelector{Selector: selector},
	); err != nil {
		snapshot["podsError"] = err.Error()
		return snapshot
	}

	pods := make([]any, 0, len(podList.Items))
	for i := range podList.Items {
		pods = append(pods, snapshotPod(&podList.Items[i]))
	}

	snapshot["pods"] = pods

	return snapshot
}

func snapshotPod(pod *corev1.Pod) any {
	if pod == nil {
		return nil
	}

	return map[string]any{
		"name":                  pod.GetName(),
		"phase":                 pod.Status.Phase,
		"podIP":                 pod.Status.PodIP,
		"deletionTimestamp":     pod.GetDeletionTimestamp(),
		"conditions":            pod.Status.Conditions,
		"initContainerStatuses": snapshotContainerStatuses(pod.Status.InitContainerStatuses),
		"containerStatuses":     snapshotContainerStatuses(pod.Status.ContainerStatuses),
	}
}

func snapshotContainerStatuses(statuses []corev1.ContainerStatus) []any {
	snapshots := make([]any, 0, len(statuses))
	for _, status := range statuses {
		snapshots = append(snapshots, map[string]any{
			"name":         status.Name,
			"ready":        status.Ready,
			"restartCount": status.RestartCount,
			"started":      status.Started,
			"state":        status.State,
			"lastState":    status.LastTerminationState,
		})
	}

	return snapshots
}
