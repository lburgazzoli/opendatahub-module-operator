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
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	infraApi "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/api/infrastructure/v1alpha1"
	moduleconfig "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/pkg/config"
	dbcontroller "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/pkg/controller"
	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/pkg/postgres"
	pginstance "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/pkg/postgres/instance"
	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/pkg/resources/gvk"
	common "github.com/opendatahub-io/odh-platform-utilities/api/common"
)

func TestResolveInternalImage(t *testing.T) {
	cfg := &moduleconfig.Config{
		Internal: moduleconfig.InternalConfig{
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
					Type: infraApi.ProviderTypeInternal,
					Internal: &infraApi.InternalProviderSpec{
						Extensions: tc.extensions,
					},
				},
			}

			image, err := resolveInternalImage(provider, cfg)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(image).To(Equal(tc.wantImage))
		})
	}
}

func TestComputeInternalAdminSecret_GeneratesCredentialsOnFirstReconcile(t *testing.T) {
	g := NewWithT(t)
	ctx := context.Background()

	scheme := runtime.NewScheme()
	g.Expect(appsv1.AddToScheme(scheme)).To(Succeed())
	g.Expect(corev1.AddToScheme(scheme)).To(Succeed())
	g.Expect(infraApi.AddToScheme(scheme)).To(Succeed())

	cfg := &moduleconfig.Config{OperatorNamespace: "operator-ns"}
	provider := &infraApi.DatabaseProvider{
		ObjectMeta: metav1.ObjectMeta{Name: "internal"},
		Spec: infraApi.DatabaseProviderSpec{
			Type:     infraApi.ProviderTypeInternal,
			Internal: &infraApi.InternalProviderSpec{},
		},
	}

	cli := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(provider).Build()

	tlsState, err := computeInternalAdminSecret(ctx, cli, provider, cfg)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(tlsState.Enabled).To(BeFalse())
	g.Expect(tlsState.AdminSecret).NotTo(BeNil())
	g.Expect(tlsState.AdminSecret.Data).To(SatisfyAll(
		HaveKey(postgres.SecretKeyHost),
		HaveKey(postgres.SecretKeyUser),
		HaveKey(postgres.SecretKeyPassword),
		HaveKey(postgres.SecretKeyDatabase),
	))
}

func TestComputeInternalAdminSecret_PreservesExistingPassword(t *testing.T) {
	g := NewWithT(t)
	ctx := context.Background()

	scheme := runtime.NewScheme()
	g.Expect(appsv1.AddToScheme(scheme)).To(Succeed())
	g.Expect(corev1.AddToScheme(scheme)).To(Succeed())
	g.Expect(infraApi.AddToScheme(scheme)).To(Succeed())

	cfg := &moduleconfig.Config{OperatorNamespace: "operator-ns"}
	provider := &infraApi.DatabaseProvider{
		ObjectMeta: metav1.ObjectMeta{Name: "internal"},
		Spec: infraApi.DatabaseProviderSpec{
			Type:     infraApi.ProviderTypeInternal,
			Internal: &infraApi.InternalProviderSpec{},
		},
	}
	// Simulate deploy action having already created the Secret.
	existing := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: cfg.OperatorNamespace,
			Name:      dbcontroller.InternalAdminSecretName(provider.Name),
		},
		Data: map[string][]byte{
			postgres.SecretKeyHost:     []byte("internal.operator-ns.svc"),
			postgres.SecretKeyPort:     []byte("5432"),
			postgres.SecretKeyUser:     []byte("postgres"),
			postgres.SecretKeyPassword: []byte("stable-password"),
			postgres.SecretKeyDatabase: []byte("postgres"),
		},
	}

	cli := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(provider, existing).Build()

	first, err := computeInternalAdminSecret(ctx, cli, provider, cfg)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(first.Enabled).To(BeFalse())
	g.Expect(first.AdminSecret).NotTo(BeNil())
	g.Expect(first.AdminSecret.Data[postgres.SecretKeyPassword]).To(Equal([]byte("stable-password")))

	second, err := computeInternalAdminSecret(ctx, cli, provider, cfg)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(second.Enabled).To(BeFalse())
	g.Expect(second.AdminSecret).NotTo(BeNil())
	g.Expect(second.AdminSecret.Data).To(Equal(first.AdminSecret.Data))
}

func TestComputeInternalAdminSecret_ProjectsResolvedTLSState(t *testing.T) {
	g := NewWithT(t)
	ctx := context.Background()

	scheme := runtime.NewScheme()
	g.Expect(appsv1.AddToScheme(scheme)).To(Succeed())
	g.Expect(corev1.AddToScheme(scheme)).To(Succeed())
	g.Expect(infraApi.AddToScheme(scheme)).To(Succeed())

	cfg := &moduleconfig.Config{OperatorNamespace: "operator-ns"}
	provider := &infraApi.DatabaseProvider{
		ObjectMeta: metav1.ObjectMeta{Name: "internal"},
		Spec: infraApi.DatabaseProviderSpec{
			Type: infraApi.ProviderTypeInternal,
			Internal: &infraApi.InternalProviderSpec{
				TLS: &infraApi.InternalProviderTLSSpec{},
			},
		},
	}
	tlsSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: cfg.OperatorNamespace,
			Name:      dbcontroller.InternalTLSSecretName(provider.Name),
		},
		Data: map[string][]byte{
			"ca.crt":  []byte("ca-bytes"),
			"tls.crt": []byte("server-cert"),
			"tls.key": []byte("server-key"),
		},
	}

	cli := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(provider, tlsSecret).Build()

	tlsState, err := computeInternalAdminSecret(ctx, cli, provider, cfg)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(tlsState.Enabled).To(BeTrue())
	g.Expect(tlsState.Ready).To(BeTrue())
	g.Expect(tlsState.CAData).To(Equal([]byte("ca-bytes")))
	g.Expect(tlsState.TLSSecretHash).NotTo(BeEmpty())
	g.Expect(tlsState.TLSSecret).NotTo(BeNil())
	g.Expect(tlsState.AdminSecret).NotTo(BeNil())
	g.Expect(tlsState.AdminSecret.Data[postgres.SecretKeySSLMode]).To(Equal([]byte(postgres.SSLModeVerifyFull)))
	g.Expect(tlsState.AdminSecret.Data[postgres.SecretKeyCA]).To(Equal([]byte("ca-bytes")))
}

func TestInternalNamespace_UsesOverride(t *testing.T) {
	g := NewWithT(t)

	cfg := &moduleconfig.Config{OperatorNamespace: "operator-ns"}
	provider := &infraApi.DatabaseProvider{
		ObjectMeta: metav1.ObjectMeta{Name: "internal"},
		Spec: infraApi.DatabaseProviderSpec{
			Type: infraApi.ProviderTypeInternal,
			Internal: &infraApi.InternalProviderSpec{
				Namespace: "custom-ns",
			},
		},
	}

	g.Expect(dbcontroller.InternalNamespace(provider, cfg.OperatorNamespace)).To(Equal("custom-ns"))
	g.Expect(dbcontroller.InternalServiceHost(provider, cfg.OperatorNamespace)).To(Equal("internal.custom-ns.svc"))
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

func TestResolveInternalData_MapsProviderToTypedData(t *testing.T) {
	g := NewWithT(t)
	ctx := context.Background()

	scheme := runtime.NewScheme()
	g.Expect(infraApi.AddToScheme(scheme)).To(Succeed())
	g.Expect(corev1.AddToScheme(scheme)).To(Succeed())

	storageClassName := "fast"
	provider := &infraApi.DatabaseProvider{
		ObjectMeta: metav1.ObjectMeta{Name: "internal"},
		Spec: infraApi.DatabaseProviderSpec{
			Type: infraApi.ProviderTypeInternal,
			Internal: &infraApi.InternalProviderSpec{
				Namespace:  "custom-ns",
				Storage:    infraApi.StorageSpec{Size: resource.MustParse("10Gi"), StorageClassName: &storageClassName},
				Resources:  corev1.ResourceRequirements{Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("250m")}},
				Extensions: []string{"vector", "pg_trgm"},
				TLS: &infraApi.InternalProviderTLSSpec{
					Certificate: infraApi.InternalProviderTLSCertificateSpec{
						SecretName:  "internal-tls",
						Duration:    &metav1.Duration{Duration: 24 * 60 * 60 * 1e9},
						RenewBefore: &metav1.Duration{Duration: 12 * 60 * 60 * 1e9},
					},
				},
			},
		},
	}
	claim := &infraApi.SchemaClaim{
		ObjectMeta: metav1.ObjectMeta{Name: "claim", Namespace: "workloads"},
		Status: infraApi.SchemaClaimStatus{
			Status: common.Status{
				Conditions: []common.Condition{{Type: "Provisioned", Status: metav1.ConditionTrue}},
			},
			Provider: provider.Name,
		},
	}

	cli := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(provider, claim).Build()

	data, err := resolveInternalData(ctx, cli, provider, &moduleconfig.Config{
		OperatorNamespace: "operator-ns",
		Internal: moduleconfig.InternalConfig{
			PostgresImage: "postgres:test",
			PgvectorImage: "pgvector:test",
		},
	}, TLSState{
		Enabled:       true,
		TLSSecretHash: "tls-hash",
		AdminSecret: &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{
					dbcontroller.InternalAdminSecretKeyAnnotation: "instance-hash",
				},
			},
		},
	})
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(data.Namespace).To(Equal("custom-ns"))
	g.Expect(data.Service.Name).To(Equal(dbcontroller.InternalServiceName(provider.Name)))
	g.Expect(data.PVC.Name).To(Equal(dbcontroller.InternalPVCName(provider.Name)))
	g.Expect(data.PVC.Size).To(Equal("10Gi"))
	g.Expect(data.PVC.StorageClassName).To(Equal("fast"))
	g.Expect(data.InitDB.Extensions).To(Equal([]string{"vector", "pg_trgm"}))
	g.Expect(data.Postgres.Image).To(Equal("pgvector:test"))
	g.Expect(data.Postgres.AdminSecretName).To(Equal(dbcontroller.InternalAdminSecretName(provider.Name)))
	g.Expect(data.Postgres.InstanceHash).To(Equal("instance-hash"))
	g.Expect(data.Network.AllowedNamespaces).To(Equal([]string{"workloads"}))
	g.Expect(data.TLS.Enabled).To(BeTrue())
	g.Expect(data.TLS.UsesManagedIssuer).To(BeTrue())
	g.Expect(data.TLS.SecretName).To(Equal("internal-tls"))
	g.Expect(data.TLS.SecretHash).To(Equal("tls-hash"))
	g.Expect(data.TLS.IssuerName).To(Equal(dbcontroller.InternalTLSIssuerName(provider.Name)))
	g.Expect(data.TLS.Certificate.Name).To(Equal(dbcontroller.InternalTLSCertificateName(provider.Name)))
	g.Expect(data.TLS.IssuerRef).NotTo(BeNil())
}

func TestRenderEmbeddedResources_BuildsNonTLSObjects(t *testing.T) {
	g := NewWithT(t)

	data := pginstance.Data{
		Namespace:    "operator-ns",
		ProviderName: "embedded",
		Service:      pginstance.Service{Name: "embedded"},
		PVC: pginstance.PVC{
			Name: "embedded-pvc",
			Size: "5Gi",
		},
		InitDB: pginstance.InitDB{
			ConfigMapName: "embedded-initdb",
			Extensions:    []string{"pg_trgm", "pgcrypto"},
		},
		Postgres: pginstance.Postgres{
			Image:           "postgres:test",
			AdminSecretName: "embedded-admin",
			InstanceHash:    "instance-hash",
		},
		Network: pginstance.NetworkPolicy{
			AllowedNamespaces: []string{"workloads"},
		},
	}

	resources, err := pginstance.Resources(context.Background(), data)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(resources).To(HaveLen(5))

	pvc := lookupRenderedResource(g, resources, schema.GroupVersionKind{Version: "v1", Kind: "PersistentVolumeClaim"}, "embedded-pvc")
	g.Expect(pvc.GetNamespace()).To(Equal("operator-ns"))

	svc := lookupRenderedResource(g, resources, schema.GroupVersionKind{Version: "v1", Kind: "Service"}, "embedded")
	g.Expect(svc.Object["spec"]).To(HaveKeyWithValue("clusterIP", corev1.ClusterIPNone))

	initdb := lookupRenderedResource(g, resources, schema.GroupVersionKind{Version: "v1", Kind: "ConfigMap"}, "embedded-initdb")
	g.Expect(initdb.Object["data"]).To(HaveKeyWithValue(
		"00-init-extensions.sql",
		"CREATE EXTENSION IF NOT EXISTS pg_trgm;\nCREATE EXTENSION IF NOT EXISTS pgcrypto;\n",
	))

	statefulSet := lookupRenderedResource(g, resources, appsv1.SchemeGroupVersion.WithKind("StatefulSet"), "embedded")
	annotations, found, err := unstructured.NestedStringMap(
		statefulSet.Object,
		"spec", "template", "metadata", "annotations",
	)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(found).To(BeTrue())
	g.Expect(annotations).To(HaveKeyWithValue(
		dbcontroller.InternalAdminSecretKeyAnnotation,
		"instance-hash",
	))

	networkPolicy := lookupRenderedResource(
		g,
		resources,
		networkingv1.SchemeGroupVersion.WithKind("NetworkPolicy"),
		"embedded",
	)
	ingress, found, err := unstructured.NestedSlice(networkPolicy.Object, "spec", "ingress")
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(found).To(BeTrue())
	g.Expect(ingress).To(HaveLen(1))

	rule, ok := ingress[0].(map[string]any)
	g.Expect(ok).To(BeTrue())
	g.Expect(rule).To(HaveKey("from"))
}

func TestRenderEmbeddedResources_BuildsTLSObjects(t *testing.T) {
	g := NewWithT(t)

	data := pginstance.Data{
		Namespace:    "operator-ns",
		ProviderName: "embedded",
		Service:      pginstance.Service{Name: "embedded"},
		PVC: pginstance.PVC{
			Name: "embedded-pvc",
			Size: "5Gi",
		},
		InitDB: pginstance.InitDB{
			ConfigMapName: "embedded-initdb",
		},
		Postgres: pginstance.Postgres{
			Image:           "postgres:test",
			AdminSecretName: "embedded-admin",
			InstanceHash:    "instance-hash",
		},
		TLS: pginstance.TLS{
			Enabled:           true,
			UsesManagedIssuer: true,
			SecretName:        "embedded-tls",
			SecretHash:        "tls-hash",
			IssuerName:        "embedded-issuer",
			IssuerRef: &infraApi.CertManagerIssuerRef{
				Name:  "embedded-issuer",
				Kind:  gvk.CertManagerIssuer.Kind,
				Group: gvk.CertManagerIssuer.Group,
			},
			Certificate: pginstance.Certificate{
				Name: "embedded-cert",
			},
		},
	}

	resources, err := pginstance.Resources(context.Background(), data)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(resources).To(HaveLen(7))

	issuer := lookupRenderedResource(g, resources, gvk.CertManagerIssuer, "embedded-issuer")
	g.Expect(issuer.GetKind()).To(Equal(gvk.CertManagerIssuer.Kind))

	certificate := lookupRenderedResource(g, resources, gvk.CertManagerCertificate, "embedded-cert")
	g.Expect(certificate.GetKind()).To(Equal(gvk.CertManagerCertificate.Kind))

	statefulSet := lookupRenderedResource(g, resources, appsv1.SchemeGroupVersion.WithKind("StatefulSet"), "embedded")
	annotations, found, err := unstructured.NestedStringMap(
		statefulSet.Object,
		"spec", "template", "metadata", "annotations",
	)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(found).To(BeTrue())
	g.Expect(annotations).To(HaveKeyWithValue(
		"db.infrastructure.opendatahub.io/tls-secret-hash",
		"tls-hash",
	))

	templateSpec, found, err := unstructured.NestedMap(statefulSet.Object, "spec", "template", "spec")
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(found).To(BeTrue())
	g.Expect(templateSpec).To(HaveKey("securityContext"))

	containers, found, err := unstructured.NestedSlice(statefulSet.Object, "spec", "template", "spec", "containers")
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(found).To(BeTrue())
	g.Expect(containers).To(HaveLen(1))

	container, ok := containers[0].(map[string]any)
	g.Expect(ok).To(BeTrue())
	g.Expect(container).To(HaveKeyWithValue("args", []any{
		"-c",
		"ssl=on",
		"-c",
		"ssl_cert_file=/var/lib/postgresql/tls/tls.crt",
		"-c",
		"ssl_key_file=/var/lib/postgresql/tls/tls.key",
	}))

	tlsMountFound := false
	if volumeMounts, ok := container["volumeMounts"].([]any); ok {
		for _, mount := range volumeMounts {
			mountMap, ok := mount.(map[string]any)
			if ok && mountMap["name"] == "tls" {
				tlsMountFound = true
				break
			}
		}
	}
	g.Expect(tlsMountFound).To(BeTrue())

	volumes, found, err := unstructured.NestedSlice(statefulSet.Object, "spec", "template", "spec", "volumes")
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(found).To(BeTrue())
	g.Expect(volumes).To(HaveLen(3))
}

func lookupRenderedResource(
	g Gomega,
	resources []unstructured.Unstructured,
	targetGVK schema.GroupVersionKind,
	name string,
) unstructured.Unstructured {
	g.Expect(resources).NotTo(BeEmpty())
	for _, obj := range resources {
		if obj.GroupVersionKind() == targetGVK && obj.GetName() == name {
			return obj
		}
	}

	g.Expect(false).To(BeTrue(), "resource %s %s not found", targetGVK.String(), name)
	return unstructured.Unstructured{}
}
