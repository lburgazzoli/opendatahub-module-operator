package controller_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	infraApi "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/api/infrastructure/v1alpha1"
	moduleconfig "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/pkg/config"
	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/pkg/controller"
	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/pkg/postgres"
)

func TestNewClient_LoadsProviderConfigAndUsesFactory(t *testing.T) {
	g := NewWithT(t)

	provider := &infraApi.DatabaseProvider{
		ObjectMeta: metav1.ObjectMeta{Name: "sample-external"},
		Spec: infraApi.DatabaseProviderSpec{
			Type: infraApi.ProviderTypeExternal,
			External: &infraApi.ExternalProviderSpec{
				ConnectionSecretRef: corev1.SecretReference{
					Namespace: "db-admin",
					Name:      "external-admin",
				},
			},
		},
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "external-admin",
			Namespace: "db-admin",
		},
		Data: map[string][]byte{
			postgres.SecretKeyHost:     []byte("postgres.db-admin.svc"),
			postgres.SecretKeyPort:     []byte("5432"),
			postgres.SecretKeyUser:     []byte("postgres"),
			postgres.SecretKeyPassword: []byte("secret"),
			postgres.SecretKeyDatabase: []byte("postgres"),
		},
	}

	cli := fake.NewClientBuilder().
		WithScheme(providerConfigScheme()).
		WithObjects(secret).
		Build()

	var gotConfig postgres.Config
	client, cfg, err := controller.NewClient(
		context.Background(),
		cli,
		provider,
		&moduleconfig.Config{OperatorNamespace: "odh-db-operator-system"},
		func(_ context.Context, cfg postgres.Config) (postgres.Client, error) {
			gotConfig = cfg
			return stubClient{config: cfg}, nil
		},
	)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(cfg).To(Equal(postgres.Config{
		Host:     "postgres.db-admin.svc",
		Port:     postgres.DefaultPort,
		User:     "postgres",
		Password: "secret",
		DBName:   "postgres",
		SSLMode:  postgres.SSLModeRequire,
	}))
	g.Expect(gotConfig).To(Equal(cfg))
	g.Expect(client.Config()).To(Equal(cfg))
}

type stubClient struct {
	config postgres.Config
}

func (s stubClient) Config() postgres.Config  { return s.config }
func (stubClient) Close()                     {}
func (stubClient) Ping(context.Context) error { return nil }
func (stubClient) Exec(context.Context, string, ...any) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, nil
}
func (stubClient) Query(context.Context, string, ...any) (pgx.Rows, error) {
	return nil, nil
}
func (stubClient) QueryRow(context.Context, string, ...any) (pgx.Row, error) {
	return nil, nil
}
