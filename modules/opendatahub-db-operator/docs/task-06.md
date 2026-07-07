# Task 06 — `SchemaClaim` Reconciler

See `docs/plan.md` §6.

## Goal

Full `SchemaClaim` reconciliation: resolve provider, idempotent schema+user provisioning, SSA
Secret write, `Retain`/`Delete` deletion semantics, conditions/status.

## Depends on

Task-03 (scaffolding + provider resolution + the `upgradeIfNeeded` hook wiring), Task-05 (DDL
layer). This task's core reconciler logic does not need task-08, but its acceptance-criteria
integration tests (below) provision against a real `Embedded` provider, so **task-08 must be done
before this task's tests can pass**, even though it isn't a code dependency — same kind of
ordering task-04 has with task-05 (code can be written first; tests need the other task to exist
to actually run).

## Key files/packages

- `internal/controller/schemaclaim/schemaclaim_controller.go` — replaces the task-03 placeholder
  action with the real pipeline.
- `internal/controller/schemaclaim/finalizer.go` — deletion handling.

## Steps

1. Resolve `spec.schema`: if unset, default to `${namespace}_${name}` sanitized to a valid
   PostgreSQL identifier (lowercase, non-alphanumeric → `_`, truncate/hash if it exceeds
   PostgreSQL's 63-byte identifier limit); always write the resolved value to `status.schema`.
2. Resolve provider via `providerresolve.Resolve`; not found/not reachable → `Provisioned: False`
   with the actionable message from task-03/04, requeue with backoff, return early (no finalizer
   added yet — nothing to clean up).
3. Add a finalizer (`infrastructure.opendatahub.io/schemaclaim-cleanup` or similar) before any DDL
   runs, so deletion always has cleanup to do.
4. `pkg/postgres.CreateSchema` (idempotent — succeeds whether this is the first `SchemaClaim`
   for this schema or a second one reusing it per spec.md's multi-tenant-reuse behavior).
5. Generate a password (`pkg/postgres.GeneratePassword`), `pkg/postgres.CreateSchemaUser`
   scoped to the resolved schema with `spec.access` privileges.
6. SSA-write the credentials Secret: name `== claim.Name`, claim's own namespace, owner reference
   to the `SchemaClaim`, keys for host/port/database/schema/user/password (exact key names should
   match spec.md's `status.connection` shape so consumers don't need a translation layer).
7. Set `status.connection` (host/port/database/schema + `secretRef`), `status.provider` (from
   step 2, only when a selector was used), `Provisioned: True, reason: SchemaReady`. Do not set a
   custom phase string — `common.Status.Phase` (embedded, task-02) is managed generically; this
   reconciler only ever sets conditions.
8. Deletion (finalizer logic):
   - `deletionPolicy: Retain` (default): drop only the provisioned user
     (`pkg/postgres.DropRole`); the Secret is garbage-collected automatically via its owner
     reference — no explicit delete step needed for it. Schema/data untouched.
   - `deletionPolicy: Delete`: `pkg/postgres.DropSchemaCascade` first, then `DropRole`, then
     remove the finalizer (Secret GC as above).
   - Either path: if the resolved provider no longer exists or is unreachable at deletion time,
     do not block finalizer removal indefinitely — log and remove the finalizer after a bounded
     number of retries, since a provider that's gone means the schema/user are already
     unreachable/moot (document this as an accepted edge case, not silently ignored).
9. Do not remove or bypass the `upgradeIfNeeded` action task-03 already wired ahead of this
   reconciler's real logic. If this task's DDL statements or Secret key shape differ from
   whatever a hypothetical prior controller version would have produced, that mismatch is exactly
   what the hook exists to handle in the future — leave it as the no-op task-03 built unless a
   concrete migration need is identified while implementing this task.
10. No manual requeue needed here — task-03's `WithDefaultRequeueAfter(cfg.ClaimRetryInterval)`
    on the `SchemaClaim` builder already re-checks a `Provisioned: True` claim on that cadence for
    every successful reconcile, so drift (e.g. the provisioned role or Secret disappearing
    outside this controller's own action) is eventually noticed without requiring a new event.

## Testing note

This task's acceptance-criteria integration tests run against a real `Embedded` provider
(task-08), so sequence accordingly if working through tasks out of order — but do not defer this
task's own tests to task-10; they belong here.

## Acceptance criteria

- **Unit test** for the schema-defaulting/sanitization logic in step 1 (pure function, no cluster
  needed): `${namespace}_${name}` construction, lowercasing, non-alphanumeric→`_` substitution,
  and the 63-byte-limit truncate/hash behavior — this task is not complete without it.

- Integration test: create a `SchemaClaim` against an `Embedded` provider (task-08) end-to-end →
  `Provisioned: True`, Secret exists with working credentials, `status.schema` populated
  correctly for both explicit and defaulted `spec.schema`.
- Integration test: two `SchemaClaim`s targeting the same resolved schema name both succeed, each
  getting its own user (multi-tenant reuse).
- Integration test: `deletionPolicy: Retain` leaves schema/data intact after claim deletion,
  recreating the claim reuses the schema and issues a brand-new user/password (old credentials
  confirmed no longer valid).
- Integration test: `deletionPolicy: Delete` removes the schema entirely.
- Negative-path test: provider missing → `Pending` with actionable message, no panic, no
  finalizer stuck.
