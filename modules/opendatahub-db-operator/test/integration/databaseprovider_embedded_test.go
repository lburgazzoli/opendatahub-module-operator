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

	k8sm "github.com/lburgazzoli/gomega-matchers/pkg/matchers/k8s"
	"github.com/lburgazzoli/gomega-matchers/pkg/matchers/k8s/condition"
	. "github.com/onsi/gomega"
	"github.com/rs/xid"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	infraApi "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/api/infrastructure/v1alpha1"
	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/internal/controller/databaseprovider"
	dbcontroller "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/pkg/controller"
)

type embeddedDatabaseProviderSuite struct {
	env *integrationEnv
}

func newEmbeddedDatabaseProviderSuite(t *testing.T) (*embeddedDatabaseProviderSuite, error) {
	t.Helper()

	env, err := newIntegrationEnv(t)
	if err != nil {
		return nil, err
	}

	return &embeddedDatabaseProviderSuite{env: env}, nil
}

func TestDatabaseProviderEmbedded(t *testing.T) {
	g := NewWithT(t)

	suite, err := newEmbeddedDatabaseProviderSuite(t)
	g.Expect(err).NotTo(HaveOccurred())

	t.Run("embedded provider suite", suite.Run)
}

func (st *embeddedDatabaseProviderSuite) Run(t *testing.T) {
	t.Run("creates embedded postgres resources", st.testProvisioning)
	t.Run("creates embedded postgres resources in configured namespace", st.testProvisioningCustomNamespace)
	t.Run("rejects unmapped extensions", st.testImageUnmapped)
	t.Run("rejects extension changes after provisioning", st.testExtensionChangeRequiresRecreate)
}

func (st *embeddedDatabaseProviderSuite) testProvisioning(t *testing.T) {
	provider := st.createEmbeddedProvider(t, "embedded-"+xid.New().String(), []string{"vector", "pg_trgm"})

	st.expectProvisionedInNamespace(t, provider, st.env.Config.OperatorNamespace)
}

func (st *embeddedDatabaseProviderSuite) testProvisioningCustomNamespace(t *testing.T) {
	namespace := "embedded-" + xid.New().String()
	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}}
	NewWithT(t).Expect(st.env.Client.Create(t.Context(), ns)).To(Succeed())
	t.Cleanup(func() {
		st.env.deleteAndWait(context.Background(), t, ns)
	})

	provider := st.createEmbeddedProvider(
		t,
		"embedded-"+xid.New().String(),
		[]string{"vector", "pg_trgm"},
		withEmbeddedNamespace(namespace),
	)

	st.expectProvisionedInNamespace(t, provider, namespace)
}

func (st *embeddedDatabaseProviderSuite) testImageUnmapped(t *testing.T) {
	g := NewWithT(t)
	provider := st.createEmbeddedProvider(t, "embedded-"+xid.New().String(), []string{"postgis"})

	expectDatabaseProviderUnreachable(
		t,
		st.env,
		provider,
		"ImageUnmapped",
		"use an External provider",
	)

	statefulSet := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: dbcontroller.EmbeddedNamespace(provider, st.env.Config),
			Name:      dbcontroller.EmbeddedServiceName(provider.Name),
		},
	}
	g.Eventually(t.Context(), k8sm.Absent(st.env.Client, statefulSet)).Should(BeTrue())
}

func (st *embeddedDatabaseProviderSuite) testExtensionChangeRequiresRecreate(t *testing.T) {
	g := NewWithT(t)
	provider := st.createEmbeddedProvider(t, "embedded-"+xid.New().String(), []string{"pg_trgm"})

	st.waitReachable(t, provider)

	g.Expect(st.env.Client.Get(t.Context(), client.ObjectKeyFromObject(provider), provider)).To(Succeed())
	provider.Spec.Embedded.Extensions = []string{"pg_trgm", "pgcrypto"}
	g.Expect(st.env.Client.Update(t.Context(), provider)).To(Succeed())

	expectDatabaseProviderUnreachable(
		t,
		st.env,
		provider,
		"ExtensionChangeRequiresRecreate",
		"recreate the provider",
	)
}

func (st *embeddedDatabaseProviderSuite) createEmbeddedProvider(
	t *testing.T,
	name string,
	extensions []string,
	opts ...func(*infraApi.DatabaseProvider),
) *infraApi.DatabaseProvider {
	t.Helper()

	provider := &infraApi.DatabaseProvider{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec: infraApi.DatabaseProviderSpec{
			Type: infraApi.ProviderTypeEmbedded,
			Embedded: &infraApi.EmbeddedProviderSpec{
				DeletionPolicy: infraApi.DeletionPolicyRetain,
				Storage: infraApi.StorageSpec{
					Size: resource.MustParse("1Gi"),
				},
				Extensions: extensions,
			},
		},
	}
	for _, opt := range opts {
		opt(provider)
	}

	NewWithT(t).Expect(st.env.Client.Create(t.Context(), provider)).To(Succeed())
	t.Cleanup(func() {
		st.env.deleteAndWait(context.Background(), t, provider)
	})

	return provider
}

func (st *embeddedDatabaseProviderSuite) expectProvisionedInNamespace(
	t *testing.T,
	provider *infraApi.DatabaseProvider,
	namespace string,
) {
	t.Helper()

	g := NewWithT(t)
	st.waitReachable(t, provider)

	statefulSet := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      dbcontroller.EmbeddedServiceName(provider.Name),
		},
	}
	g.Eventually(t.Context(), k8sm.Lookup(st.env.Client, statefulSet)).To(Succeed())
	g.Expect(statefulSet.Status.ReadyReplicas).To(Equal(int32(1)))

	adminSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      dbcontroller.EmbeddedAdminSecretName(provider.Name),
		},
	}
	g.Eventually(t.Context(), k8sm.Lookup(st.env.Client, adminSecret)).To(Succeed())
	g.Expect(adminSecret.Data).To(HaveKey(dbcontroller.EmbeddedAdminSecretPasswordKey))

	networkPolicy := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      dbcontroller.EmbeddedServiceName(provider.Name),
		},
	}
	g.Eventually(t.Context(), k8sm.Lookup(st.env.Client, networkPolicy)).To(Succeed())

	current := &infraApi.DatabaseProvider{ObjectMeta: metav1.ObjectMeta{Name: provider.Name}}
	g.Eventually(t.Context(), k8sm.Lookup(st.env.Client, current)).To(Succeed())
	g.Expect(current.Labels).To(HaveKeyWithValue("db.infrastructure.opendatahub.io/capability-pgvector", "true"))
	g.Expect(current.Labels).To(HaveKeyWithValue("db.infrastructure.opendatahub.io/capability-pg_trgm", "true"))
}

func withEmbeddedNamespace(namespace string) func(*infraApi.DatabaseProvider) {
	return func(provider *infraApi.DatabaseProvider) {
		provider.Spec.Embedded.Namespace = namespace
	}
}

func (st *embeddedDatabaseProviderSuite) waitReachable(t *testing.T, provider *infraApi.DatabaseProvider) {
	t.Helper()

	NewWithT(t).Eventually(t.Context(), k8sm.Get(st.env.Client, provider)).Should(
		WithTransform(k8sm.ConditionsOf[metav1.Condition](),
			ContainElement(condition.Is(databaseprovider.ConditionReachable, metav1.ConditionTrue))),
	)
}
