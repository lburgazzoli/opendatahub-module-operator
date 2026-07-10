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
	"testing"

	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	infraApi "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/api/infrastructure/v1alpha1"
	moduleconfig "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/pkg/config"
	dbcontroller "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/pkg/controller"
	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/pkg/postgres"
	common "github.com/opendatahub-io/odh-platform-utilities/api/common"
)

func TestResolveEmbeddedImage(t *testing.T) {
	cfg := &moduleconfig.Config{
		Embedded: moduleconfig.EmbeddedConfig{
			PostgresImage: "postgres:test",
			PgvectorImage: "pgvector:test",
		},
	}

	tests := []struct {
		name       string
		extensions []string
		wantImage  string
	}{
		{
			name:       "vector selects pgvector image",
			extensions: []string{"vector", "pg_trgm"},
			wantImage:  "pgvector:test",
		},
		{
			name:       "stock extensions select postgres image",
			extensions: []string{"pg_trgm", "pgcrypto"},
			wantImage:  "postgres:test",
		},
		{
			name:       "no extensions selects postgres image",
			extensions: nil,
			wantImage:  "postgres:test",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)

			provider := &infraApi.DatabaseProvider{
				ObjectMeta: metav1.ObjectMeta{Name: "provider"},
				Spec: infraApi.DatabaseProviderSpec{
					Type: infraApi.ProviderTypeEmbedded,
					Embedded: &infraApi.EmbeddedProviderSpec{
						Extensions: tc.extensions,
					},
				},
			}

			image, err := resolveEmbeddedImage(provider, cfg)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(image).To(Equal(tc.wantImage))
		})
	}
}

func TestComputeEmbeddedAdminSecret_GeneratesCredentialsOnFirstReconcile(t *testing.T) {
	g := NewWithT(t)
	ctx := context.Background()

	scheme := runtime.NewScheme()
	g.Expect(appsv1.AddToScheme(scheme)).To(Succeed())
	g.Expect(corev1.AddToScheme(scheme)).To(Succeed())
	g.Expect(infraApi.AddToScheme(scheme)).To(Succeed())

	cfg := &moduleconfig.Config{OperatorNamespace: "operator-ns"}
	provider := &infraApi.DatabaseProvider{
		ObjectMeta: metav1.ObjectMeta{Name: "embedded"},
		Spec: infraApi.DatabaseProviderSpec{
			Type:     infraApi.ProviderTypeEmbedded,
			Embedded: &infraApi.EmbeddedProviderSpec{},
		},
	}

	cli := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(provider).Build()

	secret, err := computeEmbeddedAdminSecret(ctx, cli, provider, cfg)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(secret.Data).To(SatisfyAll(
		HaveKey(postgres.SecretKeyHost),
		HaveKey(postgres.SecretKeyUser),
		HaveKey(postgres.SecretKeyPassword),
		HaveKey(postgres.SecretKeyDatabase),
	))
}

func TestComputeEmbeddedAdminSecret_PreservesExistingPassword(t *testing.T) {
	g := NewWithT(t)
	ctx := context.Background()

	scheme := runtime.NewScheme()
	g.Expect(appsv1.AddToScheme(scheme)).To(Succeed())
	g.Expect(corev1.AddToScheme(scheme)).To(Succeed())
	g.Expect(infraApi.AddToScheme(scheme)).To(Succeed())

	cfg := &moduleconfig.Config{OperatorNamespace: "operator-ns"}
	provider := &infraApi.DatabaseProvider{
		ObjectMeta: metav1.ObjectMeta{Name: "embedded"},
		Spec: infraApi.DatabaseProviderSpec{
			Type:     infraApi.ProviderTypeEmbedded,
			Embedded: &infraApi.EmbeddedProviderSpec{},
		},
	}
	// Simulate deploy action having already created the Secret.
	existing := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: cfg.OperatorNamespace,
			Name:      dbcontroller.EmbeddedAdminSecretName(provider.Name),
		},
		Data: map[string][]byte{
			postgres.SecretKeyHost:     []byte("embedded.operator-ns.svc"),
			postgres.SecretKeyPort:     []byte("5432"),
			postgres.SecretKeyUser:     []byte("postgres"),
			postgres.SecretKeyPassword: []byte("stable-password"),
			postgres.SecretKeyDatabase: []byte("postgres"),
		},
	}

	cli := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(provider, existing).Build()

	first, err := computeEmbeddedAdminSecret(ctx, cli, provider, cfg)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(first.Data[postgres.SecretKeyPassword]).To(Equal([]byte("stable-password")))

	second, err := computeEmbeddedAdminSecret(ctx, cli, provider, cfg)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(second.Data).To(Equal(first.Data))
}

func TestEmbeddedNamespace_UsesOverride(t *testing.T) {
	g := NewWithT(t)

	cfg := &moduleconfig.Config{OperatorNamespace: "operator-ns"}
	provider := &infraApi.DatabaseProvider{
		ObjectMeta: metav1.ObjectMeta{Name: "embedded"},
		Spec: infraApi.DatabaseProviderSpec{
			Type: infraApi.ProviderTypeEmbedded,
			Embedded: &infraApi.EmbeddedProviderSpec{
				Namespace: "custom-ns",
			},
		},
	}

	g.Expect(dbcontroller.EmbeddedNamespace(provider, cfg.OperatorNamespace)).To(Equal("custom-ns"))
	g.Expect(dbcontroller.EmbeddedServiceHost(provider, cfg.OperatorNamespace)).To(Equal("embedded.custom-ns.svc"))
}

func TestReferencedClaimNamespaces_UsesPinnedProvider(t *testing.T) {
	g := NewWithT(t)
	ctx := context.Background()

	scheme := runtime.NewScheme()
	g.Expect(infraApi.AddToScheme(scheme)).To(Succeed())

	alpha := &infraApi.DatabaseProvider{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "alpha",
			Labels: map[string]string{"cap": "vec"},
		},
	}
	bravo := &infraApi.DatabaseProvider{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "bravo",
			Labels: map[string]string{"cap": "vec"},
		},
	}
	claim := &infraApi.SchemaClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "claim",
			Namespace: "workloads",
		},
		Spec: infraApi.SchemaClaimSpec{
			Provider: infraApi.ProviderRef{
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{"cap": "vec"},
				},
			},
		},
		Status: infraApi.SchemaClaimStatus{
			Status: common.Status{
				Conditions: []common.Condition{
					{
						Type:   "Provisioned",
						Status: metav1.ConditionTrue,
					},
				},
			},
			Provider: "bravo",
		},
	}

	cli := fake.NewClientBuilder().
		WithScheme(scheme).
		WithRuntimeObjects(alpha, bravo, claim).
		Build()

	namespaces, err := referencedClaimNamespaces(ctx, cli, bravo)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(namespaces).To(Equal([]string{"workloads"}))
}
