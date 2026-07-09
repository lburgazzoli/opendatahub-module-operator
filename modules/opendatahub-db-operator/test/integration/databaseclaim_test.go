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

	"github.com/lburgazzoli/gomega-matchers/pkg/matchers/jq"
	k8sm "github.com/lburgazzoli/gomega-matchers/pkg/matchers/k8s"
	. "github.com/onsi/gomega"
	"github.com/rs/xid"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	infraApi "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/api/infrastructure/v1alpha1"
	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/pkg/postgres"
)

func TestDatabaseClaim(t *testing.T) {
	g := NewWithT(t)

	suite, err := newDatabaseClaimSuite(t)
	g.Expect(err).ToNot(HaveOccurred())

	t.Run("database claim suite", suite.Run)
}

func (st *databaseClaimSuite) Run(t *testing.T) {
	t.Run("crd validation", st.testCRDValidation)
	t.Run("provisioning", st.testProvisioning)
	t.Run("idempotency", st.testIdempotency)
	t.Run("secret deletion rotates credentials", st.testSecretDeletionRecovery)
	t.Run("role deletion rotates credentials", st.testRoleDeletionRecovery)
	t.Run("database not found", st.testDatabaseNotFound)
	t.Run("deletion keeps peer user and database", st.testDeletionKeepsPeerUserAndDatabase)
	t.Run("provider not found", st.testProviderNotFound)
	t.Run("provider creation wakes pending claim", st.testProviderCreationWakesPendingClaim)
}

func (st *databaseClaimSuite) testProvisioning(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	database := "app_" + xid.New().String()
	st.createDatabase(t, database)

	name := "dc-" + xid.New().String()
	claim := st.newClaim(name, database)
	st.createClaim(t, claim)

	st.waitProvisioned(t, claim)

	g.Eventually(ctx, k8sm.Get(st.env.Client, claim)).Should(
		jq.Matchf(`.status.database == %q`, database),
	)

	secret := st.waitCredentialsSecret(t, name, database)
	credCfg, err := postgres.ParseSecret(secret.Data)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(credCfg.DBName).To(Equal(database))
	g.Expect(postgres.Ping(ctx, credCfg)).To(Succeed())
}

func (st *databaseClaimSuite) testIdempotency(t *testing.T) {
	g := NewWithT(t)

	database := "app_" + xid.New().String()
	st.createDatabase(t, database)

	name := "dc-" + xid.New().String()
	claim := st.newClaim(name, database)
	st.createClaim(t, claim)

	st.waitProvisioned(t, claim)

	secret := st.waitCredentialsSecret(t, name, database)
	firstPw := string(secret.Data[postgres.SecretKeyPassword])
	g.Expect(firstPw).NotTo(BeEmpty())

	g.Eventually(t.Context(), k8sm.Update(
		st.env.Client,
		claim,
		k8sm.SetAnnotation("test/trigger", "1"),
	)).ShouldNot(BeNil())

	g.Consistently(t.Context(), k8sm.Get(st.env.Client, secret), "5s", "500ms").Should(
		WithTransform(k8sm.Data(), HaveKeyWithValue(postgres.SecretKeyPassword, []byte(firstPw))),
	)
}

func (st *databaseClaimSuite) testSecretDeletionRecovery(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	database := "app_" + xid.New().String()
	st.createDatabase(t, database)

	name := "dc-" + xid.New().String()
	claim := st.newClaim(name, database)
	st.createClaim(t, claim)
	st.waitProvisioned(t, claim)

	secret := st.waitCredentialsSecret(t, name, database)
	oldCfg, err := postgres.ParseSecret(secret.Data)
	g.Expect(err).NotTo(HaveOccurred())
	oldPassword := string(secret.Data[postgres.SecretKeyPassword])

	st.env.deleteAndWait(ctx, t, secret)
	st.triggerReconcile(t, claim)

	recovered := st.waitCredentialsSecret(t, name, database)
	g.Expect(string(recovered.Data[postgres.SecretKeyPassword])).NotTo(Equal(oldPassword))

	newCfg, err := postgres.ParseSecret(recovered.Data)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(postgres.Ping(ctx, newCfg)).To(Succeed())
	g.Expect(postgres.Ping(ctx, oldCfg)).To(HaveOccurred())
}

func (st *databaseClaimSuite) testRoleDeletionRecovery(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	database := "app_" + xid.New().String()
	st.createDatabase(t, database)

	name := "dc-" + xid.New().String()
	claim := st.newClaim(name, database)
	st.createClaim(t, claim)
	st.waitProvisioned(t, claim)

	secret := st.waitCredentialsSecret(t, name, database)
	oldCfg, err := postgres.ParseSecret(secret.Data)
	g.Expect(err).NotTo(HaveOccurred())
	oldPassword := string(secret.Data[postgres.SecretKeyPassword])

	st.dropRole(t, oldCfg.User, database)
	g.Expect(st.roleExists(t, oldCfg.User)).To(BeFalse())

	st.triggerReconcile(t, claim)

	g.Eventually(ctx, func() string {
		fresh := st.waitCredentialsSecret(t, name, database)
		return string(fresh.Data[postgres.SecretKeyPassword])
	}).ShouldNot(Equal(oldPassword))
	recovered := st.waitCredentialsSecret(t, name, database)

	newCfg, err := postgres.ParseSecret(recovered.Data)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(postgres.Ping(ctx, newCfg)).To(Succeed())
	g.Expect(postgres.Ping(ctx, oldCfg)).To(HaveOccurred())
}

func (st *databaseClaimSuite) testDatabaseNotFound(t *testing.T) {
	g := NewWithT(t)

	database := "missing_" + xid.New().String()
	name := "dc-" + xid.New().String()
	claim := st.newClaim(name, database)
	st.createClaim(t, claim)

	st.waitProvisioningFailure(t, claim, "DatabaseNotFound", database)
	st.expectNoCredentialsSecret(t, name)
	g.Expect(st.roleExists(t, databaseClaimRoleName(st.env.Namespace, name))).To(BeFalse())
}

func (st *databaseClaimSuite) testDeletionKeepsPeerUserAndDatabase(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	database := "shared_" + xid.New().String()
	st.createDatabase(t, database)

	nameA := "dc-" + xid.New().String()
	claimA := st.newClaim(nameA, database)
	st.createClaimWithoutCleanup(t, claimA)

	nameB := "dc-" + xid.New().String()
	claimB := st.newClaim(nameB, database)
	st.createClaimWithoutCleanup(t, claimB)
	t.Cleanup(func() {
		st.env.deleteAndWait(context.Background(), t, claimB)
	})

	st.waitProvisioned(t, claimA)
	st.waitProvisioned(t, claimB)

	secretA := st.waitCredentialsSecret(t, nameA, database)
	secretB := st.waitCredentialsSecret(t, nameB, database)

	cfgA, err := postgres.ParseSecret(secretA.Data)
	g.Expect(err).NotTo(HaveOccurred())
	cfgB, err := postgres.ParseSecret(secretB.Data)
	g.Expect(err).NotTo(HaveOccurred())

	g.Expect(cfgA.User).NotTo(Equal(cfgB.User))
	g.Expect(postgres.Ping(ctx, cfgA)).To(Succeed())
	g.Expect(postgres.Ping(ctx, cfgB)).To(Succeed())

	st.deleteClaimAndWait(t, claimA)
	st.expectNoCredentialsSecret(t, nameA)

	g.Expect(st.databaseExists(t, database)).To(BeTrue())
	g.Expect(st.roleExists(t, databaseClaimRoleName(st.env.Namespace, nameA))).To(BeFalse())
	g.Expect(st.roleExists(t, databaseClaimRoleName(st.env.Namespace, nameB))).To(BeTrue())
	g.Expect(postgres.Ping(ctx, cfgB)).To(Succeed())
}

func (st *databaseClaimSuite) testProviderNotFound(t *testing.T) {
	database := "app_" + xid.New().String()
	st.createDatabase(t, database)

	name := "dc-" + xid.New().String()
	claim := st.newClaim(name, database)
	claim.Spec.Provider = infraApi.ProviderRef{Name: "does-not-exist"}
	st.createClaim(t, claim)

	st.waitProvisioningFailure(t, claim, "ProviderNotFound", "does-not-exist")
	st.expectNoCredentialsSecret(t, name)
}

func (st *databaseClaimSuite) testProviderCreationWakesPendingClaim(t *testing.T) {
	g := NewWithT(t)

	database := "app_" + xid.New().String()
	st.createDatabase(t, database)

	providerName := "provider-" + xid.New().String()
	name := "dc-" + xid.New().String()
	claim := st.newClaim(name, database)
	claim.Spec.Provider = infraApi.ProviderRef{Name: providerName}
	st.createClaim(t, claim)

	st.waitProvisioningFailure(t, claim, "ProviderNotFound", providerName)
	st.expectNoCredentialsSecret(t, name)

	err := st.createExternalProvider(t, providerName, st.db.cfg, nil, nil)
	g.Expect(err).NotTo(HaveOccurred())

	st.waitProvisioned(t, claim)
	g.Eventually(t.Context(), k8sm.Get(st.env.Client, claim)).Should(
		WithTransform(k8sm.ConditionsOf[metav1.Condition](), ContainElement(
			SatisfyAll(
				HaveField("Type", Equal("Provisioned")),
				HaveField("Status", Equal(metav1.ConditionTrue)),
				HaveField("Message", ContainSubstring(providerName)),
			),
		)),
	)
	st.waitCredentialsSecret(t, name, database)
}

func (st *databaseClaimSuite) testCRDValidation(t *testing.T) {
	ctx := t.Context()
	cli := st.env.Client
	ns := st.env.Namespace
	selector := &metav1.LabelSelector{MatchLabels: map[string]string{"k": "v"}}

	t.Run("rejects-provider-both-set", func(t *testing.T) {
		g := NewWithT(t)
		obj := &infraApi.DatabaseClaim{
			ObjectMeta: metav1.ObjectMeta{Name: "dc-both", Namespace: ns},
			Spec: infraApi.DatabaseClaimSpec{
				Provider: infraApi.ProviderRef{Name: "p", Selector: selector},
				Database: "somedb",
			},
		}
		g.Expect(cli.Create(ctx, obj)).To(HaveOccurred())
	})

	t.Run("rejects-provider-neither-set", func(t *testing.T) {
		g := NewWithT(t)
		obj := &infraApi.DatabaseClaim{
			ObjectMeta: metav1.ObjectMeta{Name: "dc-neither", Namespace: ns},
			Spec:       infraApi.DatabaseClaimSpec{Database: "somedb"},
		}
		g.Expect(cli.Create(ctx, obj)).To(HaveOccurred())
	})

	t.Run("rejects-missing-database", func(t *testing.T) {
		g := NewWithT(t)
		obj := &infraApi.DatabaseClaim{
			ObjectMeta: metav1.ObjectMeta{Name: "dc-missing-db", Namespace: ns},
			Spec:       infraApi.DatabaseClaimSpec{Provider: infraApi.ProviderRef{Name: "p"}},
		}
		g.Expect(cli.Create(ctx, obj)).To(HaveOccurred())
	})

	t.Run("accepts-valid-and-database-is-immutable", func(t *testing.T) {
		g := NewWithT(t)
		obj := &infraApi.DatabaseClaim{
			ObjectMeta: metav1.ObjectMeta{Name: "dc-valid", Namespace: ns},
			Spec: infraApi.DatabaseClaimSpec{
				Provider: infraApi.ProviderRef{Name: "p"},
				Database: "ai_pipelines",
			},
		}
		g.Expect(cli.Create(ctx, obj)).To(Succeed())
		t.Cleanup(func() { _ = cli.Delete(ctx, obj) })

		obj.Spec.Database = "a_different_database"
		g.Expect(cli.Update(ctx, obj)).To(HaveOccurred())
	})
}
