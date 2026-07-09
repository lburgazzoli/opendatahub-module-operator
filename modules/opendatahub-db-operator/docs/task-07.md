# Task 07 — `DatabaseClaim` Reconciler

See `docs/plan.md` §6.

## Goal

Full `DatabaseClaim` reconciliation: resolve provider, verify the target database exists,
provision a dedicated user, SSA Secret write, always-`Retain` deletion semantics.

## Depends on

Task-03 (scaffolding + provider resolution), Task-05 (DDL layer). Independent of task-06 (parallel
work is fine — the two reconcilers share `pkg/postgres` and `providerresolve` but touch
disjoint CRDs). Same as task-06: this task's core reconciler logic does not need task-08, but its
acceptance-criteria integration tests provision against a real `Embedded` provider, so **task-08
must be done before this task's tests can pass**, even though it isn't a code dependency.

## Key files/packages

- `internal/controller/databaseclaim/databaseclaim_controller.go`
- `internal/controller/databaseclaim/finalizer.go`

## Steps

1. Resolve provider via `providerresolve.Resolve`; same not-found/not-reachable handling as
   task-06.
2. Verify `spec.database` exists on the provider's backend (a lightweight `pg_database` catalog
   query via the admin pool) — if missing, `Pending` with a condition message naming the missing
   database (per spec.md: "claim stays `Pending`... no default to resolve, unrelated to
   `deletionPolicy`"). Do not attempt to create the database — `DatabaseClaim` never creates a
   new database, only a user against an existing one.
3. Add finalizer before DDL.
4. Generate a password, `pkg/postgres.CreateDatabaseUser` — a dedicated user with broader
   privileges (`CREATE SCHEMA` on the target database) per `spec.access`.
5. SSA-write the credentials Secret (same shape/conventions as task-06 step 6,
   including `spec.secretName` override support).
6. Set `status.database` (echoes `spec.database` exactly — no defaulting logic, unlike
   `SchemaClaim.status.schema`), `Provisioned: True, reason: UserProvisioned`.
7. Deletion: no `deletionPolicy` field exists on this CRD — always drop only the provisioned user
   (`pkg/postgres.DropRole`); the database itself is never touched, since other
   claims/components may also hold users against it. The claim Secret is managed in place, without
   an owner reference, as in task-06.
8. Do not remove or bypass the `upgradeIfNeeded` action task-03 already wired ahead of this
   reconciler's real logic, same as task-06 step 9 — leave it as the no-op task-03 built unless a
   concrete migration need is identified while implementing this task.
9. No manual requeue needed here either — same as task-06 step 10, task-03's
   `WithDefaultRequeueAfter(cfg.ClaimRetryInterval)` on the `DatabaseClaim` builder already
   covers this.

## Acceptance criteria

- Integration test: `DatabaseClaim` targeting a pre-existing database on an `Embedded` provider
  succeeds end-to-end, `status.database == spec.database` exactly.
- Integration test: `spec.database` naming a non-existent database → `Pending` with a message
  naming that database, no DDL attempted.
- Integration test: two `DatabaseClaim`s against the same database each get independent users;
  deleting one does not affect the other's user or the database.
- Deletion never issues a `DROP DATABASE` under any code path — assert this directly (e.g. by
  asserting the database still exists and is queryable after claim deletion).
