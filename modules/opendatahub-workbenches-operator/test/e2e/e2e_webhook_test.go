//go:build e2e

package e2e

import (
	"fmt"
	"testing"
	"time"

	"github.com/rs/xid"
	. "github.com/onsi/gomega"
	admissionv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	k8serr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/lburgazzoli/gomega-matchers/pkg/matchers/jq"
	infrav1 "github.com/opendatahub-io/opendatahub-operator/v2/api/infrastructure/v1"
	"github.com/opendatahub-io/opendatahub-operator/v2/pkg/metadata/annotations"
	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-workbenches-operator/test/support"
)

const (
	hwpNameAnnotation      = "opendatahub.io/hardware-profile-name"
	hwpNamespaceAnnotation = "opendatahub.io/hardware-profile-namespace"
	notebookSampleImage    = "quay.io/thoth-station/s2i-minimal-notebook:v0.2.2"
)

type webhookTests struct {
	*workbenchesE2ETest
	webhookService *corev1.Service
	webhookConfig  *admissionv1.MutatingWebhookConfiguration
}

func newWebhookTests(suite *workbenchesE2ETest) *webhookTests {
	return &webhookTests{
		workbenchesE2ETest: suite,
		webhookService: &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "opendatahub-workbenches-webhook-service",
				Namespace: suite.operatorNamespace,
			},
		},
		webhookConfig: &admissionv1.MutatingWebhookConfiguration{
			ObjectMeta: metav1.ObjectMeta{
				Name: "opendatahub-workbenches-mutating-webhook-configuration",
			},
		},
	}
}

func (wt *webhookTests) Execute(t *testing.T) {
	t.Run("should deploy webhook resources", wt.testWebhookResources)
	t.Run("should exercise webhook examples", wt.testWebhookExamples)
}

func (wt *webhookTests) testWebhookResources(t *testing.T) {
	g := NewWithT(t)

	g.Eventually(k.Get(wt.webhookService)).WithContext(ctx).WithTimeout(timeout).WithPolling(interval).Should(And(
		jq.Match(`.metadata.name == "opendatahub-workbenches-webhook-service"`),
		jq.Match(`[.spec.ports[] | select(.port == 443 and .targetPort == 9443)] | length == 1`),
	))

	g.Eventually(k.Get(wt.webhookConfig)).WithContext(ctx).WithTimeout(timeout).WithPolling(interval).Should(And(
		jq.Match(`.metadata.name == "opendatahub-workbenches-mutating-webhook-configuration"`),
		jq.Match(`.webhooks[] | select(.name == "connection-notebook.opendatahub.io") | .clientConfig.service.name == "opendatahub-workbenches-webhook-service"`),
		jq.Match(`.webhooks[] | select(.name == "hardwareprofile-notebook-injector.opendatahub.io") | .clientConfig.service.name == "opendatahub-workbenches-webhook-service"`),
	))
}

func (wt *webhookTests) testWebhookExamples(t *testing.T) {
	t.Run("connection webhook injects secret envFrom", wt.testConnectionWebhookInjectsSecretEnvFrom)
	t.Run("connection webhook denies missing secret", wt.testConnectionWebhookDeniesMissingSecret)
	t.Run("hardware profile webhook mutates notebook", wt.testHardwareProfileWebhookMutatesNotebook)
	t.Run("hardware profile webhook denies missing profile", wt.testHardwareProfileWebhookDeniesMissingProfile)
}

func (wt *webhookTests) testConnectionWebhookInjectsSecretEnvFrom(t *testing.T) {
	g := NewWithT(t)
	operatorNamespace := support.OperatorNamespace()
	suffix := xid.New().String()
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "workbenches-webhook-conn-secret-" + suffix,
			Namespace: operatorNamespace,
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{
			"username": []byte("user"),
		},
	}
	nb := newWebhookExampleNotebook(operatorNamespace, "workbenches-webhook-conn-valid-"+suffix)
	nb.SetAnnotations(map[string]string{
		annotations.Connection: fmt.Sprintf("%s/%s", operatorNamespace, secret.GetName()),
	})

	cleanupObject(t, secret)
	cleanupObject(t, nb)
	t.Cleanup(func() {
		cleanupObject(t, nb)
		cleanupObject(t, secret)
	})

	g.Expect(k8sClient.Create(ctx, secret)).To(Succeed())
	g.Expect(k8sClient.Create(ctx, nb)).To(Succeed())

	g.Eventually(func(g Gomega) {
		stored := &unstructured.Unstructured{}
		stored.SetGroupVersionKind(nb.GroupVersionKind())
		g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(nb), stored)).To(Succeed())

		containers, found, err := unstructured.NestedSlice(stored.Object, "spec", "template", "spec", "containers")
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(found).To(BeTrue())
		envFrom := containers[0].(map[string]any)["envFrom"].([]any)
		g.Expect(envFrom).To(HaveLen(1))
		g.Expect(envFrom[0].(map[string]any)["secretRef"].(map[string]any)["name"]).To(Equal(secret.GetName()))
	}).WithContext(ctx).WithTimeout(timeout).WithPolling(interval).Should(Succeed())
}

func (wt *webhookTests) testConnectionWebhookDeniesMissingSecret(t *testing.T) {
	g := NewWithT(t)
	operatorNamespace := support.OperatorNamespace()
	nb := newWebhookExampleNotebook(operatorNamespace, "workbenches-webhook-conn-missing-"+xid.New().String())
	nb.SetAnnotations(map[string]string{
		annotations.Connection: fmt.Sprintf("%s/%s", operatorNamespace, "missing-secret"),
	})

	cleanupObject(t, nb)
	t.Cleanup(func() {
		cleanupObject(t, nb)
	})

	err := k8sClient.Create(ctx, nb)
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("connection secrets not found"))
}

func (wt *webhookTests) testHardwareProfileWebhookMutatesNotebook(t *testing.T) {
	g := NewWithT(t)
	operatorNamespace := support.OperatorNamespace()
	suffix := xid.New().String()
	hwp := &infrav1.HardwareProfile{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "workbenches-webhook-hwp-valid-" + suffix,
			Namespace: operatorNamespace,
		},
		Spec: infrav1.HardwareProfileSpec{
			Identifiers: []infrav1.HardwareIdentifier{{
				Identifier:   "cpu",
				DefaultCount: intstr.FromString("2"),
			}},
			SchedulingSpec: &infrav1.SchedulingSpec{
				SchedulingType: infrav1.NodeScheduling,
				Node: &infrav1.NodeSchedulingSpec{
					NodeSelector: map[string]string{
						"kubernetes.io/os": "linux",
					},
					Tolerations: []corev1.Toleration{{
						Key:      "gpu",
						Operator: corev1.TolerationOpExists,
						Effect:   corev1.TaintEffectNoSchedule,
					}},
				},
			},
		},
	}
	nb := newWebhookExampleNotebook(operatorNamespace, "workbenches-webhook-hwp-notebook-"+suffix)
	nb.SetAnnotations(map[string]string{
		hwpNameAnnotation: hwp.GetName(),
	})

	cleanupObject(t, hwp)
	cleanupObject(t, nb)
	t.Cleanup(func() {
		cleanupObject(t, nb)
		cleanupObject(t, hwp)
	})

	g.Expect(k8sClient.Create(ctx, hwp)).To(Succeed())
	g.Expect(k8sClient.Create(ctx, nb)).To(Succeed())

	g.Eventually(func(g Gomega) {
		stored := &unstructured.Unstructured{}
		stored.SetGroupVersionKind(nb.GroupVersionKind())
		g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(nb), stored)).To(Succeed())

		g.Expect(stored.GetAnnotations()).To(HaveKeyWithValue(hwpNamespaceAnnotation, operatorNamespace))

		containers, found, err := unstructured.NestedSlice(stored.Object, "spec", "template", "spec", "containers")
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(found).To(BeTrue())
		resourcesMap := containers[0].(map[string]any)["resources"].(map[string]any)
		requests := resourcesMap["requests"].(map[string]any)
		g.Expect(requests["cpu"]).To(Equal("2"))

		nodeSelector, found, err := unstructured.NestedStringMap(stored.Object, "spec", "template", "spec", "nodeSelector")
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(found).To(BeTrue())
		g.Expect(nodeSelector).To(HaveKeyWithValue("kubernetes.io/os", "linux"))
	}).WithContext(ctx).WithTimeout(timeout).WithPolling(interval).Should(Succeed())
}

func (wt *webhookTests) testHardwareProfileWebhookDeniesMissingProfile(t *testing.T) {
	g := NewWithT(t)
	operatorNamespace := support.OperatorNamespace()
	nb := newWebhookExampleNotebook(operatorNamespace, "workbenches-webhook-hwp-missing-"+xid.New().String())
	nb.SetAnnotations(map[string]string{
		hwpNameAnnotation: "missing-profile",
	})

	cleanupObject(t, nb)
	t.Cleanup(func() {
		cleanupObject(t, nb)
	})

	err := k8sClient.Create(ctx, nb)
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring(`hardware profile "missing-profile" not found`))
}

func deleteIfExists(obj client.Object) {
	_ = k8sClient.Delete(ctx, obj)
}

func cleanupObject(t *testing.T, obj client.Object) {
	t.Helper()

	deleteIfExists(obj)

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		fresh := obj.DeepCopyObject().(client.Object)
		err := k8sClient.Get(ctx, client.ObjectKeyFromObject(obj), fresh)
		if k8serr.IsNotFound(err) {
			return
		}

		time.Sleep(interval)
	}

	t.Logf("cleanup warning: object %T %s was not deleted before timeout", obj, client.ObjectKeyFromObject(obj))
}

func newWebhookExampleNotebook(namespace string, name string) *unstructured.Unstructured {
	nb := &unstructured.Unstructured{}
	nb.SetAPIVersion("kubeflow.org/v1")
	nb.SetKind("Notebook")
	nb.SetName(name)
	nb.SetNamespace(namespace)
	nb.Object["spec"] = map[string]any{
		"template": map[string]any{
			"spec": map[string]any{
				"containers": []any{
					map[string]any{
						"name":            name,
						"image":           notebookSampleImage,
						"imagePullPolicy": "Always",
						"workingDir":      "/opt/app-root/src",
						"env": []any{
							map[string]any{"name": "JUPYTER_NOTEBOOK_PORT", "value": "8888"},
							map[string]any{"name": "NOTEBOOK_ARGS", "value": "--NotebookApp.token='' --NotebookApp.password=''"},
						},
					},
				},
			},
		},
	}

	return nb
}
