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

package integration

import (
	"context"
	"testing"

	k8sm "github.com/lburgazzoli/gomega-matchers/pkg/matchers/k8s"
	"github.com/lburgazzoli/gomega-matchers/pkg/matchers/k8s/condition"
	. "github.com/onsi/gomega"
	"github.com/rs/xid"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"

	infraApi "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/api/infrastructure/v1alpha1"
	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/internal/controller/databaseprovider"
	dbcontroller "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/pkg/controller"
	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/pkg/postgres"
	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/pkg/resources/gvk"
	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/test/support"
)

type internalDatabaseProviderSuite struct {
	env *integrationEnv
}

func newInternalDatabaseProviderSuite(t *testing.T, env *integrationEnv) *internalDatabaseProviderSuite {
	t.Helper()

	return &internalDatabaseProviderSuite{env: env}
}

func (st *internalDatabaseProviderSuite) Run(t *testing.T) {
	t.Run("crd validation", st.testCRDValidation)
	t.Run("creates internal postgres resources", st.testProvisioning)
	t.Run("creates internal postgres resources in configured namespace", st.testProvisioningCustomNamespace)
	t.Run("creates internal postgres tls resources", st.testProvisioningTLS)
	t.Run("deleted admin secret is surfaced", st.testAdminSecretDeleted)
}

func (st *internalDatabaseProviderSuite) testCRDValidation(t *testing.T) {
	ctx := t.Context()
	cli := st.env.Client
	externalSpec := infraApi.ExternalProviderSpec{
		ConnectionSecretRef: corev1.SecretReference{Name: "admin-secret", Namespace: st.env.Namespace},
	}
	internalSpec := infraApi.InternalProviderSpec{
		Storage: infraApi.StorageSpec{Size: resource.MustParse("1Gi")},
	}

	t.Run("rejects-type-mismatch-internal", func(t *testing.T) {
		g := NewWithT(t)
		obj := &infraApi.DatabaseProvider{
			ObjectMeta: metav1.ObjectMeta{Name: "dbp-type-mismatch-internal"},
			Spec: infraApi.DatabaseProviderSpec{
				Type:     infraApi.ProviderTypeInternal,
				External: &externalSpec,
			},
		}
		g.Expect(cli.Create(ctx, obj)).To(HaveOccurred())
	})

	t.Run("accepts-valid-internal", func(t *testing.T) {
		g := NewWithT(t)
		obj := &infraApi.DatabaseProvider{
			ObjectMeta: metav1.ObjectMeta{Name: "dbp-valid-internal"},
			Spec: infraApi.DatabaseProviderSpec{
				Type:     infraApi.ProviderTypeInternal,
				Internal: &internalSpec,
			},
		}
		g.Expect(cli.Create(ctx, obj)).To(Succeed())
		t.Cleanup(func() { _ = cli.Delete(ctx, obj) })
	})

	t.Run("rejects-invalid-extension-name", func(t *testing.T) {
		g := NewWithT(t)
		bad := internalSpec
		bad.Extensions = []string{"Not-Valid!"}
		obj := &infraApi.DatabaseProvider{
			ObjectMeta: metav1.ObjectMeta{Name: "dbp-bad-extension"},
			Spec: infraApi.DatabaseProviderSpec{
				Type:     infraApi.ProviderTypeInternal,
				Internal: &bad,
			},
		}
		g.Expect(cli.Create(ctx, obj)).To(HaveOccurred())
	})

	t.Run("rejects-unsupported-extension", func(t *testing.T) {
		g := NewWithT(t)
		bad := internalSpec
		bad.Extensions = []string{"postgis"}
		obj := &infraApi.DatabaseProvider{
			ObjectMeta: metav1.ObjectMeta{Name: "dbp-unsupported-extension"},
			Spec: infraApi.DatabaseProviderSpec{
				Type:     infraApi.ProviderTypeInternal,
				Internal: &bad,
			},
		}
		g.Expect(cli.Create(ctx, obj)).To(HaveOccurred())
	})

	t.Run("rejects-zero-storage-size", func(t *testing.T) {
		g := NewWithT(t)
		bad := internalSpec
		bad.Storage.Size = resource.MustParse("0")
		obj := &infraApi.DatabaseProvider{
			ObjectMeta: metav1.ObjectMeta{Name: "dbp-zero-storage"},
			Spec: infraApi.DatabaseProviderSpec{
				Type:     infraApi.ProviderTypeInternal,
				Internal: &bad,
			},
		}
		g.Expect(cli.Create(ctx, obj)).To(HaveOccurred())
	})
}

func (st *internalDatabaseProviderSuite) testProvisioning(t *testing.T) {
	provider := st.createInternalProvider(t, "internal-"+xid.New().String(), []string{"vector", "pg_trgm"})

	st.expectProvisionedInNamespace(t, provider, st.env.Config.OperatorNamespace)
}

func (st *internalDatabaseProviderSuite) testProvisioningCustomNamespace(t *testing.T) {
	namespace := "internal-" + xid.New().String()
	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}}
	NewWithT(t).Expect(st.env.Client.Create(t.Context(), ns)).To(Succeed())
	t.Cleanup(func() {
		if err := support.DeleteAndWait(context.Background(), st.env.Client, ns); err != nil {
			t.Errorf("deleting namespace: %v", err)
		}
	})

	provider := st.createInternalProvider(
		t,
		"internal-"+xid.New().String(),
		[]string{"vector", "pg_trgm"},
		withInternalNamespace(namespace),
	)

	st.expectProvisionedInNamespace(t, provider, namespace)
}

func (st *internalDatabaseProviderSuite) testProvisioningTLS(t *testing.T) {
	provider := st.createInternalProvider(
		t,
		"internal-"+xid.New().String(),
		[]string{"pg_trgm"},
		withInternalTLS(),
	)

	st.expectProvisionedTLSInNamespace(t, provider, st.env.Config.OperatorNamespace)
}

func (st *internalDatabaseProviderSuite) testAdminSecretDeleted(t *testing.T) {
	g := NewWithT(t)

	provider := st.createInternalProvider(t, "internal-"+xid.New().String(), []string{"pg_trgm"})
	st.waitReachable(t, provider)

	adminSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: st.env.Config.OperatorNamespace,
			Name:      dbcontroller.InternalAdminSecretName(provider.Name),
		},
	}
	g.Expect(st.env.Client.Get(t.Context(), client.ObjectKeyFromObject(adminSecret), adminSecret)).To(Succeed())
	initialHash := adminSecret.Annotations[dbcontroller.InternalAdminSecretKeyAnnotation]
	g.Expect(initialHash).NotTo(BeEmpty())

	g.Expect(support.DeleteAndWait(t.Context(), st.env.Client, adminSecret)).To(Succeed())

	// The controller detects the missing Secret, regenerates credentials with a
	// fresh instance hash, and recreates the Secret. The new hash must differ
	// from the original so the StatefulSet rolls to pick up the new credentials.
	refreshed := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{
		Namespace: st.env.Config.OperatorNamespace,
		Name:      dbcontroller.InternalAdminSecretName(provider.Name),
	}}
	g.Eventually(t.Context(), k8sm.Get(st.env.Client, refreshed)).Should(
		WithTransform(func(s *corev1.Secret) string {
			return s.Annotations[dbcontroller.InternalAdminSecretKeyAnnotation]
		}, SatisfyAll(
			Not(BeEmpty()),
			Not(Equal(initialHash)),
		)),
	)
}

func (st *internalDatabaseProviderSuite) createInternalProvider(
	t *testing.T,
	name string,
	extensions []string,
	opts ...func(*infraApi.DatabaseProvider),
) *infraApi.DatabaseProvider {
	t.Helper()

	provider := &infraApi.DatabaseProvider{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec: infraApi.DatabaseProviderSpec{
			Type: infraApi.ProviderTypeInternal,
			Internal: &infraApi.InternalProviderSpec{
				Storage: infraApi.StorageSpec{
					Size: resource.MustParse("1Gi"),
				},
				Extensions: extensions,
			},
		},
	}
	for _, opt := range opts {
		opt(provider)
	}

	NewWithT(t).Expect(st.env.Client.Create(t.Context(), provider)).To(Succeed())
	t.Cleanup(func() {
		if err := support.DeleteAndWait(context.Background(), st.env.Client, provider); err != nil {
			t.Errorf("deleting provider: %v", err)
		}
	})

	return provider
}

func (st *internalDatabaseProviderSuite) expectProvisionedInNamespace(
	t *testing.T,
	provider *infraApi.DatabaseProvider,
	namespace string,
) {
	t.Helper()

	g := NewWithT(t)
	st.waitReachable(t, provider)

	statefulSet := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      dbcontroller.InternalServiceName(provider.Name),
		},
	}
	g.Eventually(t.Context(), k8sm.Lookup(st.env.Client, statefulSet)).To(Succeed())
	g.Expect(statefulSet.Status.ReadyReplicas).To(Equal(int32(1)))

	adminSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      dbcontroller.InternalAdminSecretName(provider.Name),
		},
	}
	g.Eventually(t.Context(), k8sm.Lookup(st.env.Client, adminSecret)).To(Succeed())
	g.Expect(adminSecret.Data).To(HaveKey(postgres.SecretKeyPassword))

	networkPolicy := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      dbcontroller.InternalServiceName(provider.Name),
		},
	}
	g.Eventually(t.Context(), k8sm.Lookup(st.env.Client, networkPolicy)).To(Succeed())

}

func (st *internalDatabaseProviderSuite) expectProvisionedTLSInNamespace(
	t *testing.T,
	provider *infraApi.DatabaseProvider,
	namespace string,
) {
	t.Helper()

	g := NewWithT(t)
	st.expectProvisionedInNamespace(t, provider, namespace)

	issuer := &unstructured.Unstructured{}
	issuer.SetGroupVersionKind(gvk.CertManagerIssuer)
	issuer.SetNamespace(namespace)
	issuer.SetName(dbcontroller.InternalTLSIssuerName(provider.Name))
	g.Eventually(t.Context(), k8sm.Lookup(st.env.Client, issuer)).To(Succeed())

	certificate := &unstructured.Unstructured{}
	certificate.SetGroupVersionKind(gvk.CertManagerCertificate)
	certificate.SetNamespace(namespace)
	certificate.SetName(dbcontroller.InternalTLSCertificateName(provider.Name))
	g.Eventually(t.Context(), k8sm.Lookup(st.env.Client, certificate)).To(Succeed())

	tlsSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      dbcontroller.InternalTLSSecretName(provider.Name),
		},
	}
	g.Eventually(t.Context(), k8sm.Lookup(st.env.Client, tlsSecret)).To(Succeed())
	g.Eventually(func(g Gomega) {
		g.Expect(st.env.Client.Get(t.Context(), client.ObjectKeyFromObject(tlsSecret), tlsSecret)).To(Succeed())
		g.Expect(tlsSecret.Data["tls.crt"]).NotTo(BeEmpty())
		g.Expect(tlsSecret.Data["tls.key"]).NotTo(BeEmpty())
		g.Expect(tlsSecret.Data["ca.crt"]).NotTo(BeEmpty())
	}).WithContext(t.Context()).Should(Succeed())

	adminSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      dbcontroller.InternalAdminSecretName(provider.Name),
		},
	}
	g.Eventually(func(g Gomega) {
		g.Expect(st.env.Client.Get(t.Context(), client.ObjectKeyFromObject(adminSecret), adminSecret)).To(Succeed())
		g.Expect(adminSecret.Data[postgres.SecretKeySSLMode]).To(Equal([]byte(postgres.SSLModeVerifyFull)))
		g.Expect(adminSecret.Data[postgres.SecretKeyCA]).NotTo(BeEmpty())
	}).WithContext(t.Context()).Should(Succeed())

	statefulSet := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      dbcontroller.InternalServiceName(provider.Name),
		},
	}
	g.Eventually(func(g Gomega) {
		g.Expect(st.env.Client.Get(t.Context(), client.ObjectKeyFromObject(statefulSet), statefulSet)).To(Succeed())
		g.Expect(statefulSet.Spec.Template.Spec.Containers).NotTo(BeEmpty())
		g.Expect(statefulSet.Spec.Template.Spec.Containers[0].Args).To(ContainElements(
			"ssl=on",
			"ssl_cert_file=/var/lib/postgresql/tls/tls.crt",
			"ssl_key_file=/var/lib/postgresql/tls/tls.key",
		))
	}).WithContext(t.Context()).Should(Succeed())

	g.Eventually(func(g Gomega) {
		g.Expect(st.env.Client.Get(t.Context(), client.ObjectKeyFromObject(provider), provider)).To(Succeed())
		g.Expect(provider.Status.TLS).NotTo(BeNil())
		g.Expect(provider.Status.TLS.Enabled).To(BeTrue())
		g.Expect(provider.Status.TLS.Ready).To(BeTrue())
		g.Expect(provider.Status.TLS.SecretName).To(Equal(dbcontroller.InternalTLSSecretName(provider.Name)))
		g.Expect(provider.Status.TLS.CertificateName).To(Equal(dbcontroller.InternalTLSCertificateName(provider.Name)))
		g.Expect(provider.Status.TLS.IssuerRef).NotTo(BeNil())
		g.Expect(provider.Status.TLS.IssuerRef.Name).To(Equal(dbcontroller.InternalTLSIssuerName(provider.Name)))
		g.Expect(provider.Status.Conditions).To(ContainElement(
			condition.Is(databaseprovider.ConditionTLSConfiguration, metav1.ConditionTrue),
		))
	}).WithContext(t.Context()).Should(Succeed())
}

func withInternalNamespace(namespace string) func(*infraApi.DatabaseProvider) {
	return func(provider *infraApi.DatabaseProvider) {
		provider.Spec.Internal.Namespace = namespace
	}
}

func withInternalTLS() func(*infraApi.DatabaseProvider) {
	return func(provider *infraApi.DatabaseProvider) {
		provider.Spec.Internal.TLS = &infraApi.InternalProviderTLSSpec{}
	}
}

func (st *internalDatabaseProviderSuite) waitReachable(t *testing.T, provider *infraApi.DatabaseProvider) {
	t.Helper()

	NewWithT(t).Eventually(t.Context(), k8sm.Get(st.env.Client, provider)).Should(
		WithTransform(k8sm.ConditionsOf[metav1.Condition](),
			ContainElement(condition.Is(databaseprovider.ConditionReachable, metav1.ConditionTrue))),
	)
}
