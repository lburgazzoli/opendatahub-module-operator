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
   `modules/opendatahub-db-operator/`. Use the composite integration/e2e
   targets rather than calling the `*-run` targets directly after cleanup,
   because `cleanup-integration` and `cleanup-e2e` remove CRDs/operator state:
   ```
   make test
   make lint
   make manifests generate
   kubectl get namespace odh-db-operator-system || kubectl create namespace odh-db-operator-system
   go run sigs.k8s.io/kustomize/kustomize/v5@v5.8.1 build config/default
   go run sigs.k8s.io/kustomize/kustomize/v5@v5.8.1 build config/default | kubectl apply --dry-run=server -f -
   kubectl apply --dry-run=server -f config/rbac/schemaclaim-creator-role.yaml
   kubectl apply --dry-run=server -f config/rbac/db-consumer-role.yaml
   make test-integration
   make test-e2e
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

## Closeout Notes

### Verification Results

The following commands were run successfully during closeout on 2026-07-09:

```text
make test
make lint
make manifests generate
kubectl get namespace odh-db-operator-system || kubectl create namespace odh-db-operator-system
go run sigs.k8s.io/kustomize/kustomize/v5@v5.8.1 build config/default
go run sigs.k8s.io/kustomize/kustomize/v5@v5.8.1 build config/default | kubectl apply --dry-run=server -f -
kubectl apply --dry-run=server -f config/rbac/schemaclaim-creator-role.yaml
kubectl apply --dry-run=server -f config/rbac/db-consumer-role.yaml
make test-integration
make test-e2e
make cleanup-e2e
```

### Adversarial Review Disposition

Fixed during closeout:

- `DatabaseClaim.spec.access` now controls granted database privileges, so
  `ReadOnly` claims no longer receive `CREATE`.
- `SchemaClaim` `ReadWrite` credentials now behave as schema-local admins:
  they receive `CREATE ON SCHEMA` in addition to table DML, and the tests now
  verify they can create tables while `ReadOnly` credentials cannot.
- `postgres.Config.DSN()` now uses an escaped PostgreSQL URL instead of raw
  keyword-string concatenation, which makes passwords and usernames with
  special characters safe to parse.
- Embedded admin-secret recovery now treats a surviving PVC as evidence of an
  existing instance, preventing accidental password regeneration after a
  partial workload teardown.

Consciously deferred with written reasons:

- Default-provider fallback from `spec.md` remains intentionally unsupported:
  this branch requires `spec.provider` to be explicit, per the final API and
  tests accepted during implementation.
- Selector resolution remains sticky once a claim has bound to a matching
  provider. This was an intentional stability choice made during implementation
  and is covered by integration tests.
- Claim-side `secretName` override remains part of the shipped API by explicit
  design choice on this branch.
- Automatic drift recovery for missing claim secrets / roles / schema remains
  enabled by explicit design choice on this branch, even though the original
  spec was stricter.
- `status.matchedProviders` is still omitted; only the selected provider is
  surfaced in status.
- External providers still assume static PostgreSQL credentials in
  `connectionSecretRef`; IAM/workload-identity style auth was not implemented
  in this module.
- Embedded extension handling still uses underscore-form names such as
  `uuid_ossp`; the canonical PostgreSQL spelling `uuid-ossp` is not accepted
  by the current CRD validation.
- The embedded NetworkPolicy currently allows ingress on port `5432` before any
  claim namespaces are authorized so the operator can continue reaching the
  instance without an additional policy exception; tightening that posture was
  deferred.
