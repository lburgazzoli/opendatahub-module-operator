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
	"strings"
	"testing"

	"github.com/lburgazzoli/gomega-matchers/pkg/matchers/jq"
	k8sm "github.com/lburgazzoli/gomega-matchers/pkg/matchers/k8s"
	. "github.com/onsi/gomega"
	"github.com/rs/xid"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	infraApi "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/api/infrastructure/v1alpha1"
	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/pkg/postgres"
)

func TestSchemaClaim(t *testing.T) {
	g := NewWithT(t)

	suite, err := newSchemaClaimSuite(t)
	g.Expect(err).ToNot(HaveOccurred())

	t.Run("schema claim suite", suite.Run)
}

func (st *schemaClaimSuite) Run(t *testing.T) {
	t.Run("crd validation", st.testCRDValidation)
	t.Run("provisioning", st.testProvisioning)
	t.Run("explicit schema", st.testExplicitSchema)
	t.Run("idempotency", st.testIdempotency)
	t.Run("provider not found", st.testProviderNotFound)
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
	g.Expect(postgres.Ping(ctx, credCfg)).To(Succeed())
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

// testProviderNotFound verifies Provisioned: False when the provider is missing.
func (st *schemaClaimSuite) testProviderNotFound(t *testing.T) {
	name := "sc-" + xid.New().String()
	claim := st.newClaim(name)
	claim.Spec.Provider = infraApi.ProviderRef{Name: "does-not-exist"}
	st.createClaim(t, claim)

	st.waitProvisioningFailure(t, claim, "ProviderNotFound")
	st.expectNoCredentialsSecret(t, name)
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
				Provider: infraApi.ProviderRef{Name: "p"},
				Schema:   "my_schema",
			},
		}
		g.Expect(cli.Create(ctx, obj)).To(Succeed())
		t.Cleanup(func() { _ = cli.Delete(ctx, obj) })

		obj.Spec.Schema = "a_different_schema"
		g.Expect(cli.Update(ctx, obj)).To(HaveOccurred())
	})
}
