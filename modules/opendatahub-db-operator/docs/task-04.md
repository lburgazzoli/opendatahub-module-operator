# Task 04 — `DatabaseProvider`: `External`

See `docs/plan.md` §6.

## Goal

Implement `External` provider reconciliation: validate connectivity against
`spec.external.connectionSecretRef` and set the `Reachable` condition. No lifecycle management —
the controller never owns this instance.

## Depends on

Task-03 (reconciler scaffolding), Task-05 (DDL layer — needed to actually open a connection;
if sequencing is more convenient, this task can stub the connectivity check behind the same
interface task-05 finalizes and be revisited once task-05 lands).

## Key files/packages

- `internal/controller/databaseprovider/external.go` — connectivity-check action.
- `pkg/postgres/connect.go` — thin `pgxpool.New`-based "can we connect" helper, shared with
  the claim reconcilers' connection-pool cache (`docs/plan.md` §8).

## Steps

1. Read `spec.external.connectionSecretRef` (a `corev1.SecretReference` — cross-namespace,
   admin-controlled, unlike claim secrets which are always same-namespace).
2. Parse the Secret's connection fields (host, port, user, password — exact key names TBD per
   spec.md's "exact schema is DB-server-specific and validated at reconcile time"; for v1,
   standard keys `host`/`port`/`user`/`password`/`dbname` are sufficient, IAM/workload-identity
   variants are future work not blocking this task).
3. Open a short-lived `pgxpool` connection (single connection, immediate ping, then close) —
   this is a liveness check, not a long-lived pool; the claim reconcilers own the long-lived pool
   per resolved provider.
4. On success: `Reachable: True, reason: ConnectionVerified`. On failure: `Reachable: False` with
   the underlying error surfaced in the condition message (sanitized — never include the
   password).
5. Capability labels on `External` providers are admin-set directly (not derived) — no action
   needed here; the controller trusts them as-is per spec.md ("a wrong label means schema/claim
   provisioning succeeds, but the component's own `CREATE EXTENSION` call fails downstream").
6. No manual requeue needed here — task-03's `WithDefaultRequeueAfter(cfg.
   DatabaseProviderRetryInterval)` on the `DatabaseProvider` builder already re-checks this
   reconciler on that cadence for every successful reconcile, so a previously-unreachable
   `External` provider is re-checked and can recover to `Reachable: True` without needing an
   unrelated event.

## Acceptance criteria

- **Unit tests** for the Secret-field parsing (step 2): missing key, empty value, and
  malformed port each produce a clear, typed error before any connection is attempted — this
  task is not complete without them.
- **Integration test** against a testcontainers-launched Postgres, exercised at the connectivity-
  helper level: valid secret → success; wrong password → error with a message that does not leak
  the password; unreachable host → error with a timeout-appropriate reason.
- **Integration test**, run against the connected cluster, exercised at the reconciler level (not
  just the helper): create a real `DatabaseProvider{spec.type: External}` CR pointing at a
  testcontainers-launched (or otherwise reachable) Postgres and confirm the controller itself
  sets `status.conditions[Reachable]` accordingly on the live object — the helper-level test above
  is necessary but not sufficient; this task is not complete without proof the reconciler wires
  the helper's result into the CR's actual status.
- No connection pool is left open after the check (verify via testcontainers connection-count
  query, or by asserting the helper's `Close()` is always called, including on error paths).
