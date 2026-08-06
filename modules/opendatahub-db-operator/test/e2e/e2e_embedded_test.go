package e2e

import (
	"context"
	"testing"

	k8sm "github.com/lburgazzoli/gomega-matchers/pkg/matchers/k8s"
	"github.com/lburgazzoli/gomega-matchers/pkg/matchers/k8s/condition"
	. "github.com/onsi/gomega"
	"github.com/rs/xid"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	infraApi "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/api/infrastructure/v1alpha1"
	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/internal/controller/databaseclaim"
	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/internal/controller/databaseprovider"
	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/internal/controller/schemaclaim"
	dbcontroller "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/pkg/controller"
	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/pkg/postgres"
)

func (st *e2eSuite) runInternal(t *testing.T) {
	t.Run("internal provider serves schema and database claims together", st.testInternalProviderServesClaims)
	t.Run("internal provider propagates tls credentials to claims", st.testInternalProviderServesTLSClaims)
}

func (st *e2eSuite) testInternalProviderServesClaims(t *testing.T) {
	g := NewWithT(t)
	st.ensureWorkloadNamespace(t)

	provider := st.createInternalProvider(t)
	st.waitProviderReachable(t, provider)

	schemaClaim := &infraApi.SchemaClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "e2e-sc-" + xid.New().String(),
			Namespace: st.workloadNamespace,
		},
		Spec: infraApi.SchemaClaimSpec{
			Provider: infraApi.ProviderRef{Name: provider.Name},
		},
	}
	databaseClaim := &infraApi.DatabaseClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "e2e-dc-" + xid.New().String(),
			Namespace: st.workloadNamespace,
		},
		Spec: infraApi.DatabaseClaimSpec{
			Provider: infraApi.ProviderRef{Name: provider.Name},
			Database: "postgres",
		},
	}
	g.Expect(st.Client.Create(t.Context(), schemaClaim)).To(Succeed())
	t.Cleanup(func() {
		st.deleteAndWait(context.Background(), t, schemaClaim)
	})
	g.Expect(st.Client.Create(t.Context(), databaseClaim)).To(Succeed())
	t.Cleanup(func() {
		st.deleteAndWait(context.Background(), t, databaseClaim)
	})

	g.Eventually(t.Context(), k8sm.Get(st.Client, schemaClaim)).Should(
		WithTransform(k8sm.ConditionsOf[metav1.Condition](),
			ContainElement(condition.Is(schemaclaim.ConditionProvisioned, metav1.ConditionTrue))),
	)
	g.Eventually(t.Context(), k8sm.Get(st.Client, databaseClaim)).Should(
		WithTransform(k8sm.ConditionsOf[metav1.Condition](),
			ContainElement(condition.Is(databaseclaim.ConditionProvisioned, metav1.ConditionTrue))),
	)

	schemaSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: schemaClaim.Name, Namespace: st.workloadNamespace},
	}
	databaseSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: databaseClaim.Name, Namespace: st.workloadNamespace},
	}
	g.Eventually(t.Context(), k8sm.Get(st.Client, schemaSecret)).Should(
		WithTransform(k8sm.Data(), SatisfyAll(
			HaveKey(postgres.SecretKeyHost),
			HaveKey(postgres.SecretKeyUser),
			HaveKey(postgres.SecretKeyPassword),
			HaveKeyWithValue(postgres.SecretKeyDatabase, []byte("postgres")),
			HaveKey(postgres.SecretKeySchema),
		)),
	)
	g.Eventually(t.Context(), k8sm.Get(st.Client, databaseSecret)).Should(
		WithTransform(k8sm.Data(), SatisfyAll(
			HaveKey(postgres.SecretKeyHost),
			HaveKey(postgres.SecretKeyUser),
			HaveKey(postgres.SecretKeyPassword),
			HaveKeyWithValue(postgres.SecretKeyDatabase, []byte("postgres")),
		)),
	)
	g.Eventually(t.Context(), k8sm.Lookup(st.Client, schemaSecret)).To(Succeed())
	g.Eventually(t.Context(), k8sm.Lookup(st.Client, databaseSecret)).To(Succeed())

	schemaCfg, err := postgres.ParseSecret(schemaSecret.Data)
	g.Expect(err).NotTo(HaveOccurred())
	databaseCfg, err := postgres.ParseSecret(databaseSecret.Data)
	g.Expect(err).NotTo(HaveOccurred())

	expectedHost := dbcontroller.InternalServiceHost(provider, st.operatorNamespace)
	g.Expect(schemaCfg.Host).To(Equal(expectedHost))
	g.Expect(databaseCfg.Host).To(Equal(expectedHost))
	g.Expect(schemaCfg.User).NotTo(Equal(databaseCfg.User))
}

func (st *e2eSuite) testInternalProviderServesTLSClaims(t *testing.T) {
	g := NewWithT(t)
	st.ensureWorkloadNamespace(t)

	provider := st.createInternalProvider(t, func(provider *infraApi.DatabaseProvider) {
		provider.Spec.Internal.TLS = &infraApi.InternalProviderTLSSpec{}
	})
	st.waitProviderReachable(t, provider)

	g.Eventually(func(g Gomega) {
		g.Expect(st.Client.Get(t.Context(), client.ObjectKeyFromObject(provider), provider)).To(Succeed())
		g.Expect(provider.Status.Conditions).To(SatisfyAll(
			ContainElement(condition.Is(databaseprovider.ConditionReachable, metav1.ConditionTrue)),
			ContainElement(condition.Is(databaseprovider.ConditionTLSConfiguration, metav1.ConditionTrue)),
		))
		g.Expect(provider.Status.TLS).NotTo(BeNil())
		g.Expect(provider.Status.TLS.Enabled).To(BeTrue())
		g.Expect(provider.Status.TLS.Ready).To(BeTrue())
		g.Expect(provider.Status.TLS.SecretName).To(Equal(dbcontroller.InternalTLSSecretName(provider.Name)))
		g.Expect(provider.Status.TLS.CertificateName).To(Equal(dbcontroller.InternalTLSCertificateName(provider.Name)))
		g.Expect(provider.Status.TLS.IssuerRef).NotTo(BeNil())
		g.Expect(provider.Status.TLS.IssuerRef.Name).To(Equal(dbcontroller.InternalTLSIssuerName(provider.Name)))
	}).WithContext(t.Context()).Should(Succeed())

	schemaClaim := &infraApi.SchemaClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "e2e-sc-tls-" + xid.New().String(),
			Namespace: st.workloadNamespace,
		},
		Spec: infraApi.SchemaClaimSpec{
			Provider: infraApi.ProviderRef{Name: provider.Name},
		},
	}
	databaseClaim := &infraApi.DatabaseClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "e2e-dc-tls-" + xid.New().String(),
			Namespace: st.workloadNamespace,
		},
		Spec: infraApi.DatabaseClaimSpec{
			Provider: infraApi.ProviderRef{Name: provider.Name},
			Database: "postgres",
		},
	}
	g.Expect(st.Client.Create(t.Context(), schemaClaim)).To(Succeed())
	t.Cleanup(func() {
		st.deleteAndWait(context.Background(), t, schemaClaim)
	})
	g.Expect(st.Client.Create(t.Context(), databaseClaim)).To(Succeed())
	t.Cleanup(func() {
		st.deleteAndWait(context.Background(), t, databaseClaim)
	})

	g.Eventually(t.Context(), k8sm.Get(st.Client, schemaClaim)).Should(
		WithTransform(k8sm.ConditionsOf[metav1.Condition](), SatisfyAll(
			ContainElement(condition.Is(schemaclaim.ConditionProvisioned, metav1.ConditionTrue)),
			ContainElement(condition.Is(schemaclaim.ConditionTLSConfiguration, metav1.ConditionTrue)),
		)),
	)
	g.Eventually(t.Context(), k8sm.Get(st.Client, databaseClaim)).Should(
		WithTransform(k8sm.ConditionsOf[metav1.Condition](), SatisfyAll(
			ContainElement(condition.Is(databaseclaim.ConditionProvisioned, metav1.ConditionTrue)),
			ContainElement(condition.Is(databaseclaim.ConditionTLSConfiguration, metav1.ConditionTrue)),
		)),
	)

	schemaSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: schemaClaim.Name, Namespace: st.workloadNamespace},
	}
	databaseSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: databaseClaim.Name, Namespace: st.workloadNamespace},
	}
	g.Eventually(t.Context(), k8sm.Get(st.Client, schemaSecret)).Should(
		WithTransform(k8sm.Data(), SatisfyAll(
			HaveKeyWithValue(postgres.SecretKeySSLMode, []byte(postgres.SSLModeVerifyFull)),
			HaveKey(postgres.SecretKeyCA),
			HaveKey(postgres.SecretKeySchema),
		)),
	)
	g.Eventually(t.Context(), k8sm.Get(st.Client, databaseSecret)).Should(
		WithTransform(k8sm.Data(), SatisfyAll(
			HaveKeyWithValue(postgres.SecretKeySSLMode, []byte(postgres.SSLModeVerifyFull)),
			HaveKey(postgres.SecretKeyCA),
		)),
	)
	g.Eventually(t.Context(), k8sm.Lookup(st.Client, schemaSecret)).To(Succeed())
	g.Eventually(t.Context(), k8sm.Lookup(st.Client, databaseSecret)).To(Succeed())

	schemaCfg, err := postgres.ParseSecret(schemaSecret.Data)
	g.Expect(err).NotTo(HaveOccurred())
	databaseCfg, err := postgres.ParseSecret(databaseSecret.Data)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(schemaCfg.TLSEnabled()).To(BeTrue())
	g.Expect(schemaCfg.TLSReady()).To(BeTrue())
	g.Expect(databaseCfg.TLSEnabled()).To(BeTrue())
	g.Expect(databaseCfg.TLSReady()).To(BeTrue())
}

func (st *e2eSuite) createInternalProvider(
	t *testing.T,
	opts ...func(*infraApi.DatabaseProvider),
) *infraApi.DatabaseProvider {
	t.Helper()

	g := NewWithT(t)

	provider := &infraApi.DatabaseProvider{
		ObjectMeta: metav1.ObjectMeta{Name: "e2e-internal-" + xid.New().String()},
		Spec: infraApi.DatabaseProviderSpec{
			Type: infraApi.ProviderTypeInternal,
			Internal: &infraApi.InternalProviderSpec{
				Storage: infraApi.StorageSpec{
					Size: resource.MustParse("1Gi"),
				},
				Extensions: []string{"pg_trgm"},
			},
		},
	}
	for _, opt := range opts {
		opt(provider)
	}

	g.Expect(st.Client.Create(t.Context(), provider)).To(Succeed())
	t.Cleanup(func() {
		st.deleteAndWait(context.Background(), t, provider)
	})

	return provider
}
