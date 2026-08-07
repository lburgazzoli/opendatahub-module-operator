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

	infraApi "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/api/infrastructure/v1alpha1"
)

// SQL statement templates. Each uses %s placeholders for already-quoted
// identifiers/literals -- never for raw strings. Placing them as named
// constants keeps the DDL readable and makes the quoting contract explicit:
// every %s argument must come from QuoteIdentifier or QuoteLiteral.
const (
	sqlCreateSchemaIfNotExists = "CREATE SCHEMA IF NOT EXISTS %s"
	sqlCreateDatabase          = "CREATE DATABASE %s"
	sqlDropSchemaCascade       = "DROP SCHEMA IF EXISTS %s CASCADE"
	sqlDropRole                = "DROP ROLE IF EXISTS %s"
	sqlGrantUsageOnSchema      = "GRANT USAGE ON SCHEMA %s TO %s"
	sqlGrantCreateOnSchema     = "GRANT CREATE ON SCHEMA %s TO %s"

	sqlGrantSelectOnTables    = "GRANT SELECT ON ALL TABLES IN SCHEMA %s TO %s"
	sqlDefaultPrivGrantSelect = "ALTER DEFAULT PRIVILEGES IN SCHEMA %s GRANT SELECT ON TABLES TO %s"
	sqlGrantDMLOnTables       = "GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA %s TO %s"
	sqlDefaultPrivGrantDML    = "" +
		"ALTER DEFAULT PRIVILEGES IN SCHEMA %s " +
		"GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO %s"

	sqlGrantConnectOnDatabase = "GRANT CONNECT ON DATABASE %s TO %s"
	sqlGrantCreateOnDatabase  = "GRANT CREATE ON DATABASE %s TO %s"

	sqlCreateExtensionIfNotExists = "CREATE EXTENSION IF NOT EXISTS %s"

	sqlDatabaseExists  = "SELECT EXISTS(SELECT 1 FROM pg_database WHERE datname = $1)"
	sqlSchemaExists    = "SELECT EXISTS(SELECT 1 FROM pg_namespace WHERE nspname = $1)"
	sqlSetRolePassword = `ALTER ROLE %s WITH PASSWORD %s`
)

// sqlEnsureRole is a PL/pgSQL DO block that creates a role only when it does
// not already exist. Placeholders: 1) role-as-literal (pg_roles lookup),
// 2) role-as-identifier + password-as-literal (CREATE branch only).
const sqlEnsureRole = `DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = %s) THEN
    CREATE ROLE %s WITH LOGIN PASSWORD %s;
  END IF;
END
$$`

// CreateSchema creates a schema if it doesn't already exist. Idempotent.
func CreateSchema(ctx context.Context, cli Client, schema string) error {
	_, err := cli.Exec(ctx, fmt.Sprintf(sqlCreateSchemaIfNotExists, QuoteIdentifier(schema)))
	return err
}

// CreateDatabase creates a database if it doesn't already exist. Idempotent.
func CreateDatabase(ctx context.Context, cli Client, database string) error {
	exists, err := DatabaseExists(ctx, cli, database)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}
	_, err = cli.Exec(ctx, fmt.Sprintf(sqlCreateDatabase, QuoteIdentifier(database)))
	return err
}

// DropSchemaCascade drops a schema and all its contained objects. Used for
// DeletionPolicy=Delete (docs/plan.md §6).
func DropSchemaCascade(ctx context.Context, cli Client, schema string) error {
	_, err := cli.Exec(ctx, fmt.Sprintf(sqlDropSchemaCascade, QuoteIdentifier(schema)))
	return err
}

// EnsureRole creates a PostgreSQL login role with the given password if it does
// not already exist. It is a true CREATE-only idempotent operation — it never
// alters an existing role's password. Use SetRolePassword for explicit rotation.
func EnsureRole(ctx context.Context, cli Client, role, password string) error {
	stmt := fmt.Sprintf(sqlEnsureRole,
		QuoteLiteral(role),
		QuoteIdentifier(role), QuoteLiteral(password),
	)
	_, err := cli.Exec(ctx, stmt)
	return err
}

// SetRolePassword explicitly rotates the password of an existing PostgreSQL role.
// Callers must ensure this is intentional — rotating breaks all active connections
// that use the old credentials. Only call this when a credentials Secret has been
// lost and an explicit rotation is required to restore a known-good state.
func SetRolePassword(ctx context.Context, cli Client, role, password string) error {
	_, err := cli.Exec(ctx, fmt.Sprintf(sqlSetRolePassword, QuoteIdentifier(role), QuoteLiteral(password)))
	return err
}

// DropRole drops a PostgreSQL role if it exists. Idempotent.
func DropRole(ctx context.Context, cli Client, role string) error {
	_, err := cli.Exec(ctx, fmt.Sprintf(sqlDropRole, QuoteIdentifier(role)))
	return err
}

// GrantSchemaPrivileges grants privileges to a role on a schema according to
// the claim access mode. ReadWrite grants schema-local administration: USAGE,
// CREATE, and DML on tables. ReadOnly grants USAGE and SELECT only. It also
// sets default privileges so future admin-created tables are covered.
func GrantSchemaPrivileges(
	ctx context.Context,
	cli Client,
	schema string,
	role string,
	accessMode infraApi.AccessMode,
) error {
	q := QuoteIdentifier

	if _, err := cli.Exec(ctx, fmt.Sprintf(sqlGrantUsageOnSchema, q(schema), q(role))); err != nil {
		return fmt.Errorf("grant usage on schema: %w", err)
	}

	var tableGrant, defaultPriv string
	if accessMode == infraApi.AccessModeReadOnly {
		tableGrant = sqlGrantSelectOnTables
		defaultPriv = sqlDefaultPrivGrantSelect
	} else {
		if _, err := cli.Exec(ctx, fmt.Sprintf(sqlGrantCreateOnSchema, q(schema), q(role))); err != nil {
			return fmt.Errorf("grant create on schema: %w", err)
		}
		tableGrant = sqlGrantDMLOnTables
		defaultPriv = sqlDefaultPrivGrantDML
	}

	if _, err := cli.Exec(ctx, fmt.Sprintf(tableGrant, q(schema), q(role))); err != nil {
		return fmt.Errorf("grant on tables: %w", err)
	}
	if _, err := cli.Exec(ctx, fmt.Sprintf(defaultPriv, q(schema), q(role))); err != nil {
		return fmt.Errorf("alter default privileges: %w", err)
	}
	return nil
}

// GrantDatabasePrivileges grants privileges on a database according to the
// claim access mode. ReadOnly grants CONNECT only; ReadWrite grants both
// CONNECT and CREATE.
func GrantDatabasePrivileges(
	ctx context.Context,
	cli Client,
	database string,
	role string,
	accessMode infraApi.AccessMode,
) error {
	q := QuoteIdentifier
	if _, err := cli.Exec(ctx, fmt.Sprintf(sqlGrantConnectOnDatabase, q(database), q(role))); err != nil {
		return fmt.Errorf("grant connect on database: %w", err)
	}
	if accessMode != infraApi.AccessModeReadOnly {
		if _, err := cli.Exec(ctx, fmt.Sprintf(sqlGrantCreateOnDatabase, q(database), q(role))); err != nil {
			return fmt.Errorf("grant create on database: %w", err)
		}
	}
	return nil
}

// RevokeDatabasePrivileges revokes the privileges granted by GrantDatabasePrivileges.
func RevokeDatabasePrivileges(ctx context.Context, cli Client, database, role string) error {
	q := QuoteIdentifier
	var errs []error
	if _, err := cli.Exec(ctx, "REVOKE CREATE ON DATABASE "+q(database)+" FROM "+q(role)); err != nil &&
		!IsUndefinedObject(err) {
		errs = append(errs, err)
	}
	if _, err := cli.Exec(ctx, "REVOKE CONNECT ON DATABASE "+q(database)+" FROM "+q(role)); err != nil &&
		!IsUndefinedObject(err) {
		errs = append(errs, err)
	}
	if len(errs) == 0 {
		return nil
	}
	return fmt.Errorf("revoking database privileges (partial): %v", errs)
}

// RevokeSchemaPrivileges revokes the privileges granted by GrantSchemaPrivileges.
// All three revocations are attempted in order; errors are joined and returned
// so the caller is aware of partial failures, but revocation continues even if
// one step fails (the role or schema may already be gone during deletion).
func RevokeSchemaPrivileges(ctx context.Context, cli Client, schema, role string) error {
	q := QuoteIdentifier
	var errs []error

	if _, err := cli.Exec(
		ctx,
		"ALTER DEFAULT PRIVILEGES IN SCHEMA "+q(schema)+" REVOKE ALL ON TABLES FROM "+q(role),
	); err != nil && !IsUndefinedObject(err) {
		errs = append(errs, err)
	}

	if _, err := cli.Exec(
		ctx,
		"REVOKE ALL ON ALL TABLES IN SCHEMA "+q(schema)+" FROM "+q(role),
	); err != nil &&
		!IsUndefinedObject(err) {
		errs = append(errs, err)
	}

	if _, err := cli.Exec(
		ctx,
		"REVOKE ALL ON SCHEMA "+q(schema)+" FROM "+q(role),
	); err != nil && !IsUndefinedObject(err) {
		errs = append(errs, err)
	}

	if len(errs) == 0 {
		return nil
	}

	return fmt.Errorf("revoking schema privileges (partial): %v", errs)
}

// HasSchemaPrivileges reports whether role already holds the privileges
// expected for accessMode on schema: USAGE for ReadOnly, USAGE+CREATE for
// ReadWrite. COALESCE handles the NULL returned when the role is unknown to
// HAS_SCHEMA_PRIVILEGE (treats it as false).
//
// Note: this checks schema-level grants only (USAGE / CREATE on the schema
// itself). It does not check DML grants on individual tables that existed
// before the schema was claimed, or on tables whose default privileges were
// manually revoked. If such tables exist, HAS_SCHEMA_PRIVILEGE may return
// true while queries against those tables still fail. This is an acceptable
// bounded risk: new schemas provisioned by this operator contain no
// pre-existing tables, so the check is complete for the normal provisioning
// path. Operators who manually manipulate schema objects should verify
// per-table grants independently.
func HasSchemaPrivileges(
	ctx context.Context,
	cli Client,
	schema string,
	role string,
	accessMode infraApi.AccessMode,
) (bool, error) {
	row, err := cli.QueryRow(ctx,
		"SELECT COALESCE(HAS_SCHEMA_PRIVILEGE($1, $2, 'USAGE'), false)", role, schema,
	)
	if err != nil {
		return false, err
	}

	var hasUsage bool
	err = row.Scan(&hasUsage)
	if err != nil || !hasUsage {
		return false, err
	}
	if accessMode == infraApi.AccessModeReadOnly {
		return true, nil
	}

	row, err = cli.QueryRow(ctx,
		"SELECT COALESCE(HAS_SCHEMA_PRIVILEGE($1, $2, 'CREATE'), false)", role, schema,
	)
	if err != nil {
		return false, err
	}

	var hasCreate bool
	err = row.Scan(&hasCreate)

	return hasCreate, err
}

// HasDatabasePrivileges reports whether role already holds the privileges
// expected for accessMode on database: CONNECT for ReadOnly, CONNECT+CREATE
// for ReadWrite.
func HasDatabasePrivileges(
	ctx context.Context,
	cli Client,
	database string,
	role string,
	accessMode infraApi.AccessMode,
) (bool, error) {
	row, err := cli.QueryRow(ctx,
		"SELECT COALESCE(HAS_DATABASE_PRIVILEGE($1, $2, 'CONNECT'), false)", role, database,
	)
	if err != nil {
		return false, err
	}

	var hasConnect bool
	err = row.Scan(&hasConnect)
	if err != nil || !hasConnect {
		return false, err
	}
	if accessMode == infraApi.AccessModeReadOnly {
		return true, nil
	}
	row, err = cli.QueryRow(ctx,
		"SELECT COALESCE(HAS_DATABASE_PRIVILEGE($1, $2, 'CREATE'), false)", role, database,
	)
	if err != nil {
		return false, err
	}

	var hasCreate bool
	err = row.Scan(&hasCreate)
	return hasCreate, err
}

// DatabaseExists returns true if a database with the given name exists on the
// server. Used by DatabaseClaim to verify spec.database before provisioning.
func DatabaseExists(ctx context.Context, cli Client, name string) (bool, error) {
	row, err := cli.QueryRow(ctx, sqlDatabaseExists, name)
	if err != nil {
		return false, err
	}

	var exists bool
	err = row.Scan(&exists)
	return exists, err
}

// SchemaExists returns true if a schema with the given name exists.
func SchemaExists(ctx context.Context, cli Client, name string) (bool, error) {
	row, err := cli.QueryRow(ctx, sqlSchemaExists, name)
	if err != nil {
		return false, err
	}

	var exists bool
	err = row.Scan(&exists)
	return exists, err
}

// RoleExists returns true if a PostgreSQL role with the given name exists.
func RoleExists(ctx context.Context, cli Client, name string) (bool, error) {
	row, err := cli.QueryRow(ctx,
		"SELECT EXISTS(SELECT 1 FROM pg_roles WHERE rolname = $1)", name,
	)
	if err != nil {
		return false, err
	}

	var exists bool
	err = row.Scan(&exists)
	return exists, err
}

// CreateExtensionIfNotExists runs CREATE EXTENSION IF NOT EXISTS for each name.
// Used by the Embedded provider's bootstrap step (task-08).
func CreateExtensionIfNotExists(ctx context.Context, cli Client, extensions []string) error {
	for _, ext := range extensions {
		if _, err := cli.Exec(ctx, fmt.Sprintf(sqlCreateExtensionIfNotExists, QuoteIdentifier(ext))); err != nil {
			return fmt.Errorf("creating extension %q: %w", ext, err)
		}
	}

	return nil
}
