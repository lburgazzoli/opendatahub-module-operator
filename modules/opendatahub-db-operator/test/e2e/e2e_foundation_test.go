package e2e

import (
	"context"
	"os"
	"testing"

	k8sm "github.com/lburgazzoli/gomega-matchers/pkg/matchers/k8s"
	"github.com/lburgazzoli/gomega-matchers/pkg/matchers/k8s/condition"
	. "github.com/onsi/gomega"
	"github.com/rs/xid"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	infraApi "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/api/infrastructure/v1alpha1"
	servicesApi "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/api/services/v1alpha1"
	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/internal/controller/databaseclaim"
	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/internal/controller/databaseprovider"
	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/internal/controller/schemaclaim"
	dbcontroller "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/pkg/controller"
	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/pkg/postgres"
	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/test/support"
)

const defaultE2ETestNamespace = "odh-db-operator-e2e"

type e2eSuite struct {
	Client            client.Client
	operatorNamespace string
	workloadNamespace string
}

func e2eTestNamespace() string {
	if namespace := os.Getenv("E2E_TEST_NAMESPACE"); namespace != "" {
		return namespace
	}

	return defaultE2ETestNamespace
}

func (st *e2eSuite) testFoundation(t *testing.T) {
	t.Run("installs module CRDs", st.testCRDsInstalled)
	t.Run("deploys operator config", st.testOperatorConfigMap)
}

func (st *e2eSuite) testWorkflows(t *testing.T) {
	t.Run("embedded provider serves schema and database claims together", st.testEmbeddedProviderServesClaims)
	t.Run("missing provider is surfaced on claims", st.testMissingProvider)
	t.Run("deleted embedded admin secret is surfaced", st.testDeletedAdminSecret)
}

func (st *e2eSuite) testCRDsInstalled(t *testing.T) {
	g := NewWithT(t)

	for _, name := range []string{
		infraApi.SchemaClaimResource + "." + infraApi.GroupName,
		infraApi.DatabaseClaimResource + "." + infraApi.GroupName,
		infraApi.DatabaseProviderResource + "." + infraApi.GroupName,
		servicesApi.DatabaseServiceCRDName,
	} {
		crd := &apiextensionsv1.CustomResourceDefinition{
			ObjectMeta: metav1.ObjectMeta{Name: name},
		}
		g.Eventually(t.Context(), k8sm.Lookup(st.Client, crd)).Should(Succeed())
	}
}

func (st *e2eSuite) testOperatorConfigMap(t *testing.T) {
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      operatorConfigMapName,
			Namespace: st.operatorNamespace,
		},
	}

	NewWithT(t).Eventually(t.Context(), k8sm.Get(st.Client, cm)).Should(
		WithTransform(k8sm.Data(), SatisfyAll(
			HaveKey("platformType"),
			HaveKey("platformVersion"),
		)),
	)
}

func (st *e2eSuite) testEmbeddedProviderServesClaims(t *testing.T) {
	g := NewWithT(t)
	st.ensureWorkloadNamespace(t)

	provider := st.createEmbeddedProvider(t)
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

	expectedHost := dbcontroller.EmbeddedServiceHost(provider, st.operatorNamespace)
	g.Expect(schemaCfg.Host).To(Equal(expectedHost))
	g.Expect(databaseCfg.Host).To(Equal(expectedHost))
	g.Expect(schemaCfg.User).NotTo(Equal(databaseCfg.User))
}

func (st *e2eSuite) testMissingProvider(t *testing.T) {
	g := NewWithT(t)
	st.ensureWorkloadNamespace(t)

	claim := &infraApi.SchemaClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "e2e-sc-" + xid.New().String(),
			Namespace: st.workloadNamespace,
		},
		Spec: infraApi.SchemaClaimSpec{
			Provider: infraApi.ProviderRef{Name: "e2e-missing-provider"},
		},
	}
	g.Expect(st.Client.Create(t.Context(), claim)).To(Succeed())
	t.Cleanup(func() {
		st.deleteAndWait(context.Background(), t, claim)
	})

	g.Eventually(t.Context(), k8sm.Get(st.Client, claim)).Should(
		WithTransform(k8sm.ConditionsOf[metav1.Condition](), ContainElement(
			SatisfyAll(
				HaveField("Type", Equal(schemaclaim.ConditionProvisioned)),
				HaveField("Status", Equal(metav1.ConditionFalse)),
				HaveField("Reason", Equal("ProviderNotFound")),
			),
		)),
	)
}

func (st *e2eSuite) testDeletedAdminSecret(t *testing.T) {
	provider := st.createEmbeddedProvider(t)
	st.waitProviderReachable(t, provider)

	adminSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      dbcontroller.EmbeddedAdminSecretName(provider.Name),
			Namespace: st.operatorNamespace,
		},
	}
	st.deleteAndWait(t.Context(), t, adminSecret)

	st.expectProviderUnreachable(t, provider, "AdminSecretUnavailable", adminSecret.Name)
}

func (st *e2eSuite) ensureWorkloadNamespace(t *testing.T) {
	t.Helper()
	NewWithT(t).Expect(support.EnsureNamespace(t.Context(), st.Client, st.workloadNamespace)).To(Succeed())
}

func (st *e2eSuite) createEmbeddedProvider(t *testing.T) *infraApi.DatabaseProvider {
	t.Helper()

	provider := &infraApi.DatabaseProvider{
		ObjectMeta: metav1.ObjectMeta{Name: "e2e-embedded-" + xid.New().String()},
		Spec: infraApi.DatabaseProviderSpec{
			Type: infraApi.ProviderTypeEmbedded,
			Embedded: &infraApi.EmbeddedProviderSpec{
				Storage: infraApi.StorageSpec{
					Size: resource.MustParse("1Gi"),
				},
				Extensions: []string{"pg_trgm"},
			},
		},
	}

	NewWithT(t).Expect(st.Client.Create(t.Context(), provider)).To(Succeed())
	t.Cleanup(func() {
		st.deleteAndWait(context.Background(), t, provider)
	})

	return provider
}

func (st *e2eSuite) waitProviderReachable(t *testing.T, provider *infraApi.DatabaseProvider) {
	t.Helper()

	NewWithT(t).Eventually(t.Context(), k8sm.Get(st.Client, provider)).Should(
		WithTransform(k8sm.ConditionsOf[metav1.Condition](),
			ContainElement(condition.Is(databaseprovider.ConditionReachable, metav1.ConditionTrue))),
	)
}

func (st *e2eSuite) expectProviderUnreachable(
	t *testing.T,
	provider *infraApi.DatabaseProvider,
	reason string,
	messageSubstring string,
) {
	t.Helper()

	NewWithT(t).Eventually(t.Context(), k8sm.Get(st.Client, provider)).Should(
		WithTransform(k8sm.ConditionsOf[metav1.Condition](), ContainElement(
			SatisfyAll(
				HaveField("Type", Equal(databaseprovider.ConditionReachable)),
				HaveField("Status", Equal(metav1.ConditionFalse)),
				HaveField("Reason", Equal(reason)),
				HaveField("Message", ContainSubstring(messageSubstring)),
			),
		)),
	)
}

func (st *e2eSuite) deleteAndWait(ctx context.Context, t *testing.T, obj client.Object) {
	t.Helper()

	key := client.ObjectKeyFromObject(obj)
	if err := st.Client.Get(ctx, key, obj); err != nil {
		if apierrors.IsNotFound(err) {
			return
		}
		t.Fatalf("reading %s before delete: %v", key, err)
	}

	if err := st.Client.Delete(ctx, obj); err != nil && !apierrors.IsNotFound(err) {
		t.Fatalf("deleting %s: %v", key, err)
	}

	NewWithT(t).Eventually(ctx, k8sm.NotFound(st.Client, obj)).Should(BeTrue(), "waiting for %s to be deleted", key)
}
