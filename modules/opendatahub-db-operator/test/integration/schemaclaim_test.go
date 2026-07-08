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

	infraApi "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/api/infrastructure/v1alpha1"
	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/pkg/postgres"
)

func TestSchemaClaim(t *testing.T) {
	suite := &schemaClaimSuite{env: newIntegrationEnv(t)}
	t.Run("schema claim suite", suite.Run)
}

func (st *schemaClaimSuite) Run(t *testing.T) {
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
