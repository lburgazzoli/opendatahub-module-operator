# Task 11 — Docs, Verification & Adversarial Review

See `docs/plan.md` (whole document) and the "Adversarial review" convention this repo uses after
completing a module.

## Goal

Close out the module: README/CRD examples matching spec.md exactly, full verification gate pass,
an adversarial review comparing the implementation against `spec.md` (this module has no
monolith source to diff against, so the review target is the spec, not a prior implementation),
and updating the root `CLAUDE.md` module list.

## Depends on

Tasks 01–10.

## Steps

1. `README.md`: quickstart, CRD examples transcribed verbatim from spec.md's YAML examples
   (`SchemaClaim`, `DatabaseClaim`, `DatabaseProvider` External and Embedded) so a reader can
   copy-paste a working example.
2. Run the full verification gate, one command at a time, from
   `modules/opendatahub-db-operator/`:
   ```
   make test
   make lint
   make manifests generate
   make cleanup-integration
   make test-integration-run
   export IMG="opendatahub-db-operator:dev"
   make container-prep
   make container-build IMG="${IMG}"
   make helm
   make cleanup-e2e
   make deploy-helm IMG="${IMG}"
   make test-e2e-run
   make cleanup-e2e
   ```
3. Spawn a clean-context review agent (no access to this conversation's history) with exactly two
   inputs: `spec.md` and the final module source tree. Ask it to identify any field, behavior,
   condition reason, or edge case from spec.md that the implementation does not cover, and any
   implementation behavior not grounded in spec.md. This adapts
   `.agents/skills/odh-module-migrate/references/adversarial-review.md`'s pattern (normally a
   monolith-vs-module diff) to a spec-vs-module diff, since this module has no monolith source.
4. Spawn a second adversarial pass focused specifically on the `Embedded` provider's security
   posture: password generation entropy, no-password-in-logs, admin-secret idempotency, whether
   any code path could regenerate a password after first boot, and SQL-injection safety of every
   `internal` (see note below) `postgres` DDL call site (every identifier must go through the
   quoting helpers from task-05 — grep for any `fmt.Sprintf` building SQL directly as a red flag).
5. Address every finding from steps 3–4. Do not consider the module complete until both reviews
   have run and their findings are resolved, per this repo's standing convention.
6. Update the root `CLAUDE.md` "Current Modules" table to include `opendatahub-db-operator`, and
   classify it parallel-safe vs. sequential in the "Test Parallelism" section — this module owns
   no shared non-core CRDs with any existing module (`infrastructure.opendatahub.io` is new and
   unique to this module), so it belongs in the parallel-safe set unless review finds otherwise.

## Acceptance criteria

- All verification-gate commands in step 2 pass.
- Both adversarial reviews (steps 3–4) have run and every finding is either fixed or explicitly
  and consciously deferred with a written reason (not silently dropped).
- `README.md` examples are copy-paste-runnable against a `kind` cluster with this module
  installed.
- Root `CLAUDE.md` "Current Modules" and "Test Parallelism" sections updated.
