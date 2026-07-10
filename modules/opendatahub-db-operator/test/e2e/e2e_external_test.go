package e2e

import (
	"context"
	"fmt"
	"testing"

	k8sm "github.com/lburgazzoli/gomega-matchers/pkg/matchers/k8s"
	"github.com/lburgazzoli/gomega-matchers/pkg/matchers/k8s/condition"
	. "github.com/onsi/gomega"
	"github.com/rs/xid"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	infraApi "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/api/infrastructure/v1alpha1"
	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/internal/controller/databaseclaim"
	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/internal/controller/schemaclaim"
	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/pkg/postgres"
)

const (
	externalPostgresUser     = "admin"
	externalPostgresPassword = "adminpass"
	externalPostgresDB       = "appdb"
)

func TestDatabaseOperatorE2EExternal(t *testing.T) {
	suite := newE2ESuite(t)

	t.Run("external provider serves schema and database claims together", suite.testExternalProviderServesClaims)
	t.Run("missing provider is surfaced on claims", suite.testMissingProvider)
}

func (st *e2eSuite) testExternalProviderServesClaims(t *testing.T) {
	g := NewWithT(t)
	st.ensureWorkloadNamespace(t)

	provider, expectedHost := st.createExternalProviderWithDatabase(t)
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
			Database: externalPostgresDB,
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
			HaveKeyWithValue(postgres.SecretKeyDatabase, []byte(externalPostgresDB)),
			HaveKey(postgres.SecretKeySchema),
		)),
	)
	g.Eventually(t.Context(), k8sm.Get(st.Client, databaseSecret)).Should(
		WithTransform(k8sm.Data(), SatisfyAll(
			HaveKey(postgres.SecretKeyHost),
			HaveKey(postgres.SecretKeyUser),
			HaveKey(postgres.SecretKeyPassword),
			HaveKeyWithValue(postgres.SecretKeyDatabase, []byte(externalPostgresDB)),
		)),
	)
	g.Eventually(t.Context(), k8sm.Lookup(st.Client, schemaSecret)).To(Succeed())
	g.Eventually(t.Context(), k8sm.Lookup(st.Client, databaseSecret)).To(Succeed())

	schemaCfg, err := postgres.ParseSecret(schemaSecret.Data)
	g.Expect(err).NotTo(HaveOccurred())
	databaseCfg, err := postgres.ParseSecret(databaseSecret.Data)
	g.Expect(err).NotTo(HaveOccurred())

	g.Expect(schemaCfg.Host).To(Equal(expectedHost))
	g.Expect(databaseCfg.Host).To(Equal(expectedHost))
	g.Expect(schemaCfg.DBName).To(Equal(externalPostgresDB))
	g.Expect(databaseCfg.DBName).To(Equal(externalPostgresDB))
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

func (st *e2eSuite) createExternalProviderWithDatabase(t *testing.T) (*infraApi.DatabaseProvider, string) {
	t.Helper()

	name := "e2e-external-" + xid.New().String()
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: st.operatorNamespace,
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": name},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"app": name},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name:  "postgres",
						Image: "postgres:16",
						Env: []corev1.EnvVar{
							{Name: "POSTGRES_USER", Value: externalPostgresUser},
							{Name: "POSTGRES_PASSWORD", Value: externalPostgresPassword},
							{Name: "POSTGRES_DB", Value: externalPostgresDB},
						},
						Ports: []corev1.ContainerPort{{
							ContainerPort: 5432,
							Name:          "postgres",
						}},
					}},
				},
			},
		},
	}
	NewWithT(t).Expect(st.Client.Create(t.Context(), deployment)).To(Succeed())
	t.Cleanup(func() {
		st.deleteAndWait(context.Background(), t, deployment)
	})

	g := NewWithT(t)
	g.Eventually(t.Context(), k8sm.Get(st.Client, deployment)).Should(
		WithTransform(func(dep *appsv1.Deployment) int32 {
			return dep.Status.ReadyReplicas
		}, BeNumerically(">=", 1)),
	)

	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: st.operatorNamespace,
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{"app": name},
			Ports: []corev1.ServicePort{{
				Name: "postgres",
				Port: 5432,
			}},
		},
	}
	g.Expect(st.Client.Create(t.Context(), service)).To(Succeed())
	t.Cleanup(func() {
		st.deleteAndWait(context.Background(), t, service)
	})

	host := fmt.Sprintf("%s.%s.svc.cluster.local", service.Name, service.Namespace)
	adminSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name + "-admin",
			Namespace: st.operatorNamespace,
		},
		StringData: map[string]string{
			postgres.SecretKeyHost:     host,
			postgres.SecretKeyPort:     fmt.Sprintf("%d", postgres.DefaultPort),
			postgres.SecretKeyUser:     externalPostgresUser,
			postgres.SecretKeyPassword: externalPostgresPassword,
			postgres.SecretKeyDatabase: externalPostgresDB,
			postgres.SecretKeySSLMode:  postgres.SSLModeDisable,
		},
	}
	g.Expect(st.Client.Create(t.Context(), adminSecret)).To(Succeed())
	t.Cleanup(func() {
		st.deleteAndWait(context.Background(), t, adminSecret)
	})

	provider := &infraApi.DatabaseProvider{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec: infraApi.DatabaseProviderSpec{
			Type: infraApi.ProviderTypeExternal,
			External: &infraApi.ExternalProviderSpec{
				ConnectionSecretRef: corev1.SecretReference{
					Name:      adminSecret.Name,
					Namespace: adminSecret.Namespace,
				},
			},
		},
	}
	g.Expect(st.Client.Create(t.Context(), provider)).To(Succeed())
	t.Cleanup(func() {
		st.deleteAndWait(context.Background(), t, provider)
	})

	return provider, host
}
