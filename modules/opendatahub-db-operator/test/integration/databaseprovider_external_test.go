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
	. "github.com/onsi/gomega"
	"github.com/rs/xid"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	infraApi "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/api/infrastructure/v1alpha1"
	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/internal/controller/databaseprovider"
	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/pkg/postgres"
	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/test/support"
	testdb "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/test/support/db"
)

type databaseProviderSuite struct {
	env *integrationEnv
	db  *testdb.Instance
}

func newDatabaseProviderSuite(t *testing.T, env *integrationEnv) (*databaseProviderSuite, error) {
	t.Helper()

	suite := &databaseProviderSuite{
		env: env,
	}

	db, err := testdb.Start(t.Context())
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

	return suite, nil
}

func (st *databaseProviderSuite) Run(t *testing.T) {
	t.Run("crd validation", st.testCRDValidation)
	t.Run("reachable", st.testReachable)
	t.Run("auth failure is surfaced", st.testAuthFailure)
}

func (st *databaseProviderSuite) testCRDValidation(t *testing.T) {
	ctx := t.Context()
	cli := st.env.Client
	externalSpec := infraApi.ExternalProviderSpec{
		ConnectionSecretRef: corev1.SecretReference{Name: "admin-secret", Namespace: st.env.Namespace},
	}
	embeddedSpec := infraApi.EmbeddedProviderSpec{
		Storage: infraApi.StorageSpec{Size: resource.MustParse("1Gi")},
	}

	cases := []struct {
		name string
		spec infraApi.DatabaseProviderSpec
	}{
		{
			name: "both-set",
			spec: infraApi.DatabaseProviderSpec{
				Type:     infraApi.ProviderTypeExternal,
				External: &externalSpec,
				Embedded: &embeddedSpec,
			},
		},
		{
			name: "neither-set",
			spec: infraApi.DatabaseProviderSpec{Type: infraApi.ProviderTypeExternal},
		},
		{
			name: "type-mismatch-external",
			spec: infraApi.DatabaseProviderSpec{
				Type:     infraApi.ProviderTypeExternal,
				Embedded: &embeddedSpec,
			},
		},
	}

	for _, tc := range cases {
		t.Run("rejects-"+tc.name, func(t *testing.T) {
			g := NewWithT(t)
			obj := &infraApi.DatabaseProvider{
				ObjectMeta: metav1.ObjectMeta{Name: "dbp-" + tc.name},
				Spec:       tc.spec,
			}
			g.Expect(cli.Create(ctx, obj)).To(HaveOccurred())
		})
	}

	t.Run("accepts-valid-external", func(t *testing.T) {
		g := NewWithT(t)
		obj := &infraApi.DatabaseProvider{
			ObjectMeta: metav1.ObjectMeta{Name: "dbp-valid-external"},
			Spec: infraApi.DatabaseProviderSpec{
				Type:     infraApi.ProviderTypeExternal,
				External: &externalSpec,
			},
		}
		g.Expect(cli.Create(ctx, obj)).To(Succeed())
		t.Cleanup(func() { _ = cli.Delete(ctx, obj) })
	})
}

func (st *databaseProviderSuite) testReachable(t *testing.T) {
	cfg := st.db.Config()
	secret := st.createConnectionSecret(t, "provider-secret-"+xid.New().String(), cfg)
	provider := st.createExternalProvider(t, "provider-"+xid.New().String(), secret)

	st.waitReachable(t, provider)
}

func (st *databaseProviderSuite) testAuthFailure(t *testing.T) {
	g := NewWithT(t)

	g.Eventually(func() error {
		return postgres.Ping(t.Context(), st.db.Config())
	}).Should(Succeed())

	cfg := st.db.Config()
	cfg.Password = "wrong-password-sentinel"
	g.Eventually(func() error {
		return postgres.Ping(t.Context(), cfg)
	}).Should(MatchError(ContainSubstring("password authentication failed")))

	secret := st.createConnectionSecret(t, "provider-secret-"+xid.New().String(), cfg)
	provider := st.createExternalProvider(t, "provider-"+xid.New().String(), secret)

	expectDatabaseProviderUnreachable(
		t,
		st.env,
		provider,
		"ConnectionCheckFailed",
		"password authentication failed",
	)

	msg := st.reachableConditionMessage(t, provider)
	NewWithT(t).Expect(msg).NotTo(ContainSubstring(cfg.Password))
}

func (st *databaseProviderSuite) createConnectionSecret(
	t *testing.T,
	name string,
	cfg postgres.Config,
) *corev1.Secret {
	t.Helper()

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: st.env.Namespace,
		},
		StringData: map[string]string{
			postgres.SecretKeyHost:     cfg.Host,
			postgres.SecretKeyPort:     fmt.Sprintf("%d", cfg.Port),
			postgres.SecretKeyUser:     cfg.User,
			postgres.SecretKeyPassword: cfg.Password,
			postgres.SecretKeyDatabase: cfg.DBName,
		},
	}

	NewWithT(t).Expect(st.env.Client.Create(t.Context(), secret)).To(Succeed())
	t.Cleanup(func() {
		if err := support.DeleteAndWait(context.Background(), st.env.Client, secret); err != nil {
			t.Errorf("deleting secret: %v", err)
		}
	})

	return secret
}

func (st *databaseProviderSuite) createExternalProvider(
	t *testing.T,
	name string,
	secret *corev1.Secret,
) *infraApi.DatabaseProvider {
	t.Helper()

	provider := &infraApi.DatabaseProvider{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec: infraApi.DatabaseProviderSpec{
			Type: infraApi.ProviderTypeExternal,
			External: &infraApi.ExternalProviderSpec{
				ConnectionSecretRef: corev1.SecretReference{
					Name:      secret.Name,
					Namespace: secret.Namespace,
				},
			},
		},
	}

	NewWithT(t).Expect(st.env.Client.Create(t.Context(), provider)).To(Succeed())
	t.Cleanup(func() {
		if err := support.DeleteAndWait(context.Background(), st.env.Client, provider); err != nil {
			t.Errorf("deleting provider: %v", err)
		}
	})

	return provider
}

func (st *databaseProviderSuite) waitReachable(t *testing.T, provider *infraApi.DatabaseProvider) {
	t.Helper()

	NewWithT(t).Eventually(t.Context(), k8sm.Get(st.env.Client, provider)).Should(
		WithTransform(k8sm.ConditionsOf[metav1.Condition](),
			ContainElement(condition.Is(databaseprovider.ConditionReachable, metav1.ConditionTrue))),
	)
}

func (st *databaseProviderSuite) reachableConditionMessage(
	t *testing.T,
	provider *infraApi.DatabaseProvider,
) string {
	t.Helper()

	current := &infraApi.DatabaseProvider{ObjectMeta: metav1.ObjectMeta{Name: provider.Name}}
	NewWithT(t).Eventually(t.Context(), k8sm.Lookup(st.env.Client, current)).To(Succeed())

	for _, cond := range current.Status.Conditions {
		if cond.Type == databaseprovider.ConditionReachable {
			return cond.Message
		}
	}

	t.Fatalf("reachable condition not found for provider %q", provider.Name)
	return ""
}
