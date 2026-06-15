package support

import (
	"context"
	"errors"
	"testing"

	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestWrapPollCallsPollAndReturnsError(t *testing.T) {
	g := NewWithT(t)

	wantErr := errors.New("boom")
	pollCalls := 0

	wrapped := WrapPoll(t, "poll", func() error {
		pollCalls++
		return wantErr
	}, nil)

	err := wrapped()

	g.Expect(err).To(MatchError(wantErr))
	g.Expect(pollCalls).To(Equal(1))
}

func TestWrapPollRendersLatestValue(t *testing.T) {
	g := NewWithT(t)

	current := 0
	rendered := 0

	wrapped := WrapPoll(t, "poll", func() error {
		current = 41
		return nil
	}, func() any {
		rendered = current
		return map[string]any{"value": current}
	})

	g.Expect(wrapped()).To(Succeed())
	g.Expect(rendered).To(Equal(41))
}

func TestWrapEventuallyCallsPollAndReturnsValue(t *testing.T) {
	g := NewWithT(t)

	type payload struct {
		Value int
	}

	pollCalls := 0
	wrapped := WrapEventually(t, "poll", func(context.Context) (*payload, error) {
		pollCalls++
		return &payload{Value: 7}, nil
	}, func(value *payload) any {
		return map[string]any{"value": value.Value}
	})

	value, err := wrapped(t.Context())

	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(pollCalls).To(Equal(1))
	g.Expect(value).NotTo(BeNil())
	g.Expect(value.Value).To(Equal(7))
}

func TestSnapshotObjectIncludesIdentityAndPayload(t *testing.T) {
	g := NewWithT(t)

	object := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example",
			Namespace: "test-ns",
		},
		Data: map[string]string{
			"key": "value",
		},
	}

	snapshot, ok := SnapshotObject(object).(map[string]any)
	g.Expect(ok).To(BeTrue())
	g.Expect(snapshot).To(HaveKeyWithValue("name", "example"))
	g.Expect(snapshot).To(HaveKeyWithValue("namespace", "test-ns"))
	g.Expect(snapshot).To(HaveKey("object"))
}

func TestSnapshotDeploymentStatusIncludesIdentityAndStatus(t *testing.T) {
	g := NewWithT(t)

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example",
			Namespace: "test-ns",
		},
		Status: appsv1.DeploymentStatus{
			Replicas:      1,
			ReadyReplicas: 0,
		},
	}

	snapshot, ok := SnapshotDeploymentStatus(deployment).(map[string]any)
	g.Expect(ok).To(BeTrue())
	g.Expect(snapshot).To(HaveKeyWithValue("name", "example"))
	g.Expect(snapshot).To(HaveKeyWithValue("namespace", "test-ns"))
	g.Expect(snapshot).To(HaveKey("status"))
}
