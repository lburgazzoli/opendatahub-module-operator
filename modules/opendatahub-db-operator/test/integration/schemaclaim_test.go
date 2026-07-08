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
	"strings"
	"testing"

	. "github.com/onsi/gomega"
	"github.com/rs/xid"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	infraApi "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/api/infrastructure/v1alpha1"
	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/internal/controller/schemaclaim"
	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/pkg/postgres"
	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/test/support"
	"github.com/opendatahub-io/odh-platform-utilities/framework/controller/conditions"
)

func requireSharedSetup(t *testing.T) {
	t.Helper()
	if sharedClient == nil || sharedProvider == nil {
		t.Skip("shared postgres/manager not available (Docker missing?)")
	}
}

// TestSchemaClaim_Provisioning exercises the full happy path.
func TestSchemaClaim_Provisioning(t *testing.T) {
	requireSharedSetup(t)
	g := NewWithT(t)
	ns := support.IntegrationTestNamespace()
	ctx := t.Context()

	name := "sc-" + xid.New().String()
	claim := &infraApi.SchemaClaim{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
		Spec: infraApi.SchemaClaimSpec{
			Provider: infraApi.ProviderRef{Name: sharedProvider.Name},
		},
	}
	g.Expect(sharedClient.Create(ctx, claim)).To(Succeed())
	t.Cleanup(func() { deleteAndWait(context.Background(), sharedClient, claim) })

	g.Eventually(func(g Gomega) {
		g.Expect(sharedClient.Get(ctx, client.ObjectKeyFromObject(claim), claim)).To(Succeed())
		cond := conditions.FindStatusCondition(claim, schemaclaim.ConditionProvisioned)
		g.Expect(cond).NotTo(BeNil())
		g.Expect(cond.Status).To(Equal(metav1.ConditionTrue))
	}).Should(Succeed())

	// status.schema defaults to ${namespace}_${name} with hyphens sanitized to underscores
	expectedSchema := strings.ReplaceAll(ns+"_"+name, "-", "_")
	g.Expect(claim.Status.Schema).To(Equal(expectedSchema))

	// Credentials Secret exists with pg.* keys
	secret := &corev1.Secret{}
	g.Expect(sharedClient.Get(ctx, client.ObjectKey{Namespace: ns, Name: name}, secret)).To(Succeed())
	g.Expect(secret.Data).To(HaveKey(postgres.SecretKeyHost))
	g.Expect(secret.Data).To(HaveKey(postgres.SecretKeyUser))
	g.Expect(secret.Data).To(HaveKey(postgres.SecretKeyPassword))
	g.Expect(secret.Data).To(HaveKey(postgres.SecretKeySchema))

	// Credentials actually work
	credCfg, err := postgres.ParseSecret(secret.Data)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(postgres.Ping(ctx, credCfg)).To(Succeed())
}

// TestSchemaClaim_ExplicitSchema verifies spec.schema is respected.
func TestSchemaClaim_ExplicitSchema(t *testing.T) {
	requireSharedSetup(t)
	g := NewWithT(t)
	ns := support.IntegrationTestNamespace()
	ctx := t.Context()

	name := "sc-" + xid.New().String()
	claim := &infraApi.SchemaClaim{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
		Spec: infraApi.SchemaClaimSpec{
			Provider: infraApi.ProviderRef{Name: sharedProvider.Name},
			Schema:   "explicit_schema",
		},
	}
	g.Expect(sharedClient.Create(ctx, claim)).To(Succeed())
	t.Cleanup(func() {
		deleteAndWait(context.Background(), sharedClient, claim)
	})

	g.Eventually(func(g Gomega) {
		g.Expect(sharedClient.Get(ctx, client.ObjectKeyFromObject(claim), claim)).To(Succeed())
		cond := conditions.FindStatusCondition(claim, schemaclaim.ConditionProvisioned)
		g.Expect(cond).NotTo(BeNil())
		g.Expect(cond.Status).To(Equal(metav1.ConditionTrue))
	}).Should(Succeed())

	g.Expect(claim.Status.Schema).To(Equal("explicit_schema"))
}

// TestSchemaClaim_Idempotency verifies a second reconcile doesn't rotate credentials.
func TestSchemaClaim_Idempotency(t *testing.T) {
	requireSharedSetup(t)
	g := NewWithT(t)
	ns := support.IntegrationTestNamespace()
	ctx := t.Context()

	name := "sc-" + xid.New().String()
	claim := &infraApi.SchemaClaim{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
		Spec:       infraApi.SchemaClaimSpec{Provider: infraApi.ProviderRef{Name: sharedProvider.Name}},
	}
	g.Expect(sharedClient.Create(ctx, claim)).To(Succeed())
	t.Cleanup(func() {
		deleteAndWait(context.Background(), sharedClient, claim)
	})

	// Wait for first provisioning
	g.Eventually(func(g Gomega) {
		g.Expect(sharedClient.Get(ctx, client.ObjectKeyFromObject(claim), claim)).To(Succeed())
		cond := conditions.FindStatusCondition(claim, schemaclaim.ConditionProvisioned)
		g.Expect(cond).NotTo(BeNil())
		g.Expect(cond.Status).To(Equal(metav1.ConditionTrue))
	}).Should(Succeed())

	// Read the first password
	secret := &corev1.Secret{}
	g.Expect(sharedClient.Get(ctx, client.ObjectKey{Namespace: ns, Name: name}, secret)).To(Succeed())
	firstPw := string(secret.Data[postgres.SecretKeyPassword])
	g.Expect(firstPw).NotTo(BeEmpty())

	// Trigger a re-reconcile by touching the claim
	g.Expect(sharedClient.Get(ctx, client.ObjectKeyFromObject(claim), claim)).To(Succeed())
	if claim.Annotations == nil {
		claim.Annotations = make(map[string]string)
	}
	claim.Annotations["test/trigger"] = "1"
	g.Expect(sharedClient.Update(ctx, claim)).To(Succeed())

	// Verify password is stable after re-reconcile
	g.Consistently(func(g Gomega) {
		s := &corev1.Secret{}
		g.Expect(sharedClient.Get(ctx, client.ObjectKey{Namespace: ns, Name: name}, s)).To(Succeed())
		g.Expect(string(s.Data[postgres.SecretKeyPassword])).To(Equal(firstPw))
	}, "5s", "500ms").Should(Succeed())
}

// TestSchemaClaim_ProviderNotFound verifies Provisioned: False when the provider is missing.
func TestSchemaClaim_ProviderNotFound(t *testing.T) {
	requireSharedSetup(t)
	g := NewWithT(t)
	ns := support.IntegrationTestNamespace()
	ctx := t.Context()

	name := "sc-" + xid.New().String()
	claim := &infraApi.SchemaClaim{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
		Spec:       infraApi.SchemaClaimSpec{Provider: infraApi.ProviderRef{Name: "does-not-exist"}},
	}
	g.Expect(sharedClient.Create(ctx, claim)).To(Succeed())
	t.Cleanup(func() {
		deleteAndWait(context.Background(), sharedClient, claim)
	})

	g.Eventually(func(g Gomega) {
		g.Expect(sharedClient.Get(ctx, client.ObjectKeyFromObject(claim), claim)).To(Succeed())
		cond := conditions.FindStatusCondition(claim, schemaclaim.ConditionProvisioned)
		g.Expect(cond).NotTo(BeNil())
		g.Expect(cond.Status).To(Equal(metav1.ConditionFalse))
		g.Expect(cond.Reason).To(Equal("ProviderNotFound"))
	}).Should(Succeed())

	// No Secret must exist
	g.Expect(sharedClient.Get(ctx, client.ObjectKey{Namespace: ns, Name: name}, &corev1.Secret{})).
		To(MatchError(ContainSubstring("not found")))
}
