package support

import (
	"context"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	modulemanager "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/pkg/manager"
	. "github.com/onsi/gomega"
)

func TestClearFinalizersAndDeleteDeletesObject(t *testing.T) {
	t.Parallel()

	g := NewWithT(t)
	ctx := context.Background()

	scheme, err := modulemanager.NewScheme()
	g.Expect(err).NotTo(HaveOccurred())

	obj := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-secret",
			Namespace:  "test-ns",
			Finalizers: []string{"test/finalizer"},
		},
	}

	cli := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(obj).
		Build()

	g.Expect(ClearFinalizersAndDelete(ctx, cli, obj)).To(Succeed())

	current := &corev1.Secret{}
	err = cli.Get(ctx, client.ObjectKeyFromObject(obj), current)
	g.Expect(apierrors.IsNotFound(err)).To(BeTrue())
}

func TestClearFinalizersAndDeleteIgnoresMissingObject(t *testing.T) {
	t.Parallel()

	g := NewWithT(t)
	ctx := context.Background()

	scheme, err := modulemanager.NewScheme()
	g.Expect(err).NotTo(HaveOccurred())

	obj := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "missing-secret",
			Namespace: "test-ns",
		},
	}

	cli := fake.NewClientBuilder().
		WithScheme(scheme).
		Build()

	g.Expect(ClearFinalizersAndDelete(ctx, cli, obj)).To(Succeed())
}

func TestDeleteAndWaitDeletesObjectAndResetsFields(t *testing.T) {
	t.Parallel()

	g := NewWithT(t)
	ctx := context.Background()

	scheme, err := modulemanager.NewScheme()
	g.Expect(err).NotTo(HaveOccurred())

	obj := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "test-secret",
			Namespace:       "test-ns",
			Finalizers:      []string{"test/finalizer"},
			ResourceVersion: "123",
			UID:             "uid-123",
		},
	}

	cli := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(obj).
		Build()

	g.Expect(DeleteAndWait(ctx, cli, obj)).To(Succeed())

	current := &corev1.Secret{}
	err = cli.Get(ctx, client.ObjectKey{Name: obj.Name, Namespace: obj.Namespace}, current)
	g.Expect(apierrors.IsNotFound(err)).To(BeTrue())
	g.Expect(obj.GetResourceVersion()).To(BeEmpty())
	g.Expect(obj.GetUID()).To(BeEmpty())
	g.Expect(obj.GetDeletionTimestamp()).To(BeNil())
	g.Expect(obj.GetFinalizers()).To(BeNil())
}

func TestDeleteAndWaitIgnoresMissingObject(t *testing.T) {
	t.Parallel()

	g := NewWithT(t)
	ctx := context.Background()

	scheme, err := modulemanager.NewScheme()
	g.Expect(err).NotTo(HaveOccurred())

	obj := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "missing-secret",
			Namespace:       "test-ns",
			ResourceVersion: "123",
			UID:             "uid-123",
			Finalizers:      []string{"test/finalizer"},
		},
	}

	cli := fake.NewClientBuilder().
		WithScheme(scheme).
		Build()

	g.Expect(DeleteAndWait(ctx, cli, obj)).To(Succeed())
	g.Expect(obj.GetResourceVersion()).To(BeEmpty())
	g.Expect(obj.GetUID()).To(BeEmpty())
	g.Expect(obj.GetDeletionTimestamp()).To(BeNil())
	g.Expect(obj.GetFinalizers()).To(BeNil())
}

func TestDeleteAndWaitTreatsRecreatedObjectAsDeleted(t *testing.T) {
	t.Parallel()

	g := NewWithT(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	scheme, err := modulemanager.NewScheme()
	g.Expect(err).NotTo(HaveOccurred())

	obj := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "test-secret",
			Namespace:       "test-ns",
			ResourceVersion: "123",
			UID:             "uid-old",
		},
	}

	cli := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(obj).
		Build()

	go func() {
		<-time.After(10 * time.Millisecond)
		_ = cli.Create(context.Background(), &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      obj.Name,
				Namespace: obj.Namespace,
				UID:       "uid-new",
			},
		})
	}()

	g.Expect(DeleteAndWait(ctx, cli, obj)).To(Succeed())

	current := &corev1.Secret{}
	g.Eventually(func() error {
		return cli.Get(
			context.Background(),
			client.ObjectKey{Name: obj.Name, Namespace: obj.Namespace},
			current,
		)
	}).Should(Succeed())
	g.Expect(current.GetUID()).To(Equal(types.UID("uid-new")))
	g.Expect(obj.GetUID()).To(BeEmpty())
}
