package portforward

import (
	"testing"
	"time"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestOptionsValidate(t *testing.T) {
	t.Run("requires address", func(t *testing.T) {
		g := NewWithT(t)
		err := (Options{}).Validate()
		g.Expect(err).To(MatchError("at least one listen address is required"))
	})

	t.Run("rejects invalid local port", func(t *testing.T) {
		g := NewWithT(t)
		err := (Options{Addresses: []string{"127.0.0.1"}, LocalPort: 70000}).Validate()
		g.Expect(err).To(MatchError(ContainSubstring("local port must be between 0 and 65535")))
	})
}

func TestSelectReadyPod(t *testing.T) {
	g := NewWithT(t)
	now := metav1.NewTime(time.Now())

	name, err := selectReadyPod([]corev1.Pod{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:              "deleting",
				DeletionTimestamp: &now,
			},
			Status: corev1.PodStatus{Phase: corev1.PodRunning},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Name: "pending"},
			Status:     corev1.PodStatus{Phase: corev1.PodPending},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Name: "not-ready"},
			Status: corev1.PodStatus{
				Phase: corev1.PodRunning,
				Conditions: []corev1.PodCondition{
					{Type: corev1.PodReady, Status: corev1.ConditionFalse},
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Name: "ready"},
			Status: corev1.PodStatus{
				Phase: corev1.PodRunning,
				Conditions: []corev1.PodCondition{
					{Type: corev1.PodReady, Status: corev1.ConditionTrue},
				},
			},
		},
	})
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(name).To(Equal("ready"))
}

func TestSelectReadyPod_ReturnsErrorWhenNoReadyPodExists(t *testing.T) {
	g := NewWithT(t)

	_, err := selectReadyPod([]corev1.Pod{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "pending"},
			Status:     corev1.PodStatus{Phase: corev1.PodPending},
		},
	})
	g.Expect(err).To(MatchError("no running ready pod found for port-forward"))
}
