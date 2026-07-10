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
	"crypto/sha256"
	"fmt"
	"maps"
	"regexp"
	"testing"

	k8sm "github.com/lburgazzoli/gomega-matchers/pkg/matchers/k8s"
	"github.com/lburgazzoli/gomega-matchers/pkg/matchers/k8s/condition"
	. "github.com/onsi/gomega"
	"github.com/rs/xid"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	infraApi "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/api/infrastructure/v1alpha1"
	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/internal/controller/databaseclaim"
	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/pkg/postgres"
)

const maxDatabaseClaimRoleLen = 63

var databaseClaimNonIdentRe = regexp.MustCompile(`[^a-z0-9_]`)

type databaseClaimSuite struct {
	env          *integrationEnv
	db           *testDatabase
	providerName string
}

func newDatabaseClaimSuite(t *testing.T) (*databaseClaimSuite, error) {
	t.Helper()

	env, err := newIntegrationEnv(t)
	if err != nil {
		return nil, err
	}

	suite := &databaseClaimSuite{
		env:          env,
		providerName: "database-provider-" + xid.New().String(),
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

		if err := suite.db.Close(context.Background()); err != nil {
			t.Errorf("closing database: %v", err)
		}
	})

	if err := suite.createProvider(t); err != nil {
		return nil, err
	}

	return suite, nil
}

func (st *databaseClaimSuite) createProvider(t *testing.T) error {
	return st.createExternalProvider(t, st.providerName, st.db.cfg, nil, nil)
}

func (st *databaseClaimSuite) createExternalProvider(
	t *testing.T,
	name string,
	cfg postgres.Config,
	labels map[string]string,
	annotations map[string]string,
) error {
	t.Helper()

	mergedAnnotations := map[string]string{
		"db.infrastructure.opendatahub.io/operator-namespace": st.env.Namespace,
	}
	maps.Copy(mergedAnnotations, annotations)

	adminSecretData := map[string]string{
		postgres.SecretKeyHost:     cfg.Host,
		postgres.SecretKeyPort:     fmt.Sprintf("%d", cfg.Port),
		postgres.SecretKeyUser:     cfg.User,
		postgres.SecretKeyPassword: cfg.Password,
		postgres.SecretKeyDatabase: cfg.DBName,
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
	if err := st.env.Client.Create(t.Context(), adminSecret); err != nil {
		return err
	}
	t.Cleanup(func() {
		st.env.deleteAndWait(context.Background(), t, adminSecret)
	})

	provider := &infraApi.DatabaseProvider{
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Labels:      labels,
			Annotations: mergedAnnotations,
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
	if err := st.env.Client.Create(t.Context(), provider); err != nil {
		return err
	}
	t.Cleanup(func() {
		st.env.deleteAndWait(context.Background(), t, provider)
	})
	return nil
}

func (st *databaseClaimSuite) createDatabase(t *testing.T, name string) {
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

func (st *databaseClaimSuite) roleExists(t *testing.T, role string) bool {
	t.Helper()

	g := NewWithT(t)
	pool, err := st.db.openAdminPool(t.Context())
	g.Expect(err).NotTo(HaveOccurred())
	t.Cleanup(pool.Close)

	exists, err := postgres.RoleExists(t.Context(), pool, role)
	g.Expect(err).NotTo(HaveOccurred())
	return exists
}

func (st *databaseClaimSuite) databaseExists(t *testing.T, name string) bool {
	t.Helper()

	g := NewWithT(t)
	pool, err := st.db.openAdminPool(t.Context())
	g.Expect(err).NotTo(HaveOccurred())
	t.Cleanup(pool.Close)

	exists, err := postgres.DatabaseExists(t.Context(), pool, name)
	g.Expect(err).NotTo(HaveOccurred())
	return exists
}

func (st *databaseClaimSuite) newClaim(name string, database string) *infraApi.DatabaseClaim {
	return &infraApi.DatabaseClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: st.env.Namespace,
		},
		Spec: infraApi.DatabaseClaimSpec{
			Provider: infraApi.ProviderRef{
				Name: st.providerName,
			},
			Database: database,
		},
	}
}

func (st *databaseClaimSuite) createClaim(t *testing.T, claim *infraApi.DatabaseClaim) {
	t.Helper()

	g := NewWithT(t)
	g.Expect(st.env.Client.Create(t.Context(), claim)).To(Succeed())

	t.Cleanup(func() {
		st.env.deleteAndWait(context.Background(), t, claim)
	})
}

func (st *databaseClaimSuite) createClaimWithoutCleanup(t *testing.T, claim *infraApi.DatabaseClaim) {
	t.Helper()
	NewWithT(t).Expect(st.env.Client.Create(t.Context(), claim)).To(Succeed())
}

func (st *databaseClaimSuite) deleteClaimAndWait(t *testing.T, claim *infraApi.DatabaseClaim) {
	t.Helper()

	g := NewWithT(t)
	g.Expect(st.env.Client.Delete(t.Context(), claim)).To(Succeed())
	g.Eventually(t.Context(), k8sm.NotFound(st.env.Client, claim)).Should(BeTrue())
}

func (st *databaseClaimSuite) waitProvisioned(t *testing.T, claim *infraApi.DatabaseClaim) {
	t.Helper()
	NewWithT(t).Eventually(t.Context(), k8sm.Get(st.env.Client, claim)).Should(
		WithTransform(k8sm.ConditionsOf[metav1.Condition](),
			ContainElement(condition.Is(databaseclaim.ConditionProvisioned, metav1.ConditionTrue))),
	)
}

func (st *databaseClaimSuite) waitProvisioningFailure(
	t *testing.T,
	claim *infraApi.DatabaseClaim,
	reason string,
	messageSubstring string,
) {
	t.Helper()
	NewWithT(t).Eventually(t.Context(), k8sm.Get(st.env.Client, claim)).Should(
		WithTransform(k8sm.ConditionsOf[metav1.Condition](), ContainElement(
			SatisfyAll(
				HaveField("Type", Equal(databaseclaim.ConditionProvisioned)),
				HaveField("Status", Equal(metav1.ConditionFalse)),
				HaveField("Reason", Equal(reason)),
				HaveField("Message", ContainSubstring(messageSubstring)),
			),
		)),
	)
}

func (st *databaseClaimSuite) waitCredentialsSecret(
	t *testing.T,
	name string,
	database string,
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
				HaveKeyWithValue(postgres.SecretKeyDatabase, []byte(database)),
			),
		),
	)

	g.Eventually(t.Context(), k8sm.Lookup(st.env.Client, secret)).To(Succeed())
	return secret
}

func (st *databaseClaimSuite) expectNoCredentialsSecret(t *testing.T, name string) {
	t.Helper()

	g := NewWithT(t)
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: st.env.Namespace},
	}

	g.Consistently(t.Context(), k8sm.NotFound(st.env.Client, secret), "2s", "200ms").Should(BeTrue())
	g.Eventually(t.Context(), k8sm.Absent(st.env.Client, secret), "2s", "200ms").Should(BeTrue())
}

func (st *databaseClaimSuite) dropRole(t *testing.T, role string, database string) {
	t.Helper()

	g := NewWithT(t)
	pool, err := st.db.openAdminPool(t.Context())
	g.Expect(err).NotTo(HaveOccurred())
	t.Cleanup(pool.Close)

	g.Expect(postgres.RevokeDatabasePrivileges(t.Context(), pool, database, role)).To(Succeed())
	g.Expect(postgres.DropRole(t.Context(), pool, role)).To(Succeed())
}

func (st *databaseClaimSuite) triggerReconcile(t *testing.T, claim *infraApi.DatabaseClaim) {
	t.Helper()

	g := NewWithT(t)
	current := &infraApi.DatabaseClaim{}
	g.Expect(st.env.Client.Get(t.Context(), client.ObjectKeyFromObject(claim), current)).To(Succeed())

	if current.Annotations == nil {
		current.Annotations = map[string]string{}
	}
	current.Annotations["test/trigger"] = xid.New().String()
	g.Expect(st.env.Client.Update(t.Context(), current)).To(Succeed())
}

func databaseClaimRoleName(namespace string, name string) string {
	raw := fmt.Sprintf("%s_%s", namespace, name)
	safe := databaseClaimNonIdentRe.ReplaceAllString(raw, "_")
	if len(safe) > maxDatabaseClaimRoleLen {
		h := fmt.Sprintf("%x", sha256.Sum256([]byte(safe)))[:8]
		safe = safe[:maxDatabaseClaimRoleLen-9] + "_" + h
	}
	return safe
}
