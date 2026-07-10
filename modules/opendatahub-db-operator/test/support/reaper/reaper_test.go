package reaper

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	infraApi "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/api/infrastructure/v1alpha1"
	modulemanager "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/pkg/manager"
	. "github.com/onsi/gomega"
)

func TestNewRejectsNilClient(t *testing.T) {
	t.Parallel()

	g := NewWithT(t)

	r, err := New(nil)

	g.Expect(err).To(HaveOccurred())
	g.Expect(r).To(BeNil())
}

func TestRunCleansManagedResources(t *testing.T) {
	t.Parallel()

	g := NewWithT(t)
	ctx := context.Background()
	namespace := "test-ns"
	scheme := modulemanager.NewScheme()

	provider := &infraApi.DatabaseProvider{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "provider-stale",
			Finalizers: []string{"test/finalizer"},
		},
		Spec: infraApi.DatabaseProviderSpec{
			Type: infraApi.ProviderTypeEmbedded,
			Embedded: &infraApi.EmbeddedProviderSpec{
				Storage: infraApi.StorageSpec{},
			},
		},
	}
	schemaClaim := &infraApi.SchemaClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "schema-claim",
			Namespace:  namespace,
			Finalizers: []string{"test/finalizer"},
		},
	}
	databaseClaim := &infraApi.DatabaseClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "database-claim",
			Namespace:  namespace,
			Finalizers: []string{"test/finalizer"},
		},
	}
	claimSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "claim-secret",
			Namespace:  namespace,
			Finalizers: []string{"test/finalizer"},
		},
	}

	cli := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(
			provider,
			schemaClaim,
			databaseClaim,
			claimSecret,
		).
		Build()

	r, err := New(
		cli,
	)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(r.Run(ctx)).To(Succeed())

	expectNotFound(g, ctx, cli, &infraApi.DatabaseProvider{ObjectMeta: metav1.ObjectMeta{Name: provider.Name}})
	expectNotFound(g, ctx, cli, &infraApi.SchemaClaim{
		ObjectMeta: metav1.ObjectMeta{Name: schemaClaim.Name, Namespace: namespace},
	})
	expectNotFound(g, ctx, cli, &infraApi.DatabaseClaim{
		ObjectMeta: metav1.ObjectMeta{Name: databaseClaim.Name, Namespace: namespace},
	})
}

func expectNotFound(g *WithT, ctx context.Context, cli client.Client, obj client.Object) {
	g.Expect(cli.Get(ctx, client.ObjectKeyFromObject(obj), obj)).ToNot(Succeed())
}
