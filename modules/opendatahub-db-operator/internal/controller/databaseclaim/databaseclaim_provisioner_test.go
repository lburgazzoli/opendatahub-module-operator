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

package databaseclaim_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	. "github.com/onsi/gomega"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	infraApi "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/api/infrastructure/v1alpha1"
	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/internal/controller/databaseclaim"
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

func TestDatabaseProvisioner_Ensure(t *testing.T) {
	g := NewWithT(t)
	cfg := startPostgres(t)
	pool := openPool(t, cfg)

	claim := &infraApi.DatabaseClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example",
			Namespace: "test-ns",
		},
		Spec: infraApi.DatabaseClaimSpec{
			Database: cfg.DBName,
		},
	}
	cli := newFakeClient(t).Build()

	provisioner := databaseclaim.DatabaseProvisioner{
		Client: cli,
		Claim:  claim,
		Pool:   pool,
		Config: cfg,
	}

	secret, err := provisioner.Ensure(t.Context())
	g.Expect(err).NotTo(HaveOccurred())
	status := provisioner.ConnectionStatus()
	g.Expect(status.Database).To(Equal(cfg.DBName))
	g.Expect(status.SecretRef.Name).To(Equal(claim.Name))
	g.Expect(secret.Name).To(Equal(claim.Name))
	g.Expect(secret.Data).To(HaveKeyWithValue(postgres.SecretKeyDatabase, []byte(cfg.DBName)))

	claimCfg, err := postgres.ParseSecret(secret.Data)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(postgres.Ping(t.Context(), claimCfg)).To(Succeed())

	g.Expect(cli.Create(t.Context(), secret.DeepCopy())).To(Succeed())

	secondSecret, err := provisioner.Ensure(t.Context())
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(provisioner.ConnectionStatus()).To(Equal(status))
	g.Expect(secondSecret.Data[postgres.SecretKeyPassword]).To(Equal(secret.Data[postgres.SecretKeyPassword]))
}

func TestDatabaseProvisioner_Ensure_UsesSecretNameOverride(t *testing.T) {
	g := NewWithT(t)
	cfg := startPostgres(t)
	pool := openPool(t, cfg)

	claim := &infraApi.DatabaseClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example",
			Namespace: "test-ns",
		},
		Spec: infraApi.DatabaseClaimSpec{
			Database:   cfg.DBName,
			SecretName: "projected-creds",
		},
	}
	cli := newFakeClient(t).Build()

	provisioner := databaseclaim.DatabaseProvisioner{
		Client: cli,
		Claim:  claim,
		Pool:   pool,
		Config: cfg,
	}

	secret, err := provisioner.Ensure(t.Context())
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(secret.Name).To(Equal("projected-creds"))
	g.Expect(provisioner.ConnectionStatus().SecretRef.Name).To(Equal("projected-creds"))
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

func TestDatabaseProvisioner_Ensure_DatabaseMissing(t *testing.T) {
	g := NewWithT(t)
	cfg := startPostgres(t)
	pool := openPool(t, cfg)

	claim := &infraApi.DatabaseClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example",
			Namespace: "test-ns",
		},
		Spec: infraApi.DatabaseClaimSpec{
			Database: "missingdb",
		},
	}
	cli := newFakeClient(t).Build()

	provisioner := databaseclaim.DatabaseProvisioner{
		Client: cli,
		Claim:  claim,
		Pool:   pool,
		Config: cfg,
	}

	_, err := provisioner.Ensure(t.Context())
	g.Expect(err).To(HaveOccurred())

	var notFound databaseclaim.ErrDatabaseNotFound
	g.Expect(errors.As(err, &notFound)).To(BeTrue())
	g.Expect(notFound.Database).To(Equal("missingdb"))
}

func TestDatabaseProvisioner_Ensure_ReconcilesAccessChanges(t *testing.T) {
	g := NewWithT(t)
	cfg := startPostgres(t)
	pool := openPool(t, cfg)

	claim := &infraApi.DatabaseClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example",
			Namespace: "test-ns",
		},
		Spec: infraApi.DatabaseClaimSpec{
			Database: cfg.DBName,
			Access:   infraApi.AccessModeReadOnly,
		},
	}
	cli := newFakeClient(t).Build()

	provisioner := databaseclaim.DatabaseProvisioner{
		Client: cli,
		Claim:  claim,
		Pool:   pool,
		Config: cfg,
	}

	secret, err := provisioner.Ensure(t.Context())
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(cli.Create(t.Context(), secret.DeepCopy())).To(Succeed())

	claimCfg, err := postgres.ParseSecret(secret.Data)
	g.Expect(err).NotTo(HaveOccurred())
	rolePool := openPool(t, claimCfg)

	_, err = rolePool.Exec(
		t.Context(),
		fmt.Sprintf("CREATE SCHEMA %s", postgres.QuoteIdentifier("readonly_blocked")),
	)
	g.Expect(err).To(HaveOccurred())

	claim.Spec.Access = infraApi.AccessModeReadWrite
	_, err = provisioner.Ensure(t.Context())
	g.Expect(err).NotTo(HaveOccurred())

	schemaName := "readwrite_allowed"
	_, err = rolePool.Exec(
		t.Context(),
		fmt.Sprintf("CREATE SCHEMA %s", postgres.QuoteIdentifier(schemaName)),
	)
	g.Expect(err).NotTo(HaveOccurred())
	t.Cleanup(func() { _ = postgres.DropSchemaCascade(context.Background(), pool, schemaName) })
}
