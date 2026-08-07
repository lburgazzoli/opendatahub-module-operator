# Task 07 — `DatabaseClaim` Reconciler

See `docs/plan.md` §6.

## Goal

Full `DatabaseClaim` reconciliation: resolve provider, determine the effective database from the
claim or provider default, optionally create an explicit database when allowed, provision a
dedicated user, SSA Secret write, always-`Retain` deletion semantics.

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
2. Resolve the effective database name from `spec.database` or the provider default database.
3. Verify that effective database exists on the provider's backend (a lightweight `pg_database`
   catalog query via the admin pool). If it is missing and came from the provider default, stay
   `Pending` with a condition message naming the missing database. If it is missing and was
   explicitly requested by the claim, create it only when the provider allows
   `spec.external.capabilities: [CreateDatabase]`; otherwise stay `Pending` with a
   `DatabaseCreateNotAllowed` condition.
4. Add finalizer before DDL.
5. Generate a password, `pkg/postgres.CreateDatabaseUser` — a dedicated user with broader
   privileges per `spec.access`:
   - `ReadOnly`: `CONNECT` only.
   - `ReadWrite`: `CONNECT` plus the ability to `CREATE SCHEMA` on the target database.
6. SSA-write the credentials Secret (same shape/conventions as task-06 step 6,
   including `spec.secretName` override support).
7. Set `status.database` to the resolved effective database and
   `Provisioned: True, reason: UserProvisioned`.
8. Deletion: no `deletionPolicy` field exists on this CRD — always drop only the provisioned user
   (`pkg/postgres.DropRole`); the database itself is never touched, since other
   claims/components may also hold users against it. The claim Secret is managed in place, without
   an owner reference, as in task-06.
9. Do not remove or bypass the `upgradeIfNeeded` action task-03 already wired ahead of this
   reconciler's real logic, same as task-06 step 9 — leave it as the no-op task-03 built unless a
   concrete migration need is identified while implementing this task.
10. No manual requeue needed here either — same as task-06 step 10, task-03's
   `WithDefaultRequeueAfter(cfg.ClaimRetryInterval)` on the `DatabaseClaim` builder already
   covers this.

## Acceptance criteria

- Integration test: `DatabaseClaim` targeting a pre-existing database on an `Embedded` provider
  succeeds end-to-end, `status.database == spec.database` exactly.
- Integration test: `DatabaseClaim` with no `spec.database` uses the provider default database and
  surfaces that resolved value in status and the generated Secret.
- Integration test: access modes are enforced with the real provisioned credentials:
  `ReadWrite` can `CREATE SCHEMA` in the target database, while `ReadOnly` cannot.
- Integration test: `spec.database` naming a non-existent database succeeds when the provider
  allows `CreateDatabase`.
- Integration test: `spec.database` naming a non-existent database without provider
  `CreateDatabase` capability → `Pending` with a message naming that database, no role Secret and
  no role creation.
- Integration test: two `DatabaseClaim`s against the same database each get independent users;
  deleting one does not affect the other's user or the database.
- Deletion never issues a `DROP DATABASE` under any code path — assert this directly (e.g. by
  asserting the database still exists and is queryable after claim deletion).
