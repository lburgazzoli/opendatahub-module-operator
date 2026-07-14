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
	"strings"
	"testing"

	"github.com/lburgazzoli/gomega-matchers/pkg/matchers/jq"
	k8sm "github.com/lburgazzoli/gomega-matchers/pkg/matchers/k8s"
	. "github.com/onsi/gomega"
	"github.com/rs/xid"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	infraApi "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/api/infrastructure/v1alpha1"
	dbcontroller "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/pkg/controller"
	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/pkg/postgres"
	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/test/support"
)

func (st *schemaClaimSuite) Run(t *testing.T) {
	t.Run("crd validation", st.testCRDValidation)
	t.Run("provisioning", st.testProvisioning)
	t.Run("access mode is enforced", st.testAccessModeEnforcement)
	t.Run("secret name override", st.testSecretNameOverride)
	t.Run("explicit schema", st.testExplicitSchema)
	t.Run("idempotency", st.testIdempotency)
	t.Run("secret deletion rotates credentials", st.testSecretDeletionRecovery)
	t.Run("role deletion rotates credentials", st.testRoleDeletionRecovery)
	t.Run("schema deletion rotates credentials", st.testSchemaDeletionRecovery)
	t.Run("provider not found", st.testProviderNotFound)
	t.Run("selector keeps current provider when better match appears", st.testSelectorKeepsCurrentProvider)
}

func pingClaimConfig(ctx context.Context, cfg postgres.Config) error {
	cli, err := postgres.NewClient(ctx, cfg)
	if err != nil {
		return err
	}
	defer cli.Close()

	return cli.Ping(ctx)
}

// testProvisioning exercises the full happy path.
func (st *schemaClaimSuite) testProvisioning(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	name := "sc-" + xid.New().String()
	claim := st.newClaim(name)
	st.createClaim(t, claim)

	st.waitProvisioned(t, claim)

	expectedSchema := strings.ReplaceAll(st.env.Namespace+"_"+name, "-", "_")
	g.Eventually(ctx, k8sm.Get(st.env.Client, claim)).Should(
		jq.Matchf(`.status.schema == %q`, expectedSchema),
	)

	secret := st.waitCredentialsSecret(t, name)

	credCfg, err := postgres.ParseSecret(secret.Data)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(credCfg.DBName).To(Equal(st.databaseName))
	g.Expect(pingClaimConfig(ctx, credCfg)).To(Succeed())
}

func (st *schemaClaimSuite) testAccessModeEnforcement(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	readWriteClaim := st.newClaim("sc-rw-" + xid.New().String())
	readWriteClaim.Spec.Access = infraApi.AccessModeReadWrite
	readWriteClaim.Spec.Schema = "rw_schema_" + xid.New().String()
	readWriteClaim.Spec.DeletionPolicy = infraApi.DeletionPolicyDelete
	st.createClaim(t, readWriteClaim)
	st.waitProvisioned(t, readWriteClaim)

	readOnlyClaim := st.newClaim("sc-ro-" + xid.New().String())
	readOnlyClaim.Spec.Access = infraApi.AccessModeReadOnly
	readOnlyClaim.Spec.Schema = "ro_schema_" + xid.New().String()
	readOnlyClaim.Spec.DeletionPolicy = infraApi.DeletionPolicyDelete
	st.createClaim(t, readOnlyClaim)
	st.waitProvisioned(t, readOnlyClaim)

	readWriteSecret := st.waitCredentialsSecret(t, readWriteClaim.Name)
	readOnlySecret := st.waitCredentialsSecret(t, readOnlyClaim.Name)

	readWriteCfg, err := postgres.ParseSecret(readWriteSecret.Data)
	g.Expect(err).NotTo(HaveOccurred())
	readOnlyCfg, err := postgres.ParseSecret(readOnlySecret.Data)
	g.Expect(err).NotTo(HaveOccurred())

	readWritePool, err := postgres.NewClient(ctx, readWriteCfg)
	g.Expect(err).NotTo(HaveOccurred())
	t.Cleanup(readWritePool.Close)

	readOnlyPool, err := postgres.NewClient(ctx, readOnlyCfg)
	g.Expect(err).NotTo(HaveOccurred())
	t.Cleanup(readOnlyPool.Close)

	readWriteTable := "rw_claim_table"
	_, err = readWritePool.Exec(
		ctx,
		fmt.Sprintf(
			"CREATE TABLE %s.%s (id int PRIMARY KEY)",
			postgres.QuoteIdentifier(readWriteCfg.Schema),
			postgres.QuoteIdentifier(readWriteTable),
		),
	)
	g.Expect(err).NotTo(HaveOccurred())

	_, err = readWritePool.Exec(
		ctx,
		fmt.Sprintf(
			"INSERT INTO %s.%s VALUES (1)",
			postgres.QuoteIdentifier(readWriteCfg.Schema),
			postgres.QuoteIdentifier(readWriteTable),
		),
	)
	g.Expect(err).NotTo(HaveOccurred())

	_, err = readOnlyPool.Exec(
		ctx,
		fmt.Sprintf(
			"CREATE TABLE %s.%s (id int PRIMARY KEY)",
			postgres.QuoteIdentifier(readOnlyCfg.Schema),
			postgres.QuoteIdentifier("ro_claim_table"),
		),
	)
	g.Expect(err).To(HaveOccurred(), "readonly schema claim must not be able to create tables")
}

func (st *schemaClaimSuite) testSecretNameOverride(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	name := "sc-" + xid.New().String()
	secretName := name + "-credentials"
	claim := st.newClaim(name)
	claim.Spec.SecretName = secretName
	st.createClaim(t, claim)

	st.waitProvisioned(t, claim)

	g.Eventually(ctx, k8sm.Get(st.env.Client, claim)).Should(
		jq.Matchf(`.status.connection.secretRef.name == %q`, secretName),
	)

	secret := st.waitCredentialsSecret(t, secretName)
	st.expectNoCredentialsSecret(t, name)

	credCfg, err := postgres.ParseSecret(secret.Data)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(credCfg.DBName).To(Equal(st.databaseName))
	g.Expect(pingClaimConfig(ctx, credCfg)).To(Succeed())
}

// testExplicitSchema verifies spec.schema is respected.
func (st *schemaClaimSuite) testExplicitSchema(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	name := "sc-" + xid.New().String()
	claim := st.newClaim(name)
	claim.Spec.Schema = "explicit_schema"
	st.createClaim(t, claim)

	st.waitProvisioned(t, claim)

	g.Eventually(ctx, k8sm.Get(st.env.Client, claim)).Should(
		jq.Match(`.status.schema == "explicit_schema"`),
	)
}

// testIdempotency verifies a second reconcile doesn't rotate credentials.
func (st *schemaClaimSuite) testIdempotency(t *testing.T) {
	g := NewWithT(t)

	name := "sc-" + xid.New().String()
	claim := st.newClaim(name)
	st.createClaim(t, claim)

	st.waitProvisioned(t, claim)

	secret := st.waitCredentialsSecret(t, name)
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

func (st *schemaClaimSuite) testSecretDeletionRecovery(t *testing.T) {
	g := NewWithT(t)

	name := "sc-" + xid.New().String()
	claim := st.newClaim(name)
	st.createClaim(t, claim)
	st.waitProvisioned(t, claim)

	secret := st.waitCredentialsSecret(t, name)
	oldCfg, err := postgres.ParseSecret(secret.Data)
	g.Expect(err).NotTo(HaveOccurred())
	oldPassword := string(secret.Data[postgres.SecretKeyPassword])

	g.Expect(support.DeleteAndWait(t.Context(), st.env.Client, secret)).To(Succeed())
	st.triggerReconcile(t, claim)

	recovered := st.waitCredentialsSecret(t, name)
	g.Expect(string(recovered.Data[postgres.SecretKeyPassword])).NotTo(Equal(oldPassword))

	newCfg, err := postgres.ParseSecret(recovered.Data)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(pingClaimConfig(t.Context(), newCfg)).To(Succeed())
	g.Expect(pingClaimConfig(t.Context(), oldCfg)).To(HaveOccurred())
}

func (st *schemaClaimSuite) testRoleDeletionRecovery(t *testing.T) {
	g := NewWithT(t)

	name := "sc-" + xid.New().String()
	claim := st.newClaim(name)
	st.createClaim(t, claim)
	st.waitProvisioned(t, claim)

	secret := st.waitCredentialsSecret(t, name)
	oldCfg, err := postgres.ParseSecret(secret.Data)
	g.Expect(err).NotTo(HaveOccurred())
	oldPassword := string(secret.Data[postgres.SecretKeyPassword])

	g.Expect(oldCfg.Schema).NotTo(BeEmpty())
	st.dropRole(t, oldCfg.User, oldCfg.Schema)
	g.Expect(st.roleExists(t, oldCfg.User)).To(BeFalse())

	st.triggerReconcile(t, claim)

	g.Eventually(t.Context(), func() string {
		fresh := st.waitCredentialsSecret(t, name)
		return string(fresh.Data[postgres.SecretKeyPassword])
	}).ShouldNot(Equal(oldPassword))
	recovered := st.waitCredentialsSecret(t, name)

	newCfg, err := postgres.ParseSecret(recovered.Data)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(pingClaimConfig(t.Context(), newCfg)).To(Succeed())
	g.Expect(pingClaimConfig(t.Context(), oldCfg)).To(HaveOccurred())
}

func (st *schemaClaimSuite) testSchemaDeletionRecovery(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	name := "sc-" + xid.New().String()
	claim := st.newClaim(name)
	st.createClaim(t, claim)
	st.waitProvisioned(t, claim)

	secret := st.waitCredentialsSecret(t, name)
	oldCfg, err := postgres.ParseSecret(secret.Data)
	g.Expect(err).NotTo(HaveOccurred())
	oldPassword := string(secret.Data[postgres.SecretKeyPassword])
	current := &infraApi.SchemaClaim{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: st.env.Namespace}}
	g.Eventually(ctx, k8sm.Lookup(st.env.Client, current)).To(Succeed())
	schemaName := current.Status.Schema
	g.Expect(schemaName).NotTo(BeEmpty())

	st.dropSchema(t, schemaName)
	g.Expect(st.schemaExists(t, schemaName)).To(BeFalse())

	st.triggerReconcile(t, claim)
	st.waitProvisioned(t, claim)

	g.Eventually(ctx, func() string {
		fresh := st.waitCredentialsSecret(t, name)
		return string(fresh.Data[postgres.SecretKeyPassword])
	}).ShouldNot(Equal(oldPassword))
	recovered := st.waitCredentialsSecret(t, name)

	newCfg, err := postgres.ParseSecret(recovered.Data)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(pingClaimConfig(ctx, newCfg)).To(Succeed())
	g.Expect(pingClaimConfig(ctx, oldCfg)).To(HaveOccurred())
	g.Expect(st.schemaExists(t, schemaName)).To(BeTrue())
}

// testProviderNotFound verifies Provisioned: False when the provider is missing.
func (st *schemaClaimSuite) testProviderNotFound(t *testing.T) {
	name := "sc-" + xid.New().String()
	claim := st.newClaim(name)
	claim.Spec.Provider = infraApi.ProviderRef{Name: "does-not-exist"}
	st.createClaim(t, claim)

	st.waitProvisioningFailure(t, claim, "ProviderNotFound")
	st.expectNoCredentialsSecret(t, name)
}

func (st *schemaClaimSuite) testSelectorKeepsCurrentProvider(t *testing.T) {
	g := NewWithT(t)

	selector := map[string]string{"cap": "sticky-" + xid.New().String()}
	name := "sc-" + xid.New().String()
	claim := st.newSelectorClaim(name, selector)

	currentProvider := "provider-" + xid.New().String()
	st.createExternalProvider(t, currentProvider, st.db.Config(), st.databaseName, selector, map[string]string{
		dbcontroller.AnnotationSelectionPriority: "0",
	})

	st.createClaim(t, claim)
	st.waitProvisioned(t, claim)
	st.waitSelectedProvider(t, claim, currentProvider)

	secret := st.waitCredentialsSecret(t, name)
	credCfg, err := postgres.ParseSecret(secret.Data)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(credCfg.DBName).To(Equal(st.databaseName))

	otherDB := "schema_" + xid.New().String()
	st.createDatabase(t, otherDB)
	betterProvider := "provider-" + xid.New().String()
	st.createExternalProvider(t, betterProvider, st.db.Config(), otherDB, selector, map[string]string{
		dbcontroller.AnnotationSelectionPriority: "100",
	})

	st.waitProvisioned(t, claim)
	st.waitSelectedProvider(t, claim, currentProvider)

	g.Consistently(t.Context(), k8sm.Get(st.env.Client, claim), "5s", "500ms").Should(
		jq.Matchf(`.status.provider == %q`, currentProvider),
	)
	g.Consistently(t.Context(), k8sm.Get(st.env.Client, secret), "5s", "500ms").Should(
		WithTransform(k8sm.Data(), HaveKeyWithValue(postgres.SecretKeyDatabase, []byte(st.databaseName))),
	)
}

func (st *schemaClaimSuite) testCRDValidation(t *testing.T) {
	ctx := t.Context()
	cli := st.env.Client
	ns := st.env.Namespace
	selector := &metav1.LabelSelector{MatchLabels: map[string]string{"k": "v"}}

	t.Run("rejects-provider-both-set", func(t *testing.T) {
		g := NewWithT(t)
		obj := &infraApi.SchemaClaim{
			ObjectMeta: metav1.ObjectMeta{Name: "sc-both", Namespace: ns},
			Spec:       infraApi.SchemaClaimSpec{Provider: infraApi.ProviderRef{Name: "p", Selector: selector}},
		}
		g.Expect(cli.Create(ctx, obj)).To(HaveOccurred())
	})

	t.Run("rejects-provider-neither-set", func(t *testing.T) {
		g := NewWithT(t)
		obj := &infraApi.SchemaClaim{
			ObjectMeta: metav1.ObjectMeta{Name: "sc-neither", Namespace: ns},
			Spec:       infraApi.SchemaClaimSpec{},
		}
		g.Expect(cli.Create(ctx, obj)).To(HaveOccurred())
	})

	t.Run("rejects-invalid-access", func(t *testing.T) {
		g := NewWithT(t)
		obj := &infraApi.SchemaClaim{
			ObjectMeta: metav1.ObjectMeta{Name: "sc-bad-access", Namespace: ns},
			Spec: infraApi.SchemaClaimSpec{
				Provider: infraApi.ProviderRef{Name: "p"},
				Access:   "NotAValidAccessMode",
			},
		}
		g.Expect(cli.Create(ctx, obj)).To(HaveOccurred())
	})

	t.Run("rejects-invalid-deletion-policy", func(t *testing.T) {
		g := NewWithT(t)
		obj := &infraApi.SchemaClaim{
			ObjectMeta: metav1.ObjectMeta{Name: "sc-bad-deletion-policy", Namespace: ns},
			Spec: infraApi.SchemaClaimSpec{
				Provider:       infraApi.ProviderRef{Name: "p"},
				DeletionPolicy: "NotAValidPolicy",
			},
		}
		g.Expect(cli.Create(ctx, obj)).To(HaveOccurred())
	})

	t.Run("rejects-schema-pattern-violation", func(t *testing.T) {
		g := NewWithT(t)
		obj := &infraApi.SchemaClaim{
			ObjectMeta: metav1.ObjectMeta{Name: "sc-bad-schema-pattern", Namespace: ns},
			Spec: infraApi.SchemaClaimSpec{
				Provider: infraApi.ProviderRef{Name: "p"},
				Schema:   "1-not-a-valid-identifier",
			},
		}
		g.Expect(cli.Create(ctx, obj)).To(HaveOccurred())
	})

	t.Run("rejects-schema-too-long", func(t *testing.T) {
		g := NewWithT(t)
		var tooLong strings.Builder
		for range 64 {
			tooLong.WriteString("a")
		}
		obj := &infraApi.SchemaClaim{
			ObjectMeta: metav1.ObjectMeta{Name: "sc-bad-schema-length", Namespace: ns},
			Spec: infraApi.SchemaClaimSpec{
				Provider: infraApi.ProviderRef{Name: "p"},
				Schema:   tooLong.String(),
			},
		}
		g.Expect(cli.Create(ctx, obj)).To(HaveOccurred())
	})

	t.Run("accepts-valid-and-schema-is-immutable", func(t *testing.T) {
		g := NewWithT(t)
		obj := &infraApi.SchemaClaim{
			ObjectMeta: metav1.ObjectMeta{Name: "sc-valid", Namespace: ns},
			Spec: infraApi.SchemaClaimSpec{
				Provider: infraApi.ProviderRef{Name: st.providerName},
				Schema:   "my_schema",
			},
		}
		g.Expect(cli.Create(ctx, obj)).To(Succeed())
		t.Cleanup(func() {
			if err := support.DeleteAndWait(context.Background(), st.env.Client, obj); err != nil {
				t.Errorf("deleting schema claim: %v", err)
			}
		})

		obj.Spec.Schema = "a_different_schema"
		g.Expect(cli.Update(ctx, obj)).To(HaveOccurred())
	})

	t.Run("rejects-access-mutation", func(t *testing.T) {
		g := NewWithT(t)
		obj := &infraApi.SchemaClaim{
			ObjectMeta: metav1.ObjectMeta{Name: "sc-immut-access", Namespace: ns},
			Spec: infraApi.SchemaClaimSpec{
				Provider: infraApi.ProviderRef{Name: st.providerName},
				Access:   infraApi.AccessModeReadWrite,
			},
		}
		g.Expect(cli.Create(ctx, obj)).To(Succeed())
		t.Cleanup(func() {
			if err := support.DeleteAndWait(context.Background(), st.env.Client, obj); err != nil {
				t.Errorf("deleting schema claim: %v", err)
			}
		})

		obj.Spec.Access = infraApi.AccessModeReadOnly
		g.Expect(cli.Update(ctx, obj)).To(HaveOccurred())
	})

	t.Run("rejects-provider-mutation", func(t *testing.T) {
		g := NewWithT(t)
		obj := &infraApi.SchemaClaim{
			ObjectMeta: metav1.ObjectMeta{Name: "sc-immut-provider", Namespace: ns},
			Spec: infraApi.SchemaClaimSpec{
				Provider: infraApi.ProviderRef{Name: st.providerName},
			},
		}
		g.Expect(cli.Create(ctx, obj)).To(Succeed())
		t.Cleanup(func() {
			if err := support.DeleteAndWait(context.Background(), st.env.Client, obj); err != nil {
				t.Errorf("deleting schema claim: %v", err)
			}
		})

		obj.Spec.Provider = infraApi.ProviderRef{Name: "q"}
		g.Expect(cli.Update(ctx, obj)).To(HaveOccurred())
	})
}
