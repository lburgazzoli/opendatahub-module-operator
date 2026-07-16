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

	"github.com/lburgazzoli/gomega-matchers/pkg/matchers/jq"
	k8sm "github.com/lburgazzoli/gomega-matchers/pkg/matchers/k8s"
	. "github.com/onsi/gomega"
	"github.com/rs/xid"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	infraApi "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/api/infrastructure/v1alpha1"
	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/pkg/postgres"
	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/test/support"
)

func (st *databaseClaimSuite) Run(t *testing.T) {
	t.Run("crd validation", st.testCRDValidation)
	t.Run("provisioning", st.testProvisioning)
	t.Run("access mode is enforced", st.testAccessModeEnforcement)
	t.Run("secret name override", st.testSecretNameOverride)
	t.Run("idempotency", st.testIdempotency)
	t.Run("secret deletion rotates credentials", st.testSecretDeletionRecovery)
	t.Run("role deletion rotates credentials", st.testRoleDeletionRecovery)
	t.Run("database not found", st.testDatabaseNotFound)
	t.Run("deletion keeps peer user and database", st.testDeletionKeepsPeerUserAndDatabase)
	t.Run("provider not found", st.testProviderNotFound)
	t.Run("provider creation wakes pending claim", st.testProviderCreationWakesPendingClaim)
}

func pingConfig(ctx context.Context, factory postgres.ClientFactory, cfg postgres.Config) error {
	cli, err := factory(ctx, cfg)
	if err != nil {
		return err
	}
	defer cli.Close()

	return cli.Ping(ctx)
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
	g.Expect(pingConfig(ctx, st.env.ClientFactory, credCfg)).To(Succeed())
}

func (st *databaseClaimSuite) testAccessModeEnforcement(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	database := "access_" + xid.New().String()
	st.createDatabase(t, database)

	readWriteClaim := st.newClaim("dc-rw-"+xid.New().String(), database)
	readWriteClaim.Spec.Access = infraApi.AccessModeReadWrite
	st.createClaim(t, readWriteClaim)
	st.waitProvisioned(t, readWriteClaim)

	readOnlyClaim := st.newClaim("dc-ro-"+xid.New().String(), database)
	readOnlyClaim.Spec.Access = infraApi.AccessModeReadOnly
	st.createClaim(t, readOnlyClaim)
	st.waitProvisioned(t, readOnlyClaim)

	readWriteSecret := st.waitCredentialsSecret(t, readWriteClaim.Name, database)
	readOnlySecret := st.waitCredentialsSecret(t, readOnlyClaim.Name, database)

	readWriteCfg, err := postgres.ParseSecret(readWriteSecret.Data)
	g.Expect(err).NotTo(HaveOccurred())

	readOnlyCfg, err := postgres.ParseSecret(readOnlySecret.Data)
	g.Expect(err).NotTo(HaveOccurred())

	readWritePool, err := st.env.ClientFactory(ctx, readWriteCfg)
	g.Expect(err).NotTo(HaveOccurred())
	t.Cleanup(readWritePool.Close)

	readOnlyPool, err := st.env.ClientFactory(ctx, readOnlyCfg)
	g.Expect(err).NotTo(HaveOccurred())
	t.Cleanup(readOnlyPool.Close)

	readWriteSchema := "rw_claim_schema"
	_, err = readWritePool.Exec(
		ctx,
		fmt.Sprintf("CREATE SCHEMA %s", postgres.QuoteIdentifier(readWriteSchema)),
	)
	g.Expect(err).NotTo(HaveOccurred())

	adminClient := st.db.Client()
	g.Expect(adminClient).NotTo(BeNil())
	t.Cleanup(func() {
		_ = postgres.DropSchemaCascade(ctx, adminClient, readWriteSchema)
	})

	_, err = readOnlyPool.Exec(
		ctx,
		fmt.Sprintf("CREATE SCHEMA %s", postgres.QuoteIdentifier("ro_claim_schema")),
	)
	g.Expect(err).To(HaveOccurred(), "readonly database claim must not be able to create schemas")
}

func (st *databaseClaimSuite) testSecretNameOverride(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	database := "app_" + xid.New().String()
	st.createDatabase(t, database)

	name := "dc-" + xid.New().String()
	secretName := name + "-credentials"
	claim := st.newClaim(name, database)
	claim.Spec.SecretName = secretName
	st.createClaim(t, claim)

	st.waitProvisioned(t, claim)

	g.Eventually(ctx, k8sm.Get(st.env.Client, claim)).Should(
		jq.Matchf(`.status.connection.secretRef.name == %q`, secretName),
	)

	secret := st.waitCredentialsSecret(t, secretName, database)
	st.expectNoCredentialsSecret(t, name)

	credCfg, err := postgres.ParseSecret(secret.Data)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(credCfg.DBName).To(Equal(database))
	g.Expect(pingConfig(ctx, st.env.ClientFactory, credCfg)).To(Succeed())
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

	g.Expect(support.DeleteAndWait(ctx, st.env.Client, secret)).To(Succeed())
	st.triggerReconcile(t, claim)

	recovered := st.waitCredentialsSecret(t, name, database)
	g.Expect(string(recovered.Data[postgres.SecretKeyPassword])).NotTo(Equal(oldPassword))

	newCfg, err := postgres.ParseSecret(recovered.Data)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(pingConfig(ctx, st.env.ClientFactory, newCfg)).To(Succeed())
	g.Expect(pingConfig(ctx, st.env.ClientFactory, oldCfg)).To(HaveOccurred())
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
	g.Expect(pingConfig(ctx, st.env.ClientFactory, newCfg)).To(Succeed())
	g.Expect(pingConfig(ctx, st.env.ClientFactory, oldCfg)).To(HaveOccurred())
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
		if err := support.DeleteAndWait(context.Background(), st.env.Client, claimB); err != nil {
			t.Errorf("deleting database claim: %v", err)
		}
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
	g.Expect(pingConfig(ctx, st.env.ClientFactory, cfgA)).To(Succeed())
	g.Expect(pingConfig(ctx, st.env.ClientFactory, cfgB)).To(Succeed())

	st.deleteClaimAndWait(t, claimA)
	st.expectNoCredentialsSecret(t, nameA)

	g.Expect(st.databaseExists(t, database)).To(BeTrue())
	g.Expect(st.roleExists(t, databaseClaimRoleName(st.env.Namespace, nameA))).To(BeFalse())
	g.Expect(st.roleExists(t, databaseClaimRoleName(st.env.Namespace, nameB))).To(BeTrue())
	g.Expect(pingConfig(ctx, st.env.ClientFactory, cfgB)).To(Succeed())
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

	err := st.createExternalProvider(t, providerName, st.db.Config(), nil, nil)
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
				Provider: infraApi.ProviderRef{Name: st.providerName},
				Database: st.db.Config().DBName,
			},
		}
		g.Expect(cli.Create(ctx, obj)).To(Succeed())
		t.Cleanup(func() {
			if err := support.DeleteAndWait(context.Background(), st.env.Client, obj); err != nil {
				t.Errorf("deleting database claim: %v", err)
			}
		})

		obj.Spec.Database = "a_different_database"
		g.Expect(cli.Update(ctx, obj)).To(HaveOccurred())
	})

	t.Run("rejects-access-mutation", func(t *testing.T) {
		g := NewWithT(t)
		obj := &infraApi.DatabaseClaim{
			ObjectMeta: metav1.ObjectMeta{Name: "dc-immut-access", Namespace: ns},
			Spec: infraApi.DatabaseClaimSpec{
				Provider: infraApi.ProviderRef{Name: st.providerName},
				Database: st.db.Config().DBName,
				Access:   infraApi.AccessModeReadWrite,
			},
		}
		g.Expect(cli.Create(ctx, obj)).To(Succeed())
		t.Cleanup(func() {
			if err := support.DeleteAndWait(context.Background(), st.env.Client, obj); err != nil {
				t.Errorf("deleting database claim: %v", err)
			}
		})

		obj.Spec.Access = infraApi.AccessModeReadOnly
		g.Expect(cli.Update(ctx, obj)).To(HaveOccurred())
	})

	t.Run("rejects-provider-mutation", func(t *testing.T) {
		g := NewWithT(t)
		obj := &infraApi.DatabaseClaim{
			ObjectMeta: metav1.ObjectMeta{Name: "dc-immut-provider", Namespace: ns},
			Spec: infraApi.DatabaseClaimSpec{
				Provider: infraApi.ProviderRef{Name: st.providerName},
				Database: st.db.Config().DBName,
			},
		}
		g.Expect(cli.Create(ctx, obj)).To(Succeed())
		t.Cleanup(func() {
			if err := support.DeleteAndWait(context.Background(), st.env.Client, obj); err != nil {
				t.Errorf("deleting database claim: %v", err)
			}
		})

		obj.Spec.Provider = infraApi.ProviderRef{Name: "q"}
		g.Expect(cli.Update(ctx, obj)).To(HaveOccurred())
	})
}
