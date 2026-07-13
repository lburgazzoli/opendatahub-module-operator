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

package schemaclaim_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	. "github.com/onsi/gomega"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	infraApi "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/api/infrastructure/v1alpha1"
	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/internal/controller/schemaclaim"
	modulemanager "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/pkg/manager"
	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/pkg/postgres"
)

func startPostgres(t *testing.T) postgres.Config {
	t.Helper()
	g := NewWithT(t)

	ctr, err := tcpostgres.Run(t.Context(), "postgres:16",
		tcpostgres.WithDatabase("testdb"),
		tcpostgres.WithUsername("testuser"),
		tcpostgres.WithPassword("testpassword"),
		tcpostgres.BasicWaitStrategies(),
	)
	g.Expect(err).NotTo(HaveOccurred())

	t.Cleanup(func() {
		_ = ctr.Terminate(context.Background())
	})

	connStr, err := ctr.ConnectionString(t.Context(), "sslmode=disable")
	g.Expect(err).NotTo(HaveOccurred())

	cfg, err := postgres.ConfigFromDSN(connStr)
	g.Expect(err).NotTo(HaveOccurred())

	return cfg
}

func openPool(t *testing.T, cfg postgres.Config) *pgxpool.Pool {
	t.Helper()
	pool, err := pgxpool.New(t.Context(), cfg.DSN())
	if err != nil {
		t.Fatalf("opening pool: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func newFakeClient(t *testing.T) *fake.ClientBuilder {
	t.Helper()
	scheme, err := modulemanager.NewScheme()
	if err != nil {
		t.Fatalf("building scheme: %v", err)
	}
	return fake.NewClientBuilder().WithScheme(scheme)
}

func TestSchemaProvisioner_Ensure(t *testing.T) {
	g := NewWithT(t)
	cfg := startPostgres(t)
	pool := openPool(t, cfg)

	claim := &infraApi.SchemaClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example",
			Namespace: "test-ns",
		},
		Spec: infraApi.SchemaClaimSpec{
			Access: infraApi.AccessModeReadWrite,
		},
	}
	cli := newFakeClient(t).Build()

	provisioner := schemaclaim.SchemaProvisioner{
		Client: cli,
		Claim:  claim,
		Pool:   pool,
		Config: cfg,
	}

	schema := provisioner.Schema()
	secret, err := provisioner.Ensure(t.Context())
	g.Expect(err).NotTo(HaveOccurred())
	status := provisioner.ConnectionStatus(schema)
	g.Expect(status.Schema).To(Equal("test_ns_example"))
	g.Expect(status.Database).To(Equal(cfg.DBName))
	g.Expect(status.SecretRef.Name).To(Equal(claim.Name))
	g.Expect(secret.Name).To(Equal(claim.Name))
	g.Expect(secret.Data).To(HaveKeyWithValue(postgres.SecretKeySchema, []byte(status.Schema)))
	g.Expect(secret.Data).To(HaveKeyWithValue(postgres.SecretKeyDatabase, []byte(cfg.DBName)))

	exists, err := postgres.SchemaExists(t.Context(), pool, status.Schema)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(exists).To(BeTrue())

	claimCfg, err := postgres.ParseSecret(secret.Data)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(claimCfg.Schema).To(Equal(status.Schema))
	g.Expect(postgres.Ping(t.Context(), claimCfg)).To(Succeed())

	g.Expect(cli.Create(t.Context(), secret.DeepCopy())).To(Succeed())

	secondSecret, err := provisioner.Ensure(t.Context())
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(provisioner.ConnectionStatus(schema)).To(Equal(status))
	g.Expect(secondSecret.Data[postgres.SecretKeyPassword]).To(Equal(secret.Data[postgres.SecretKeyPassword]))
}

func TestSchemaProvisioner_Ensure_UsesSecretNameOverride(t *testing.T) {
	g := NewWithT(t)
	cfg := startPostgres(t)
	pool := openPool(t, cfg)

	claim := &infraApi.SchemaClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example",
			Namespace: "test-ns",
		},
		Spec: infraApi.SchemaClaimSpec{
			Access:     infraApi.AccessModeReadWrite,
			SecretName: "projected-creds",
		},
	}
	cli := newFakeClient(t).Build()

	provisioner := schemaclaim.SchemaProvisioner{
		Client: cli,
		Claim:  claim,
		Pool:   pool,
		Config: cfg,
	}

	schema := provisioner.Schema()
	secret, err := provisioner.Ensure(t.Context())
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(secret.Name).To(Equal("projected-creds"))
	g.Expect(provisioner.ConnectionStatus(schema).SecretRef.Name).To(Equal("projected-creds"))
	g.Expect(cli.Create(t.Context(), secret.DeepCopy())).To(Succeed())

	secondSecret, err := provisioner.Ensure(t.Context())
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(secondSecret.Name).To(Equal("projected-creds"))
	g.Expect(secondSecret.Data[postgres.SecretKeyPassword]).To(Equal(secret.Data[postgres.SecretKeyPassword]))

	overrideSecret := &corev1.Secret{}
	overrideSecret.Name = "projected-creds"
	overrideSecret.Namespace = claim.Namespace
	g.Expect(cli.Get(t.Context(), client.ObjectKeyFromObject(overrideSecret), overrideSecret)).To(Succeed())

	defaultSecret := &corev1.Secret{}
	defaultSecret.Name = claim.Name
	defaultSecret.Namespace = claim.Namespace
	g.Expect(cli.Get(t.Context(), client.ObjectKeyFromObject(defaultSecret), defaultSecret)).ToNot(Succeed())
}
