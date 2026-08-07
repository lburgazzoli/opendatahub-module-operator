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
	"fmt"
	"maps"
	"testing"

	k8sm "github.com/lburgazzoli/gomega-matchers/pkg/matchers/k8s"
	"github.com/lburgazzoli/gomega-matchers/pkg/matchers/k8s/condition"
	infraApi "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/api/infrastructure/v1alpha1"
	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/internal/controller/schemaclaim"
	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/pkg/postgres"
	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/test/support"
	testdb "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/test/support/db"
	. "github.com/onsi/gomega"
	"github.com/rs/xid"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type schemaClaimSuite struct {
	env          *integrationEnv
	db           *testdb.Instance
	databaseName string
	providerName string
}

func newSchemaClaimSuite(t *testing.T, env *integrationEnv) (*schemaClaimSuite, error) {
	t.Helper()

	suite := &schemaClaimSuite{
		env:          env,
		db:           env.Database,
		databaseName: "schema_" + xid.New().String(),
		providerName: "schema-provider-" + xid.New().String(),
	}
	if suite.db == nil {
		return nil, fmt.Errorf("integration database is nil")
	}

	suite.createDatabase(t, suite.databaseName)
	suite.createProvider(t)

	return suite, nil
}

func (st *schemaClaimSuite) createDatabase(t *testing.T, name string) {
	t.Helper()

	g := NewWithT(t)
	pgClient := st.db.Client()
	g.Expect(pgClient).NotTo(BeNil())

	exists, err := postgres.DatabaseExists(t.Context(), pgClient, name)
	g.Expect(err).NotTo(HaveOccurred())
	if exists {
		return
	}

	_, err = pgClient.Exec(t.Context(), fmt.Sprintf("CREATE DATABASE %s", postgres.QuoteIdentifier(name)))
	g.Expect(err).NotTo(HaveOccurred())
}

func (st *schemaClaimSuite) openProviderAdminClient(ctx context.Context) (postgres.Client, error) {
	cfg := st.db.Config()
	cfg.DBName = st.databaseName

	pgClient, err := st.env.ClientFactory(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("opening provider admin client: %w", err)
	}

	return pgClient, nil
}

func (st *schemaClaimSuite) createProvider(t *testing.T) {
	st.createExternalProvider(
		t,
		st.providerName,
		st.db.Config(),
		st.databaseName,
		[]infraApi.ExternalCapability{infraApi.ExternalCapabilityCreateSchema},
		nil,
		nil,
	)
}

func (st *schemaClaimSuite) createExternalProvider(
	t *testing.T,
	name string,
	cfg postgres.Config,
	database string,
	capabilities []infraApi.ExternalCapability,
	labels map[string]string,
	annotations map[string]string,
) {
	t.Helper()

	g := NewWithT(t)

	mergedAnnotations := map[string]string{
		"db.infrastructure.opendatahub.io/operator-namespace": st.env.Namespace,
	}
	maps.Copy(mergedAnnotations, annotations)

	adminSecretData := map[string]string{
		postgres.SecretKeyHost:     cfg.Host,
		postgres.SecretKeyPort:     fmt.Sprintf("%d", cfg.Port),
		postgres.SecretKeyUser:     cfg.User,
		postgres.SecretKeyPassword: cfg.Password,
		postgres.SecretKeyDatabase: database,
	}
	if cfg.SSLMode != "" {
		adminSecretData[postgres.SecretKeySSLMode] = cfg.SSLMode
	}
	adminSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name + "-admin",
			Namespace: st.env.Namespace,
		},
		StringData: adminSecretData,
	}
	g.Expect(st.env.Client.Create(t.Context(), adminSecret)).To(Succeed())
	t.Cleanup(func() {
		if err := support.DeleteAndWait(context.Background(), st.env.Client, adminSecret); err != nil {
			t.Errorf("deleting admin secret: %v", err)
		}
	})

	provider := &infraApi.DatabaseProvider{
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Labels:      labels,
			Annotations: mergedAnnotations,
		},
		Spec: infraApi.DatabaseProviderSpec{
			Type:            infraApi.ProviderTypeExternal,
			DefaultDatabase: database,
			External: &infraApi.ExternalProviderSpec{
				ConnectionSecretRef: corev1.SecretReference{
					Name:      adminSecret.Name,
					Namespace: st.env.Namespace,
				},
				Capabilities: capabilities,
			},
		},
	}
	g.Expect(st.env.Client.Create(t.Context(), provider)).To(Succeed())
	t.Cleanup(func() {
		if err := support.DeleteAndWait(context.Background(), st.env.Client, provider); err != nil {
			t.Errorf("deleting provider: %v", err)
		}
	})
}

func (st *schemaClaimSuite) newClaim(name string) *infraApi.SchemaClaim {
	return &infraApi.SchemaClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: st.env.Namespace,
		},
		Spec: infraApi.SchemaClaimSpec{
			Provider: infraApi.ProviderRef{
				Name: st.providerName,
			},
		},
	}
}

func (st *schemaClaimSuite) newSelectorClaim(name string, selector map[string]string) *infraApi.SchemaClaim {
	return &infraApi.SchemaClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: st.env.Namespace,
		},
		Spec: infraApi.SchemaClaimSpec{
			Provider: infraApi.ProviderRef{
				Selector: &metav1.LabelSelector{MatchLabels: selector},
			},
		},
	}
}

func (st *schemaClaimSuite) createClaim(t *testing.T, claim *infraApi.SchemaClaim) {
	t.Helper()
	g := NewWithT(t)
	g.Expect(st.env.Client.Create(t.Context(), claim)).To(Succeed())

	t.Cleanup(func() {
		if err := support.DeleteAndWait(context.Background(), st.env.Client, claim); err != nil {
			t.Errorf("deleting claim: %v", err)
		}
	})
}

func (st *schemaClaimSuite) waitProvisioned(t *testing.T, claim *infraApi.SchemaClaim) {
	t.Helper()
	NewWithT(t).Eventually(t.Context(), k8sm.Get(st.env.Client, claim)).Should(
		WithTransform(k8sm.ConditionsOf[metav1.Condition](),
			ContainElement(condition.Is(schemaclaim.ConditionProvisioned, metav1.ConditionTrue))),
	)
}

func (st *schemaClaimSuite) waitSelectedProvider(
	t *testing.T,
	claim *infraApi.SchemaClaim,
	provider string,
) {
	t.Helper()
	NewWithT(t).Eventually(t.Context(), k8sm.Get(st.env.Client, claim)).Should(
		WithTransform(func(obj *infraApi.SchemaClaim) string {
			return obj.Status.Provider
		}, Equal(provider)),
	)
}

func (st *schemaClaimSuite) waitProvisioningFailure(
	t *testing.T,
	claim *infraApi.SchemaClaim,
	reason string,
) {
	t.Helper()
	NewWithT(t).Eventually(t.Context(), k8sm.Get(st.env.Client, claim)).Should(
		WithTransform(k8sm.ConditionsOf[metav1.Condition](), ContainElement(
			SatisfyAll(
				HaveField("Type", Equal(schemaclaim.ConditionProvisioned)),
				HaveField("Status", Equal(metav1.ConditionFalse)),
				HaveField("Reason", Equal(reason)),
			),
		)),
	)
}

func (st *schemaClaimSuite) waitCredentialsSecret(t *testing.T, name string) *corev1.Secret {
	return st.waitCredentialsSecretForDatabase(t, name, st.databaseName)
}

func (st *schemaClaimSuite) waitCredentialsSecretForDatabase(
	t *testing.T,
	name string,
	expectedDatabase string,
) *corev1.Secret {
	t.Helper()

	g := NewWithT(t)

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: st.env.Namespace},
	}

	g.Eventually(t.Context(), k8sm.Get(st.env.Client, secret)).Should(
		WithTransform(k8sm.Data(),
			SatisfyAll(
				HaveKey(postgres.SecretKeyHost),
				HaveKey(postgres.SecretKeyUser),
				HaveKey(postgres.SecretKeyPassword),
				HaveKeyWithValue(postgres.SecretKeyDatabase, []byte(expectedDatabase)),
				HaveKey(postgres.SecretKeySchema),
			),
		),
	)

	g.Eventually(t.Context(), k8sm.Lookup(st.env.Client, secret)).To(Succeed())

	return secret
}

func (st *schemaClaimSuite) expectNoCredentialsSecret(t *testing.T, name string) {
	t.Helper()

	g := NewWithT(t)

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: st.env.Namespace},
	}

	g.Consistently(t.Context(), k8sm.NotFound(st.env.Client, secret), "2s", "200ms").Should(BeTrue())
	g.Eventually(t.Context(), k8sm.Absent(st.env.Client, secret), "2s", "200ms").Should(BeTrue())
}

func (st *schemaClaimSuite) roleExists(t *testing.T, role string) bool {
	t.Helper()

	g := NewWithT(t)
	pgClient := st.db.Client()
	g.Expect(pgClient).NotTo(BeNil())

	exists, err := postgres.RoleExists(t.Context(), pgClient, role)
	g.Expect(err).NotTo(HaveOccurred())
	return exists
}

func (st *schemaClaimSuite) schemaExists(t *testing.T, schema string) bool {
	t.Helper()

	g := NewWithT(t)
	pgClient, err := st.openProviderAdminClient(t.Context())
	g.Expect(err).NotTo(HaveOccurred())
	t.Cleanup(pgClient.Close)

	exists, err := postgres.SchemaExists(t.Context(), pgClient, schema)
	g.Expect(err).NotTo(HaveOccurred())
	return exists
}

func (st *schemaClaimSuite) dropRole(t *testing.T, role string, schema string) {
	t.Helper()

	g := NewWithT(t)
	pgClient, err := st.openProviderAdminClient(t.Context())
	g.Expect(err).NotTo(HaveOccurred())
	t.Cleanup(pgClient.Close)

	g.Expect(postgres.RevokeSchemaPrivileges(t.Context(), pgClient, schema, role)).To(Succeed())
	g.Expect(postgres.DropRole(t.Context(), pgClient, role)).To(Succeed())
}

func (st *schemaClaimSuite) dropSchema(t *testing.T, schema string) {
	t.Helper()

	g := NewWithT(t)
	pgClient, err := st.openProviderAdminClient(t.Context())
	g.Expect(err).NotTo(HaveOccurred())
	t.Cleanup(pgClient.Close)

	g.Expect(postgres.DropSchemaCascade(t.Context(), pgClient, schema)).To(Succeed())
}

func (st *schemaClaimSuite) triggerReconcile(t *testing.T, claim *infraApi.SchemaClaim) {
	t.Helper()

	g := NewWithT(t)
	current := &infraApi.SchemaClaim{}
	g.Expect(st.env.Client.Get(t.Context(), client.ObjectKeyFromObject(claim), current)).To(Succeed())

	if current.Annotations == nil {
		current.Annotations = map[string]string{}
	}
	current.Annotations["test/trigger"] = xid.New().String()
	g.Expect(st.env.Client.Update(t.Context(), current)).To(Succeed())
}
