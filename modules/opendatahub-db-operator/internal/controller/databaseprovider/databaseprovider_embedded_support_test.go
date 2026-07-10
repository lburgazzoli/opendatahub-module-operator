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
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	infraApi "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/api/infrastructure/v1alpha1"
	moduleconfig "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/pkg/config"
	dbcontroller "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/pkg/controller"
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
		wantErr    string
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
			name:       "unmapped extension fails",
			extensions: []string{"postgis"},
			wantErr:    `extension "postgis" does not map`,
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
			if tc.wantErr != "" {
				g.Expect(err).To(HaveOccurred())
				g.Expect(err.Error()).To(ContainSubstring(tc.wantErr))
				return
			}

			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(image).To(Equal(tc.wantImage))
		})
	}
}

func TestEnsureEmbeddedAdminSecret_Idempotent(t *testing.T) {
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

	cli := fake.NewClientBuilder().
		WithScheme(scheme).
		WithRuntimeObjects(provider).
		Build()

	first, err := ensureEmbeddedAdminSecret(ctx, cli, provider, cfg)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(first.Data).To(HaveKey(dbcontroller.EmbeddedAdminSecretPasswordKey))

	original := map[string][]byte{}
	for key, value := range first.Data {
		original[key] = append([]byte(nil), value...)
	}

	second, err := ensureEmbeddedAdminSecret(ctx, cli, provider, cfg)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(second.Data).To(Equal(original))

	stored := &corev1.Secret{}
	err = cli.Get(ctx, client.ObjectKey{
		Namespace: cfg.OperatorNamespace,
		Name:      dbcontroller.EmbeddedAdminSecretName(provider.Name),
	}, stored)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(stored.Data).To(Equal(original))
}

func TestEnsureEmbeddedAdminSecret_DoesNotRecoverMissingSecretForExistingInstance(t *testing.T) {
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
	sts := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: cfg.OperatorNamespace,
			Name:      dbcontroller.EmbeddedServiceName(provider.Name),
		},
	}

	cli := fake.NewClientBuilder().
		WithScheme(scheme).
		WithRuntimeObjects(provider, sts).
		Build()

	secret, err := ensureEmbeddedAdminSecret(ctx, cli, provider, cfg)
	g.Expect(err).To(MatchError(ContainSubstring("not found for an existing instance")))
	g.Expect(secret).To(BeNil())
}

func TestEnsureEmbeddedAdminSecret_DoesNotRecoverMissingSecretWhenPVCRemains(t *testing.T) {
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
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: cfg.OperatorNamespace,
			Name:      dbcontroller.EmbeddedPVCName(provider.Name),
		},
	}

	cli := fake.NewClientBuilder().
		WithScheme(scheme).
		WithRuntimeObjects(provider, pvc).
		Build()

	secret, err := ensureEmbeddedAdminSecret(ctx, cli, provider, cfg)
	g.Expect(err).To(MatchError(ContainSubstring("not found for an existing instance")))
	g.Expect(secret).To(BeNil())
}

func TestEnsureEmbeddedAdminSecret_DoesNotRecoverIncompleteSecretForExistingInstance(t *testing.T) {
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
	sts := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: cfg.OperatorNamespace,
			Name:      dbcontroller.EmbeddedServiceName(provider.Name),
		},
	}
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: cfg.OperatorNamespace,
			Name:      dbcontroller.EmbeddedAdminSecretName(provider.Name),
		},
		Data: map[string][]byte{
			dbcontroller.EmbeddedAdminSecretUserKey: []byte("postgres"),
		},
	}

	cli := fake.NewClientBuilder().
		WithScheme(scheme).
		WithRuntimeObjects(provider, sts, secret).
		Build()

	current, err := ensureEmbeddedAdminSecret(ctx, cli, provider, cfg)
	g.Expect(err).To(MatchError(ContainSubstring("is incomplete for an existing instance")))
	g.Expect(current).To(BeNil())
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

func TestEmbeddedImageChanged_MatchesCurrentStatefulSetImage(t *testing.T) {
	g := NewWithT(t)

	cfg := &moduleconfig.Config{
		Embedded: moduleconfig.EmbeddedConfig{
			PostgresImage: "postgres:test",
			PgvectorImage: "pgvector:test",
		},
	}
	provider := &infraApi.DatabaseProvider{
		ObjectMeta: metav1.ObjectMeta{Name: "embedded"},
		Spec: infraApi.DatabaseProviderSpec{
			Type: infraApi.ProviderTypeEmbedded,
			Embedded: &infraApi.EmbeddedProviderSpec{
				Extensions: []string{"vector"},
			},
		},
	}
	sts := &appsv1.StatefulSet{
		Spec: appsv1.StatefulSetSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Name: "postgres", Image: "pgvector:test"},
					},
				},
			},
		},
	}

	g.Expect(embeddedImageChanged(sts, provider, cfg)).To(BeFalse())
}

func TestEmbeddedImageChanged_DetectsDrift(t *testing.T) {
	g := NewWithT(t)

	cfg := &moduleconfig.Config{
		Embedded: moduleconfig.EmbeddedConfig{
			PostgresImage: "postgres:test",
			PgvectorImage: "pgvector:test",
		},
	}
	provider := &infraApi.DatabaseProvider{
		ObjectMeta: metav1.ObjectMeta{Name: "embedded"},
		Spec: infraApi.DatabaseProviderSpec{
			Type: infraApi.ProviderTypeEmbedded,
			Embedded: &infraApi.EmbeddedProviderSpec{
				Extensions: []string{"vector"},
			},
		},
	}
	sts := &appsv1.StatefulSet{
		Spec: appsv1.StatefulSetSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Name: "postgres", Image: "postgres:test"},
					},
				},
			},
		},
	}

	g.Expect(embeddedImageChanged(sts, provider, cfg)).To(BeTrue())
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

	builder := fake.NewClientBuilder().
		WithScheme(scheme).
		WithRuntimeObjects(alpha, bravo, claim)

	for _, idx := range dbcontroller.FieldIndexers {
		builder = builder.WithIndex(idx.Obj, idx.Field, idx.Fn)
	}

	cli := builder.Build()

	namespaces, err := referencedClaimNamespaces(ctx, cli, bravo)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(namespaces).To(Equal([]string{"workloads"}))
}
