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
	"testing"

	k8sm "github.com/lburgazzoli/gomega-matchers/pkg/matchers/k8s"
	"github.com/lburgazzoli/gomega-matchers/pkg/matchers/k8s/condition"
	infraApi "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/api/infrastructure/v1alpha1"
	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/internal/controller/schemaclaim"
	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/pkg/postgres"
	. "github.com/onsi/gomega"
	"github.com/rs/xid"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type schemaClaimSuite struct {
	env          *integrationEnv
	db           *testDatabase
	databaseName string
	providerName string
}

func newSchemaClaimSuite(t *testing.T) (*schemaClaimSuite, error) {
	t.Helper()

	suite := &schemaClaimSuite{
		env:          newIntegrationEnv(t),
		databaseName: "schema_" + xid.New().String(),
		providerName: "schema-provider-" + xid.New().String(),
	}

	db, err := startDatabase(t.Context())
	if err != nil {
		return nil, err
	}

	suite.db = db
	t.Cleanup(func() {
		if suite.db == nil {
			return
		}

		_ = suite.db.Close(context.Background())
	})

	suite.createDatabase(t, suite.databaseName)
	suite.createProvider(t)

	return suite, nil
}

func (st *schemaClaimSuite) createDatabase(t *testing.T, name string) {
	t.Helper()

	g := NewWithT(t)
	pool, err := st.db.openAdminPool(t.Context())
	g.Expect(err).NotTo(HaveOccurred())
	t.Cleanup(pool.Close)

	exists, err := postgres.DatabaseExists(t.Context(), pool, name)
	g.Expect(err).NotTo(HaveOccurred())
	if exists {
		return
	}

	_, err = pool.Exec(t.Context(), fmt.Sprintf("CREATE DATABASE %s", postgres.QuoteIdentifier(name)))
	g.Expect(err).NotTo(HaveOccurred())
}

func (st *schemaClaimSuite) createProvider(t *testing.T) {
	t.Helper()

	g := NewWithT(t)

	adminSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      st.providerName + "-admin",
			Namespace: st.env.Namespace,
		},
		StringData: map[string]string{
			postgres.SecretKeyHost:     st.db.cfg.Host,
			postgres.SecretKeyPort:     fmt.Sprintf("%d", st.db.cfg.Port),
			postgres.SecretKeyUser:     st.db.cfg.User,
			postgres.SecretKeyPassword: st.db.cfg.Password,
			postgres.SecretKeyDatabase: st.databaseName,
		},
	}
	g.Expect(st.env.Client.Create(t.Context(), adminSecret)).To(Succeed())
	t.Cleanup(func() {
		st.env.deleteAndWait(context.Background(), t, adminSecret)
	})

	provider := &infraApi.DatabaseProvider{
		ObjectMeta: metav1.ObjectMeta{
			Name: st.providerName,
			Annotations: map[string]string{
				"db.infrastructure.opendatahub.io/operator-namespace": st.env.Namespace,
			},
		},
		Spec: infraApi.DatabaseProviderSpec{
			Type: infraApi.ProviderTypeExternal,
			External: &infraApi.ExternalProviderSpec{
				ConnectionSecretRef: corev1.SecretReference{
					Name:      adminSecret.Name,
					Namespace: st.env.Namespace,
				},
			},
		},
	}
	g.Expect(st.env.Client.Create(t.Context(), provider)).To(Succeed())
	t.Cleanup(func() {
		st.env.deleteAndWait(context.Background(), t, provider)
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

func (st *schemaClaimSuite) createClaim(t *testing.T, claim *infraApi.SchemaClaim) {
	t.Helper()
	g := NewWithT(t)
	g.Expect(st.env.Client.Create(t.Context(), claim)).To(Succeed())

	t.Cleanup(func() {
		st.env.deleteAndWait(context.Background(), t, claim)
	})
}

func (st *schemaClaimSuite) waitProvisioned(t *testing.T, claim *infraApi.SchemaClaim) {
	t.Helper()
	NewWithT(t).Eventually(t.Context(), k8sm.Get(st.env.Client, claim)).Should(
		WithTransform(k8sm.ConditionsOf[metav1.Condition](),
			ContainElement(condition.Is(schemaclaim.ConditionProvisioned, metav1.ConditionTrue))),
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
				HaveKeyWithValue(postgres.SecretKeyDatabase, []byte(st.databaseName)),
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
