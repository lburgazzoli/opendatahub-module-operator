package controller_test

import (
	"context"
	"errors"
	"testing"

	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	infraApi "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/api/infrastructure/v1alpha1"
	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/pkg/controller"
	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/pkg/postgres"
)

func TestResolveProviderConfig_UsesRewriterForConnectionOnly(t *testing.T) {
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

	resolved, err := controller.ResolveProviderConfig(
		context.Background(),
		cli,
		provider,
		"odh-db-operator-system",
		controller.PostgresConnectionConfigResolveFunc(func(
			_ context.Context,
			_ *infraApi.DatabaseProvider,
			cfg postgres.Config,
		) (postgres.Config, error) {
			cfg.Host = "127.0.0.1"
			cfg.Port = 15432
			return cfg, nil
		}),
	)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(resolved.Published).To(Equal(postgres.Config{
		Host:     "postgres.db-admin.svc",
		Port:     postgres.DefaultPort,
		User:     "postgres",
		Password: "secret",
		DBName:   "postgres",
		SSLMode:  postgres.SSLModeRequire,
	}))
	g.Expect(resolved.Connection).To(Equal(postgres.Config{
		Host:     "127.0.0.1",
		Port:     15432,
		User:     "postgres",
		Password: "secret",
		DBName:   "postgres",
		SSLMode:  postgres.SSLModeRequire,
	}))
}

func TestResolveProviderConfig_PropagatesRewriteError(t *testing.T) {
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

	rewriteErr := errors.New("rewrite failed")
	_, err := controller.ResolveProviderConfig(
		context.Background(),
		cli,
		provider,
		"odh-db-operator-system",
		controller.PostgresConnectionConfigResolveFunc(func(
			_ context.Context,
			_ *infraApi.DatabaseProvider,
			cfg postgres.Config,
		) (postgres.Config, error) {
			return cfg, rewriteErr
		}),
	)
	g.Expect(err).To(MatchError(rewriteErr))
}

func TestDefaultPostgresConnectionConfigResolver_ReturnsInputConfig(t *testing.T) {
	g := NewWithT(t)

	cfg := postgres.Config{
		Host:     "postgres.db-admin.svc",
		Port:     5432,
		User:     "postgres",
		Password: "secret",
		DBName:   "postgres",
	}

	got, err := controller.DefaultPostgresConnectionConfigResolver().Resolve(
		context.Background(),
		&infraApi.DatabaseProvider{},
		cfg,
	)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(got).To(Equal(cfg))
}
