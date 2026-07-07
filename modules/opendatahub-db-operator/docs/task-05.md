# Task 05 — PostgreSQL DDL Execution Layer

See `docs/plan.md` §8. This is the one genuinely new kind of package this module introduces
(`docs/plan.md` §12) — everything else reuses existing framework patterns.

## Goal

Build `pkg/postgres`, the shared package the `SchemaClaim`/`DatabaseClaim` reconcilers use
for every schema/role/grant DDL statement, plus the password generator shared with the
`Embedded` provider's admin-secret bootstrap (task-08).

## Depends on

Task-01 (go.mod has `jackc/pgx/v5`).

## Key files/packages

- `pkg/postgres/pool.go` — `pgxpool.Pool` cache keyed by provider name + admin-secret
  resource version; invalidated when the admin secret changes.
- `pkg/postgres/ddl.go` — statement builders: `CreateSchema`, `CreateSchemaUser`,
  `CreateDatabaseUser`, `GrantSchemaPrivileges`, `DropSchemaCascade`, `DropRole`.
- `pkg/postgres/quote.go` — identifier/literal quoting helpers.
- `pkg/postgres/password.go` — `crypto/rand`-backed password generator.

## Steps

1. `quote.go`: wrap `pgx.Identifier{name}.Sanitize()` for every identifier
   (schema/role/user/database name) interpolated into DDL; wrap `pq.QuoteLiteral()` for any
   literal embedded directly in DDL (passwords in `ALTER ROLE ... WITH PASSWORD '<literal>'`).
   Never use `fmt.Sprintf` with a raw, unquoted identifier anywhere in this package — that's the
   one security-sensitive invariant this whole layer exists to protect.
2. `password.go`: `GeneratePassword(n int) (string, error)` — `crypto/rand`, alphanumeric
   charset, same approach as Zalando postgres-operator's `pkg/util.RandomPassword`. This is the
   single implementation used both here (claim DDL) and by the `Embedded` provider's admin-secret
   bootstrap (task-08) — do not duplicate it.
3. `ddl.go` statement builders, each taking already-validated Go values (never raw user input
   bypassing `quote.go`):
   - `CreateSchema(ctx, pool, schema string) error` — `CREATE SCHEMA IF NOT EXISTS <schema>`.
   - `CreateSchemaUser(ctx, pool, schema, user, password string, access AccessMode) error` —
     `CREATE ROLE <user> WITH LOGIN PASSWORD '<password>'` then
     `GRANT USAGE, [SELECT | SELECT,INSERT,UPDATE,DELETE] ON SCHEMA <schema> TO <user>` (exact
     grant set per `ReadOnly`/`ReadWrite`; `ReadWrite` also needs default-privilege grants for
     future tables in the schema — spell out the full statement set here, don't hand-wave it).
   - `CreateDatabaseUser(ctx, pool, database, user, password string, access AccessMode) error` —
     dedicated user with `CREATE SCHEMA` privilege on the target database (`DatabaseClaim`).
   - `DropSchemaCascade(ctx, pool, schema string) error` — `DROP SCHEMA IF EXISTS <schema>
     CASCADE`.
   - `DropRole(ctx, pool, user string) error`.
   - All idempotent: `IF NOT EXISTS`/`IF EXISTS` throughout, since claim reconciliation may retry.
4. `pool.go`: `GetPool(ctx, adminSecret *corev1.Secret) (*pgxpool.Pool, error)`, cached by
   `(secret.Namespace, secret.Name, secret.ResourceVersion)` so a rotated/regenerated admin
   secret transparently gets a fresh pool without a manual invalidation call.
5. Never log a password or literal DDL statement containing one — log the statement shape
   (`"CREATE ROLE %s WITH LOGIN PASSWORD [redacted]"`) instead.

## Acceptance criteria

- Unit tests (no live DB needed) for `quote.go`: assert identifiers containing quotes,
  semicolons, or SQL keywords are safely escaped (classic SQL-injection-via-identifier test
  cases).
- Integration tests (testcontainers Postgres) for every `ddl.go` function: idempotent re-run
  produces no error; `ReadOnly` vs `ReadWrite` grants are actually enforced (attempt a write as a
  `ReadOnly` user and expect it to fail).
- `password.go` generates passwords with sufficient entropy (length + charset asserted in a
  test) and never repeats across N generations in a statistical sanity check.
- No test or log output anywhere contains a literal generated password.
