# Task 10 — Tests

See `docs/plan.md` §11.

## Goal

Add the cross-cutting, whole-module test scenarios that don't belong to any single task (multiple
CRDs interacting together, the full negative-path matrix), and wire cleanup scripts, so `make
test`, `make test-integration`, and `make test-e2e` all pass end-to-end for the whole module.
**This is not where per-task testing happens for the first time** — per `docs/plan.md` §11, tasks
01–09 each ship their own unit/integration tests as part of being complete; this task assumes
those already pass and adds only what genuinely requires the whole module assembled.

## Depends on

Tasks 01–09, each already complete with their own tests passing (per their individual acceptance
criteria) — this task adds cross-cutting coverage on top, it does not backfill anything they
should have already covered.

## Key files/packages

- `test/integration/` — full-module integration suite (all four CRDs interacting together:
  an `Embedded` provider backing both a `SchemaClaim` and a `DatabaseClaim` simultaneously).
- `test/e2e/` — e2e suite per `.agents/skills/odh-module-test` conventions.
- `hack/scripts/cleanup-integration.sh`, `cleanup-e2e.sh`.

## Steps

1. Confirm `make test`/`make test-integration` already pass from tasks 01–09 before adding
   anything new (a quick sanity run, not a re-verification of each task's own acceptance
   criteria in detail — that ownership stays with each task, per the Goal above).
2. Add cross-CRD integration coverage not naturally owned by a single task:
   - One `Embedded` `DatabaseProvider` serving both a `SchemaClaim` and a `DatabaseClaim`
     concurrently, both reaching `Provisioned: True` without interfering with each other.
   - Provider selector matching across multiple `DatabaseProvider`s with different capability
     labels and priorities, exercised through an actual `SchemaClaim` (not just the
     `providerresolve` unit test from task-03).
   - Full negative-path matrix: missing provider, unreachable `External` provider, unmapped
     `Embedded` extensions, deleted admin secret post-provisioning, deleted claim secret
     post-provisioning (confirm the documented "no recovery, delete and recreate the claim" path
     actually works end-to-end).
3. `hack/scripts/cleanup-integration.sh` / `cleanup-e2e.sh`: delete all claim CRs and `DatabaseService`
   CR first, wait for finalizer-driven DDL cleanup to complete (poll `SchemaClaim`/`DatabaseClaim`
   list until empty), only then delete the module CRDs — mirrors every other module's cleanup
   ordering (`.agents/skills/odh-module-migrate` step 7).
4. Integration CRDs installed via `make` targets, not from Go test code — tests should fail fast
   with a clear message if the expected CRDs are missing rather than hanging.
5. Wire `Makefile` composite targets: `test-integration` (setup + run), `test-e2e` (cleanup +
   deploy + run), matching every other module's target names exactly.

## Acceptance criteria

- `make test` (unit), `make test-integration` (on `kind`), and `make test-e2e` (on `kind`) all
  pass green from a clean checkout.
- Cleanup scripts leave the cluster in a state where re-running the same test suite immediately
  after also passes (no leaked CRs, Secrets, StatefulSets, or PVCs).
- Negative-path matrix from step 2 is fully covered, not just the happy path.
