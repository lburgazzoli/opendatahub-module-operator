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

package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// SQL statement templates. Each uses %s placeholders for already-quoted
// identifiers/literals -- never for raw strings. Placing them as named
// constants keeps the DDL readable and makes the quoting contract explicit:
// every %s argument must come from QuoteIdentifier or QuoteLiteral.
const (
	sqlCreateSchemaIfNotExists = "CREATE SCHEMA IF NOT EXISTS %s"
	sqlDropSchemaCascade       = "DROP SCHEMA IF EXISTS %s CASCADE"
	sqlDropRole                = "DROP ROLE IF EXISTS %s"
	sqlGrantUsageOnSchema      = "GRANT USAGE ON SCHEMA %s TO %s"

	sqlGrantSelectOnTables    = "GRANT SELECT ON ALL TABLES IN SCHEMA %s TO %s"
	sqlDefaultPrivGrantSelect = "ALTER DEFAULT PRIVILEGES IN SCHEMA %s GRANT SELECT ON TABLES TO %s"
	sqlGrantDMLOnTables       = "GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA %s TO %s"
	sqlDefaultPrivGrantDML    = "" +
		"ALTER DEFAULT PRIVILEGES IN SCHEMA %s " +
		"GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO %s"
	sqlDefaultPrivRevokeOnTables = "ALTER DEFAULT PRIVILEGES IN SCHEMA %s REVOKE ALL ON TABLES FROM %s"
	sqlRevokeAllOnTables         = "REVOKE ALL ON ALL TABLES IN SCHEMA %s FROM %s"
	sqlRevokeAllOnSchema         = "REVOKE ALL ON SCHEMA %s FROM %s"

	sqlGrantConnectOnDatabase = "GRANT CONNECT ON DATABASE %s TO %s"
	sqlGrantCreateOnDatabase  = "GRANT CREATE ON DATABASE %s TO %s"

	sqlCreateExtensionIfNotExists = "CREATE EXTENSION IF NOT EXISTS %s"

	sqlDatabaseExists = "SELECT EXISTS(SELECT 1 FROM pg_database WHERE datname = $1)"
)

// sqlCreateOrUpdateRole is a PL/pgSQL DO block because CREATE ROLE has no
// IF NOT EXISTS clause. Placeholders (filled via fmt.Sprintf before exec):
// 1) role-as-literal (pg_roles lookup), 2) role-as-identifier + password-as-literal
// (CREATE branch), 3) role-as-identifier + password-as-literal (ALTER branch).
// Kept outside the const block to avoid breaking the backtick indentation.
const sqlCreateOrUpdateRole = `DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = %s) THEN
    CREATE ROLE %s WITH LOGIN PASSWORD %s;
  ELSE
    ALTER ROLE %s WITH PASSWORD %s;
  END IF;
END
$$`

// CreateSchema creates a schema if it doesn't already exist. Idempotent.
func CreateSchema(ctx context.Context, pool *pgxpool.Pool, schema string) error {
	_, err := pool.Exec(ctx, fmt.Sprintf(sqlCreateSchemaIfNotExists, QuoteIdentifier(schema)))
	return err
}

// DropSchemaCascade drops a schema and all its contained objects. Used for
// DeletionPolicy=Delete (docs/plan.md §6).
func DropSchemaCascade(ctx context.Context, pool *pgxpool.Pool, schema string) error {
	_, err := pool.Exec(ctx, fmt.Sprintf(sqlDropSchemaCascade, QuoteIdentifier(schema)))
	return err
}

// CreateRole creates a PostgreSQL login role with the given password. Idempotent.
func CreateRole(ctx context.Context, pool *pgxpool.Pool, role, password string) error {
	stmt := fmt.Sprintf(sqlCreateOrUpdateRole,
		QuoteLiteral(role),
		QuoteIdentifier(role), QuoteLiteral(password),
		QuoteIdentifier(role), QuoteLiteral(password),
	)
	_, err := pool.Exec(ctx, stmt)
	return err
}

// DropRole drops a PostgreSQL role if it exists. Idempotent.
func DropRole(ctx context.Context, pool *pgxpool.Pool, role string) error {
	_, err := pool.Exec(ctx, fmt.Sprintf(sqlDropRole, QuoteIdentifier(role)))
	return err
}

// GrantSchemaPrivileges grants privileges to a role on a schema. For ReadWrite,
// it grants USAGE and all DML. For ReadOnly, it grants USAGE and SELECT only.
// It also sets default privileges so future tables are covered.
func GrantSchemaPrivileges(ctx context.Context, pool *pgxpool.Pool, schema, role string, readOnly bool) error {
	q := QuoteIdentifier

	if _, err := pool.Exec(ctx, fmt.Sprintf(sqlGrantUsageOnSchema, q(schema), q(role))); err != nil {
		return fmt.Errorf("grant usage on schema: %w", err)
	}

	var tableGrant, defaultPriv string
	if readOnly {
		tableGrant = sqlGrantSelectOnTables
		defaultPriv = sqlDefaultPrivGrantSelect
	} else {
		tableGrant = sqlGrantDMLOnTables
		defaultPriv = sqlDefaultPrivGrantDML
	}

	if _, err := pool.Exec(ctx, fmt.Sprintf(tableGrant, q(schema), q(role))); err != nil {
		return fmt.Errorf("grant on tables: %w", err)
	}
	if _, err := pool.Exec(ctx, fmt.Sprintf(defaultPriv, q(schema), q(role))); err != nil {
		return fmt.Errorf("alter default privileges: %w", err)
	}
	return nil
}

// GrantDatabasePrivileges grants a role CONNECT and CREATE on a database.
// Used by DatabaseClaim (broader privileges than schema-scoped user).
func GrantDatabasePrivileges(ctx context.Context, pool *pgxpool.Pool, database, role string) error {
	q := QuoteIdentifier
	if _, err := pool.Exec(ctx, fmt.Sprintf(sqlGrantConnectOnDatabase, q(database), q(role))); err != nil {
		return fmt.Errorf("grant connect on database: %w", err)
	}
	if _, err := pool.Exec(ctx, fmt.Sprintf(sqlGrantCreateOnDatabase, q(database), q(role))); err != nil {
		return fmt.Errorf("grant create on database: %w", err)
	}
	return nil
}

// RevokeSchemaPrivileges revokes the privileges granted by GrantSchemaPrivileges.
// All three revocations are attempted in order; errors are joined and returned
// so the caller is aware of partial failures, but revocation continues even if
// one step fails (the role or schema may already be gone during deletion).
func RevokeSchemaPrivileges(ctx context.Context, pool *pgxpool.Pool, schema, role string) error {
	q := QuoteIdentifier
	var errs []error
	for _, stmt := range []string{
		fmt.Sprintf(sqlDefaultPrivRevokeOnTables, q(schema), q(role)),
		fmt.Sprintf(sqlRevokeAllOnTables, q(schema), q(role)),
		fmt.Sprintf(sqlRevokeAllOnSchema, q(schema), q(role)),
	} {
		if _, err := pool.Exec(ctx, stmt); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) == 0 {
		return nil
	}
	return fmt.Errorf("revoking schema privileges (partial): %v", errs)
}

// DatabaseExists returns true if a database with the given name exists on the
// server. Used by DatabaseClaim to verify spec.database before provisioning.
func DatabaseExists(ctx context.Context, pool *pgxpool.Pool, name string) (bool, error) {
	var exists bool
	err := pool.QueryRow(ctx, sqlDatabaseExists, name).Scan(&exists)
	return exists, err
}

// RoleExists returns true if a PostgreSQL role with the given name exists.
func RoleExists(ctx context.Context, pool *pgxpool.Pool, name string) (bool, error) {
	var exists bool
	err := pool.QueryRow(ctx,
		"SELECT EXISTS(SELECT 1 FROM pg_roles WHERE rolname = $1)", name,
	).Scan(&exists)
	return exists, err
}

// CreateExtensionIfNotExists runs CREATE EXTENSION IF NOT EXISTS for each name.
// Used by the Embedded provider's bootstrap step (task-08).
func CreateExtensionIfNotExists(ctx context.Context, pool *pgxpool.Pool, extensions []string) error {
	for _, ext := range extensions {
		if _, err := pool.Exec(ctx, fmt.Sprintf(sqlCreateExtensionIfNotExists, QuoteIdentifier(ext))); err != nil {
			return fmt.Errorf("creating extension %q: %w", ext, err)
		}
	}
	return nil
}
