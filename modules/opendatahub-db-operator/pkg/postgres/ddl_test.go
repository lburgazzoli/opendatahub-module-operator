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

package postgres_test

import (
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	. "github.com/onsi/gomega"

	infraApi "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/api/infrastructure/v1alpha1"
	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/pkg/postgres"
)

func openPool(t *testing.T, cfg postgres.Config) *pgxpool.Pool {
	t.Helper()
	pool, err := pgxpool.New(t.Context(), cfg.DSN())
	if err != nil {
		t.Fatalf("opening pool: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

// TestDDL_CreateDropSchema verifies that CreateSchema and DropSchemaCascade
// are idempotent.
func TestDDL_CreateDropSchema(t *testing.T) {
	g := NewWithT(t)
	cfg := startPostgres(t)

	ctx := t.Context()
	pool := openPool(t, cfg)

	// First call creates the schema
	g.Expect(postgres.CreateSchema(ctx, pool, "myschema")).To(Succeed())
	// Second call is idempotent
	g.Expect(postgres.CreateSchema(ctx, pool, "myschema")).To(Succeed())

	// Drop -- also idempotent
	g.Expect(postgres.DropSchemaCascade(ctx, pool, "myschema")).To(Succeed())
	g.Expect(postgres.DropSchemaCascade(ctx, pool, "myschema")).To(Succeed())
}

// TestDDL_CreateDropRole verifies role creation and deletion.
func TestDDL_CreateDropRole(t *testing.T) {
	g := NewWithT(t)
	cfg := startPostgres(t)

	ctx := t.Context()
	pool := openPool(t, cfg)

	pw, err := postgres.GeneratePassword(24)
	g.Expect(err).NotTo(HaveOccurred())

	// Idempotent create
	g.Expect(postgres.CreateRole(ctx, pool, "testrole", pw)).To(Succeed())
	g.Expect(postgres.CreateRole(ctx, pool, "testrole", pw)).To(Succeed())

	// Verify the role can actually connect
	roleCfg := postgres.Config{
		Host: cfg.Host, Port: cfg.Port,
		User: "testrole", Password: pw, DBName: cfg.DBName,
	}
	g.Expect(postgres.Ping(ctx, roleCfg)).To(Succeed())

	// Idempotent drop
	g.Expect(postgres.DropRole(ctx, pool, "testrole")).To(Succeed())
	g.Expect(postgres.DropRole(ctx, pool, "testrole")).To(Succeed())

	// Connection with dropped role must fail
	err = postgres.Ping(ctx, roleCfg)
	g.Expect(err).To(HaveOccurred())
}

// TestDDL_GrantSchemaPrivileges verifies ReadWrite vs ReadOnly enforcement.
// Each subtest uses its own schema to avoid interference.
func TestDDL_GrantSchemaPrivileges(t *testing.T) {
	cfg := startPostgres(t)

	ctx := t.Context()
	pool := openPool(t, cfg)

	pw, _ := postgres.GeneratePassword(24)

	for _, tc := range []struct {
		name       string
		accessMode infraApi.AccessMode
		canWrite   bool
	}{
		{"readwrite", infraApi.AccessModeReadWrite, true},
		{"readonly", infraApi.AccessModeReadOnly, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)

			schema := "priv_" + tc.name
			role := "role_" + tc.name

			// Per-subtest schema with a single row
			g.Expect(postgres.CreateSchema(ctx, pool, schema)).To(Succeed())
			t.Cleanup(func() { _ = postgres.DropSchemaCascade(ctx, pool, schema) })
			_, err := pool.Exec(ctx, fmt.Sprintf("CREATE TABLE %s.t (id int)", postgres.QuoteIdentifier(schema)))
			g.Expect(err).NotTo(HaveOccurred())
			_, err = pool.Exec(ctx, fmt.Sprintf("INSERT INTO %s.t VALUES (1)", postgres.QuoteIdentifier(schema)))
			g.Expect(err).NotTo(HaveOccurred())

			g.Expect(postgres.CreateRole(ctx, pool, role, pw)).To(Succeed())
			g.Expect(postgres.GrantSchemaPrivileges(ctx, pool, schema, role, tc.accessMode)).To(Succeed())
			t.Cleanup(func() { _ = postgres.DropRole(ctx, pool, role) })

			roleCfg := postgres.Config{
				Host: cfg.Host, Port: cfg.Port,
				User: role, Password: pw, DBName: cfg.DBName,
			}
			rolePool, err := pgxpool.New(ctx, roleCfg.DSN())
			g.Expect(err).NotTo(HaveOccurred())
			defer rolePool.Close()

			// SELECT must always work
			var count int
			g.Expect(rolePool.QueryRow(ctx,
				fmt.Sprintf("SELECT count(*) FROM %s.t", postgres.QuoteIdentifier(schema)),
			).Scan(&count)).To(Succeed())
			g.Expect(count).To(Equal(1))

			// INSERT: allowed for ReadWrite, must fail for ReadOnly
			_, insertErr := rolePool.Exec(ctx,
				fmt.Sprintf("INSERT INTO %s.t VALUES (99)", postgres.QuoteIdentifier(schema)))
			if tc.canWrite {
				g.Expect(insertErr).To(Succeed(), fmt.Sprintf("readwrite role should be able to insert: %v", insertErr))
			} else {
				g.Expect(insertErr).To(HaveOccurred(), "readonly role must not be able to insert")
			}
		})
	}
}

// TestDDL_DatabaseExists verifies the helper correctly identifies existing
// and non-existing databases.
func TestDDL_DatabaseExists(t *testing.T) {
	g := NewWithT(t)
	cfg := startPostgres(t)

	ctx := t.Context()
	pool := openPool(t, cfg)

	exists, err := postgres.DatabaseExists(ctx, pool, cfg.DBName)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(exists).To(BeTrue())

	notExists, err := postgres.DatabaseExists(ctx, pool, "totally_nonexistent_db_xyz")
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(notExists).To(BeFalse())
}
