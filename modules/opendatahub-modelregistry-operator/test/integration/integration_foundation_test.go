package integration

import (
	"encoding/json"
	"fmt"
	"testing"

	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	klabels "k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/lburgazzoli/gomega-matchers/pkg/matchers/jq"
	k8sm "github.com/lburgazzoli/gomega-matchers/pkg/matchers/k8s"
	"github.com/lburgazzoli/gomega-matchers/pkg/matchers/k8s/condition"

	componentsv1alpha1 "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-modelregistry-operator/api/components/v1alpha1"
	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-modelregistry-operator/test/support"
	common "github.com/opendatahub-io/odh-platform-utilities/api/common"
	"github.com/opendatahub-io/odh-platform-utilities/pkg/metadata/annotations"
	mdlabels "github.com/opendatahub-io/odh-platform-utilities/pkg/metadata/labels"
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
	t.Cleanup(func() {
		if !t.Failed() {
			return
		}

		ft.dumpEnsureReadyModuleResources(t, module, workloadDeploy)
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
		k8sm.HasLabel(mdlabels.PlatformPartOf, componentsv1alpha1.ModelRegistryComponentName),
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

func (ft *foundationTests) dumpEnsureReadyModuleResources(
	t *testing.T,
	module *componentsv1alpha1.ModelRegistry,
	deployment *appsv1.Deployment,
) {
	t.Helper()

	t.Logf(
		"[FailureDump] ensureReadyModule resources value=%s",
		stringifyFailureDumpValue(ft.snapshotEnsureReadyModuleResources(t, module, deployment)),
	)
}

func (ft *foundationTests) snapshotEnsureReadyModuleResources(
	t *testing.T,
	module *componentsv1alpha1.ModelRegistry,
	deployment *appsv1.Deployment,
) any {
	t.Helper()

	snapshot := map[string]any{}

	moduleSnapshot, moduleErr := ft.lookupObjectSnapshot(t, module)
	snapshot["module"] = moduleSnapshot
	if moduleErr != nil {
		snapshot["moduleError"] = moduleErr.Error()
	}

	deploymentSnapshot, deploymentErr := ft.lookupObjectSnapshot(t, deployment)
	snapshot["deployment"] = deploymentSnapshot
	if deploymentErr != nil {
		snapshot["deploymentError"] = deploymentErr.Error()
	}
	snapshot["namespaceResources"] = ft.snapshotNamespaceResources(t, support.IntegrationTestNamespace())

	return snapshot
}

func (ft *foundationTests) lookupObjectSnapshot(t *testing.T, object client.Object) (any, error) {
	t.Helper()

	if object == nil {
		return nil, nil
	}

	key := client.ObjectKeyFromObject(object)
	if err := ft.Client.Get(t.Context(), key, object); err != nil {
		return map[string]any{
			"name":      key.Name,
			"namespace": key.Namespace,
		}, err
	}

	return support.SnapshotObject(object), nil
}

func selectorForDeployment(deployment *appsv1.Deployment) (klabels.Selector, error) {
	if deployment == nil || deployment.Spec.Selector == nil {
		return nil, nil
	}

	selector, err := metav1.LabelSelectorAsSelector(deployment.Spec.Selector)
	if err != nil {
		return nil, err
	}

	return selector, nil
}

func (ft *foundationTests) snapshotNamespaceResources(t *testing.T, namespace string) any {
	t.Helper()

	return map[string]any{
		"deployments":     ft.mustListObjectSnapshots(t, namespace, &appsv1.DeploymentList{}),
		"replicaSets":     ft.mustListObjectSnapshots(t, namespace, &appsv1.ReplicaSetList{}),
		"pods":            ft.mustListPodSnapshots(t, namespace),
		"services":        ft.mustListObjectSnapshots(t, namespace, &corev1.ServiceList{}),
		"serviceAccounts": ft.mustListObjectSnapshots(t, namespace, &corev1.ServiceAccountList{}),
		"configMaps":      ft.mustListObjectSnapshots(t, namespace, &corev1.ConfigMapList{}),
	}
}

func (ft *foundationTests) mustListObjectSnapshots(t *testing.T, namespace string, list client.ObjectList) any {
	t.Helper()

	if err := ft.Client.List(t.Context(), list, client.InNamespace(namespace)); err != nil {
		return map[string]any{"error": err.Error()}
	}

	switch typed := list.(type) {
	case *appsv1.DeploymentList:
		items := make([]any, 0, len(typed.Items))
		for i := range typed.Items {
			items = append(items, support.SnapshotObject(&typed.Items[i]))
		}
		return items
	case *appsv1.ReplicaSetList:
		items := make([]any, 0, len(typed.Items))
		for i := range typed.Items {
			items = append(items, support.SnapshotObject(&typed.Items[i]))
		}
		return items
	case *corev1.ServiceList:
		items := make([]any, 0, len(typed.Items))
		for i := range typed.Items {
			items = append(items, support.SnapshotObject(&typed.Items[i]))
		}
		return items
	case *corev1.ServiceAccountList:
		items := make([]any, 0, len(typed.Items))
		for i := range typed.Items {
			items = append(items, support.SnapshotObject(&typed.Items[i]))
		}
		return items
	case *corev1.ConfigMapList:
		items := make([]any, 0, len(typed.Items))
		for i := range typed.Items {
			items = append(items, support.SnapshotObject(&typed.Items[i]))
		}
		return items
	default:
		return map[string]any{"error": fmt.Sprintf("unsupported list type %T", list)}
	}
}

func (ft *foundationTests) mustListPodSnapshots(t *testing.T, namespace string) any {
	t.Helper()

	podList := &corev1.PodList{}
	if err := ft.Client.List(t.Context(), podList, client.InNamespace(namespace)); err != nil {
		return map[string]any{"error": err.Error()}
	}

	items := make([]any, 0, len(podList.Items))
	for i := range podList.Items {
		items = append(items, snapshotPod(&podList.Items[i]))
	}

	return items
}

func stringifyFailureDumpValue(value any) string {
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Sprintf("%+v", value)
	}

	return string(data)
}
