//go:build e2e

package e2e

import (
	"context"
	"fmt"
	"testing"
	"time"

	. "github.com/onsi/gomega"
	"github.com/rs/xid"
	admissionv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/lburgazzoli/gomega-matchers/pkg/matchers/jq"
	k8sm "github.com/lburgazzoli/gomega-matchers/pkg/matchers/k8s"
	infrav1 "github.com/opendatahub-io/opendatahub-operator/v2/api/infrastructure/v1alpha1"

	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-workbenches-operator/test/support"
)

const (
	connectionAnnotation   = "opendatahub.io/connections"
	hwpNameAnnotation      = "opendatahub.io/hardware-profile-name"
	hwpNamespaceAnnotation = "opendatahub.io/hardware-profile-namespace"
	notebookSampleImage    = "quay.io/thoth-station/s2i-minimal-notebook:v0.2.2"
)

type webhookTests struct {
	Client client.Client
}

func (wt *webhookTests) Execute(t *testing.T) {
	t.Run("should deploy webhook resources", wt.testWebhookResources)
	t.Run("should exercise webhook examples", wt.testWebhookExamples)
}

func (wt *webhookTests) testWebhookResources(t *testing.T) {
	g := NewWithT(t)

	webhookService := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "odh-workbenches-webhook-service",
			Namespace: support.OperatorNamespace(),
		},
	}
	webhookConfig := &admissionv1.MutatingWebhookConfiguration{
		ObjectMeta: metav1.ObjectMeta{
			Name: "odh-workbenches-mutating-webhook-configuration",
		},
	}

	g.Eventually(t.Context(), k8sm.Get(wt.Client, webhookService)).Should(And(
		jq.Match(`.metadata.name == "odh-workbenches-webhook-service"`),
		jq.Match(`[.spec.ports[] | select(.port == 443 and .targetPort == 9443)] | length == 1`),
	))

	g.Eventually(t.Context(), k8sm.Get(wt.Client, webhookConfig)).Should(And(
		jq.Match(`.metadata.name == "odh-workbenches-mutating-webhook-configuration"`),
		jq.Match(`.webhooks[] | select(.name == "connection-notebook.opendatahub.io")`+
			` | .clientConfig.service.name == "odh-workbenches-webhook-service"`),
		jq.Match(`.webhooks[] | select(.name == "hardwareprofile-notebook-injector.opendatahub.io")`+
			` | .clientConfig.service.name == "odh-workbenches-webhook-service"`),
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
		connectionAnnotation: fmt.Sprintf("%s/%s", operatorNamespace, secret.GetName()),
	})

	cleanupObject(t, wt.Client, secret)
	cleanupObject(t, wt.Client, nb)
	t.Cleanup(func() {
		cleanupObjectBackground(t, wt.Client, nb)
		cleanupObjectBackground(t, wt.Client, secret)
	})

	g.Expect(wt.Client.Create(t.Context(), secret)).To(Succeed())
	g.Expect(wt.Client.Create(t.Context(), nb)).To(Succeed())

	stored := &unstructured.Unstructured{}
	stored.SetGroupVersionKind(nb.GroupVersionKind())
	stored.SetName(nb.GetName())
	stored.SetNamespace(nb.GetNamespace())
	g.Eventually(t.Context(), k8sm.Get(wt.Client, stored)).Should(And(
		jq.Matchf(
			`((.spec.template.spec.containers[0].envFrom // []) | map(select(.secretRef.name == "%s")) | length) == 1`,
			secret.GetName(),
		),
	))
}

func (wt *webhookTests) testConnectionWebhookDeniesMissingSecret(t *testing.T) {
	g := NewWithT(t)
	operatorNamespace := support.OperatorNamespace()
	nb := newWebhookExampleNotebook(operatorNamespace, "workbenches-webhook-conn-missing-"+xid.New().String())
	nb.SetAnnotations(map[string]string{
		connectionAnnotation: fmt.Sprintf("%s/%s", operatorNamespace, "missing-secret"),
	})

	cleanupObject(t, wt.Client, nb)
	t.Cleanup(func() {
		cleanupObjectBackground(t, wt.Client, nb)
	})

	err := wt.Client.Create(t.Context(), nb)
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

	cleanupObject(t, wt.Client, hwp)
	cleanupObject(t, wt.Client, nb)
	t.Cleanup(func() {
		cleanupObjectBackground(t, wt.Client, nb)
		cleanupObjectBackground(t, wt.Client, hwp)
	})

	g.Expect(wt.Client.Create(t.Context(), hwp)).To(Succeed())
	g.Expect(wt.Client.Create(t.Context(), nb)).To(Succeed())

	stored := &unstructured.Unstructured{}
	stored.SetGroupVersionKind(nb.GroupVersionKind())
	stored.SetName(nb.GetName())
	stored.SetNamespace(nb.GetNamespace())
	g.Eventually(t.Context(), k8sm.Get(wt.Client, stored)).Should(And(
		k8sm.HasAnnotation(hwpNamespaceAnnotation, operatorNamespace),
		jq.Match(`.spec.template.spec.containers[0].resources.requests.cpu == "2"`),
		jq.Match(`.spec.template.spec.nodeSelector."kubernetes.io/os" == "linux"`),
	))
}

func (wt *webhookTests) testHardwareProfileWebhookDeniesMissingProfile(t *testing.T) {
	g := NewWithT(t)
	operatorNamespace := support.OperatorNamespace()
	nb := newWebhookExampleNotebook(operatorNamespace, "workbenches-webhook-hwp-missing-"+xid.New().String())
	nb.SetAnnotations(map[string]string{
		hwpNameAnnotation: "missing-profile",
	})

	cleanupObject(t, wt.Client, nb)
	t.Cleanup(func() {
		cleanupObjectBackground(t, wt.Client, nb)
	})

	err := wt.Client.Create(t.Context(), nb)
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring(`hardware profile "missing-profile" not found`))
}

func deleteIfExists(ctx context.Context, cli client.Client, obj client.Object) {
	_ = cli.Delete(ctx, obj)
}

func cleanupObject(t *testing.T, cli client.Client, obj client.Object) {
	t.Helper()
	cleanupObjectWithContext(t, t.Context(), cli, obj)
}

func cleanupObjectBackground(t *testing.T, cli client.Client, obj client.Object) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cleanupObjectWithContext(t, ctx, cli, obj)
}

func cleanupObjectWithContext(t *testing.T, ctx context.Context, cli client.Client, obj client.Object) {
	t.Helper()

	deleteIfExists(ctx, cli, obj)

	g := NewWithT(t)
	g.Eventually(ctx, k8sm.NotFound(cli, obj)).Should(BeTrue())
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
