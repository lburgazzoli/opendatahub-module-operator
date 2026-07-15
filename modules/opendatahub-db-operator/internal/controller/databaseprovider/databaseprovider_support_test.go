/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package databaseprovider

import (
	"context"
	"errors"
	"testing"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	infraApi "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/api/infrastructure/v1alpha1"
	dbcontroller "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/pkg/controller"
	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/pkg/postgres"
)

func TestLoadExternalConfig_InvalidSecret(t *testing.T) {
	newFixture := func(t *testing.T, data map[string][]byte) (*infraApi.DatabaseProvider, *corev1.Secret) {
		t.Helper()

		return &infraApi.DatabaseProvider{
				ObjectMeta: metav1.ObjectMeta{Name: "provider"},
				Spec: infraApi.DatabaseProviderSpec{
					Type: infraApi.ProviderTypeExternal,
					External: &infraApi.ExternalProviderSpec{
						ConnectionSecretRef: corev1.SecretReference{
							Name:      "admin",
							Namespace: "ns",
						},
					},
				},
			}, &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: "admin", Namespace: "ns"},
				Data:       data,
			}
	}

	newClient := func(t *testing.T, objs ...runtime.Object) *fake.ClientBuilder {
		t.Helper()

		scheme := runtime.NewScheme()
		NewWithT(t).Expect(corev1.AddToScheme(scheme)).To(Succeed())
		NewWithT(t).Expect(infraApi.AddToScheme(scheme)).To(Succeed())

		return fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(objs...)
	}

	t.Run("missing key", func(t *testing.T) {
		g := NewWithT(t)

		provider, secret := newFixture(t, map[string][]byte{
			postgres.SecretKeyPort:     []byte("5432"),
			postgres.SecretKeyUser:     []byte("admin"),
			postgres.SecretKeyPassword: []byte("secret"),
			postgres.SecretKeyDatabase: []byte("postgres"),
		})

		_, err := loadExternalConfig(context.Background(), newClient(t, secret).Build(), provider)
		var invalid ErrConnectionSecretInvalid

		g.Expect(errors.As(err, &invalid)).To(BeTrue())
		g.Expect(invalid.Error()).To(ContainSubstring(postgres.SecretKeyHost))
	})

	t.Run("empty value", func(t *testing.T) {
		g := NewWithT(t)

		provider, secret := newFixture(t, map[string][]byte{
			postgres.SecretKeyHost:     []byte("localhost"),
			postgres.SecretKeyPort:     []byte("5432"),
			postgres.SecretKeyUser:     []byte("admin"),
			postgres.SecretKeyPassword: []byte(""),
			postgres.SecretKeyDatabase: []byte("postgres"),
		})

		_, err := loadExternalConfig(context.Background(), newClient(t, secret).Build(), provider)
		var invalid ErrConnectionSecretInvalid

		g.Expect(errors.As(err, &invalid)).To(BeTrue())
		g.Expect(invalid.Error()).To(ContainSubstring(postgres.SecretKeyPassword))
	})

	t.Run("malformed port", func(t *testing.T) {
		g := NewWithT(t)

		provider, secret := newFixture(t, map[string][]byte{
			postgres.SecretKeyHost:     []byte("localhost"),
			postgres.SecretKeyPort:     []byte("not-a-port"),
			postgres.SecretKeyUser:     []byte("admin"),
			postgres.SecretKeyPassword: []byte("secret"),
			postgres.SecretKeyDatabase: []byte("postgres"),
		})

		_, err := loadExternalConfig(context.Background(), newClient(t, secret).Build(), provider)
		var invalid ErrConnectionSecretInvalid

		g.Expect(errors.As(err, &invalid)).To(BeTrue())
		g.Expect(invalid.Error()).To(ContainSubstring(postgres.SecretKeyPort))
	})
}

func TestProviderConnectionStatus(t *testing.T) {
	g := NewWithT(t)

	status := providerConnectionStatus(postgres.Config{
		Host:   "db.example.test",
		Port:   5432,
		DBName: "postgres",
	})

	g.Expect(status).To(Equal(infraApi.ProviderConnectionStatus{
		Host:     "db.example.test",
		Port:     5432,
		Database: "postgres",
	}))
}

func TestResolveExternalConfig_UsesRewriterForConnectionOnly(t *testing.T) {
	g := NewWithT(t)

	provider := &infraApi.DatabaseProvider{
		ObjectMeta: metav1.ObjectMeta{Name: "provider"},
		Spec: infraApi.DatabaseProviderSpec{
			Type: infraApi.ProviderTypeExternal,
			External: &infraApi.ExternalProviderSpec{
				ConnectionSecretRef: corev1.SecretReference{
					Name:      "admin",
					Namespace: "ns",
				},
			},
		},
	}
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "admin", Namespace: "ns"},
		Data: map[string][]byte{
			postgres.SecretKeyHost:     []byte("provider.ns.svc"),
			postgres.SecretKeyPort:     []byte("5432"),
			postgres.SecretKeyUser:     []byte("admin"),
			postgres.SecretKeyPassword: []byte("secret"),
			postgres.SecretKeyDatabase: []byte("postgres"),
		},
	}

	scheme := runtime.NewScheme()
	g.Expect(corev1.AddToScheme(scheme)).To(Succeed())
	g.Expect(infraApi.AddToScheme(scheme)).To(Succeed())

	resolved, err := resolveExternalConfig(
		context.Background(),
		fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(secret).Build(),
		provider,
		dbcontroller.PostgresConnectionConfigResolveFunc(func(
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
	g.Expect(resolved.Published.Host).To(Equal("provider.ns.svc"))
	g.Expect(resolved.Published.Port).To(Equal(5432))
	g.Expect(resolved.Connection.Host).To(Equal("127.0.0.1"))
	g.Expect(resolved.Connection.Port).To(Equal(15432))
}
