# Task 08 — `DatabaseProvider`: `Embedded` (Focus Task)

See `docs/plan.md` §7 for the full design this task implements. This is the most detailed task in
the plan — the `Embedded` provider must be **fully functional**, not a stub.

## Depends on

Task-02 (CRD types), Task-03 (scaffolding), Task-05 (for the shared `crypto/rand` password
generator only — this task does **not** use `pkg/postgres`'s DDL/connection code at all; see
"What this task deliberately does not use" below).

This task is also the right time to firm up the integration-test foundation the remaining provider
work depends on: the suite should follow the same `gomega-matchers`/harness pattern other modules
in this repo already use, rather than expanding the current package-global/skip-based setup.

## Reference

`.context/repos/opendatahub-io/model-registry-operator@main` — clone this first if not already
present (`git clone https://github.com/opendatahub-io/model-registry-operator
.context/repos/opendatahub-io/model-registry-operator@main` from the monorepo root, matching the
existing `.context/repos/<org>/<repo>@<ref>` convention). Read before starting:

- `internal/controller/modelcatalog_controller.go` — `createOrUpdatePostgresSecret()`, the
  get-or-create-password pattern this task's admin-secret step (§2 below) mirrors.
- `internal/controller/config/templates/catalog/catalog-postgres-deployment.yaml.tmpl` — how the
  generated secret's keys feed the Postgres container via `secretKeyRef`.
- `internal/controller/config/defaults.go` — `ParseTemplates()`/`utils.RandBytes` for the general
  shape of "generate once, template the rest."

## What this task builds

### 1. Image selection (`docs/plan.md` §7.1)

A `WithDataFn` callback computing the resolved image from `spec.embedded.extensions`. The
*selection logic* (which extension set maps to which image class) is fixed in Go — use a
`switch` over the extension-class check, not an `if`/`else` chain, matching this repo's style
(`docs/plan.md` "Coding style") — but the **image reference string itself must always come from
`cfg.DefaultPgvectorImage`/`cfg.DefaultPostgresImage` (task-01's config keys), never a literal
string in this function**:

| Contains | Resolves to |
|---|---|
| `vector` | `cfg.DefaultPgvectorImage` |
| only stock-bundled extensions (`pg_trgm`, `uuid-ossp`, `pgcrypto`) | `cfg.DefaultPostgresImage` |
| anything else | error → `Reachable: False, reason: ImageUnmapped`, message tells the admin to use `External` |

**Compiled defaults are the community images (`postgres:16`, `pgvector/pgvector:pg16`), not
`registry.redhat.io` — considered and rejected.** A Red Hat-shipped PostgreSQL image
(`registry.redhat.io/rhel9/postgresql-16`) was the first instinct, since it's a supported,
Red Hat-built image consistent with RHOAI's downstream support posture — but `registry.redhat.io`
requires an entitlement pull secret that a vanilla, unauthenticated `kind` cluster does not have,
which would directly break spec.md's own constraint that this module be "testable on a plain
`kind` cluster" with "no OpenShift-specific logic." The compiled default must work out of the box
on that baseline cluster, so it stays the community image. This doesn't foreclose Red Hat images
in production: on a platform where the entitlement pull secret *is* configured (e.g. a connected
OpenShift cluster), an admin repoints `cfg.DefaultPostgresImage` via the mounted ConfigMap to
`registry.redhat.io/rhel9/postgresql-16` — this is exactly why it's a config key and not a
hardcoded literal. `cfg.DefaultPgvectorImage` stays the community `pgvector/pgvector:pg16`
regardless — no known Red Hat-shipped equivalent exists, and even if one surfaces later it would
have the same registry-auth problem for the `kind`-testable baseline.

No image override field exists on the CRD (task-02) — do not add one, even as an "advanced" or
hidden field. (The config keys above are an *operator-wide* default an admin can repoint for a
mirrored registry, e.g. via the mounted ConfigMap — not a per-`DatabaseProvider` override field on
the CRD; that distinction is what keeps this consistent with "no image override field, ever.")

### 2. Admin password — get-or-create, never regenerate (`docs/plan.md` §7.2)

`internal/controller/databaseprovider/adminsecret.go`, first action in the `Embedded` pipeline,
before any manifest staging:

1. `Get` Secret `<providerName>-admin` in the module operator's own namespace.
2. If not found, or found but missing a password key: generate via
   `pkg/postgres.GeneratePassword` (task-05's implementation — do not reimplement), SSA-write
   the Secret with keys `POSTGRES_USER`, `POSTGRES_PASSWORD`, `POSTGRES_DB` (names chosen to match
   the official image's own env vars exactly, so the StatefulSet template needs zero translation),
   owner-referenced to the `DatabaseProvider`.
3. If found with a password already present: return it unchanged. **This branch must be
   unconditionally idempotent — no code path may overwrite an existing password.** Write a unit
   test that asserts calling this twice in a row (second call with the Secret already populated)
   produces byte-identical Secret data.
4. This Secret's reference becomes the internal "admin connection secret" for this provider,
   exposed to the claim reconcilers (task-06/07 via `providerresolve`) in the same shape as
   `External`'s `spec.external.connectionSecretRef` — no `if provider.Spec.Type == Embedded`
   branching in claim code.

### 3. Manifests (`docs/plan.md` §7.3)

`assets/manifests/embedded/*.yaml.tmpl`, staged onto `rr.Templates` by a small custom action
(mirrors `modelregistry`'s `customizeManifests`), rendered by `fwtemplate.NewAction`, applied by
the standard `deploy.NewAction` (SSA) — same pipeline stage every other module uses for its
resources:

- `statefulset.yaml.tmpl` — `appsv1.StatefulSet`, 1 replica, `spec.embedded.resources` as
  container resources, env vars via `secretKeyRef` into the admin secret (§2), volume from the
  PVC below, `initdb-configmap` (below) mounted at `/docker-entrypoint-initdb.d/`.
- `pvc.yaml.tmpl` — `corev1.PersistentVolumeClaim` from `spec.embedded.storage.size` /
  `storageClassName`.
- `service.yaml.tmpl` — headless `corev1.Service`.
- `initdb-configmap.yaml.tmpl` — one `CREATE EXTENSION IF NOT EXISTS <name>;` line per entry in
  `spec.embedded.extensions`, templated into a `ConfigMap` mounted at
  `/docker-entrypoint-initdb.d/`. The official image runs every script there automatically on
  first boot of an empty data directory — **no Job, no `psql`, no `pgx` connection from the
  controller for this step.**

Template data: `.Component` (the `DatabaseProvider` instance, injected automatically by
`fwtemplate.NewAction`) plus the resolved image from the `WithDataFn` in §1.

**Documented, tested limitation**: if `spec.embedded.extensions` changes after the instance
already exists (detected by comparing desired extensions against the capability labels already
derived, §5), do not attempt a live `CREATE EXTENSION` — set
`Reachable: False, reason: ExtensionChangeRequiresRecreate` instead. Write a test asserting this
exact behavior; do not silently ignore the mismatch.

### 3b. Connection host and `NetworkPolicy` (`docs/plan.md` §6 "Required for v1")

- **Deterministic Service DNS name, never a pod IP.** The claim reconcilers (task-06/07) need a
  host for this provider; compute it as `<providerName>-postgres.<operatorNamespace>.svc`
  (matching the `service.yaml.tmpl` name from §3) rather than reading anything off the
  StatefulSet's pod(s) — there is no code path in this task or in the claim reconcilers that
  reads `pod.status.podIP` for this purpose. `<operatorNamespace>` is available from `pkg/config`
  (the module's own running namespace), the same value `DatabaseService` module status already
  uses.
- `networkpolicy.yaml.tmpl` — a fifth manifest alongside `statefulset`/`pvc`/`service`/
  `initdb-configmap`: default-deny ingress on the `Embedded` pod except the Postgres port,
  allowed only from namespaces holding a `Provisioned: True` `SchemaClaim`/`DatabaseClaim`
  against this provider. Expressed as a `namespaceSelector` matching
  `kubernetes.io/metadata.name In {ns1, ns2, ...}` — computed via a `WithDataFn` that runs the
  **same claim-listing query as idle cleanup** (§7 below), so there is exactly one piece of code
  that answers "which namespaces currently have a live claim against this provider," reused by
  both. Re-rendered/re-applied (SSA) whenever that list changes, same template+deploy pipeline as
  every other `Embedded` manifest. **Not created for `External`** — this operator has no
  visibility into or ownership of that instance's network topology.

### 4. Readiness (`docs/plan.md` §7.4)

`internal/controller/databaseprovider/readiness.go` — a custom action analogous to the
framework's `deployments.NewAction`, checking `StatefulSet.Status.ReadyReplicas == 1` instead of
a Deployment's condition.

### 5. Capability labels (`docs/plan.md` §7.5)

`internal/controller/databaseprovider/capabilities.go` — once ready, derive
`db.infrastructure.opendatahub.io/capability-*` labels from `spec.embedded.extensions` (e.g.
`vector` → `capability-pgvector: "true"`) and SSA-patch them onto the `DatabaseProvider` object.
Auto-derived only — never accept these as admin-settable input for `Embedded` providers.

### 6. `Reachable` condition state machine (`docs/plan.md` §7.6)

`Provisioning` → `True (InstanceRunning)` → failure reasons: `ImageUnmapped`,
`ExtensionChangeRequiresRecreate`, `AdminSecretUnavailable`, `Idle`.

### 7. Idle cleanup for `deletionPolicy: Delete` (`docs/plan.md` §7.7)

`internal/controller/databaseprovider/idlecleanup.go` — every reconcile, list
`SchemaClaim`/`DatabaseClaim` referencing this provider (by name or matched selector). Zero
references for longer than a grace period (`cfg.EmbeddedIdleGracePeriod`, task-01) → delete
only the StatefulSet/PVC/Service (reuse the framework's `gc`-style deletion for orphaned owned
resources), set `Reachable: False, reason: Idle`. Do **not** delete the `DatabaseProvider` object
or the admin secret. Re-provisioning on the next claim must be lazy (same manifest-staging path
as initial creation) and must reuse the still-present admin secret rather than regenerating it.

### 8. Upgrade-awareness and periodic retry

Do not remove or bypass the `upgradeIfNeeded` action task-03 already wired ahead of this
reconciler's real logic, same as task-06 step 9/task-07 step 8 — leave it as the no-op task-03
built unless a concrete migration need is identified while implementing this task (e.g. a future
change to the StatefulSet template's selector/labels would be exactly the kind of thing this hook
exists to migrate). No manual requeue needed here either — task-03's
`WithDefaultRequeueAfter(cfg.DatabaseProviderRetryInterval)` on the `DatabaseProvider` builder
already re-checks idle-cleanup's grace period (§7 above) and StatefulSet readiness on that
cadence for every successful reconcile.

## Acceptance criteria

- **Unit test** for the image-selection logic (§1): every branch of the `switch` — `vector`
  present → `cfg.DefaultPgvectorImage`; only stock extensions → `cfg.DefaultPostgresImage`;
  unmapped → error — exercised as a pure function against a fake/injected config, no cluster
  needed. This task is not complete without it (`docs/plan.md` §11 names this explicitly).
- **Unit test** for admin-secret get-or-create idempotency (§2) using a fake client: first call
  on a missing Secret generates and creates it; second call against the now-populated Secret
  returns byte-identical data and issues no write. This is in addition to, not instead of, the
  integration test below — the unit test isolates the idempotency logic itself; the integration
  test (below) proves it holds against a real running instance.
- Integration test on `kind`: create an `Embedded` `DatabaseProvider` with
  `extensions: [vector, pg_trgm]` → StatefulSet/PVC/Service created, pod becomes Ready,
  `vector` extension is actually installed (verify via a query, not just the init script having
  run), capability labels `capability-pgvector`/derived stock labels present on the
  `DatabaseProvider` object, `Reachable: True`.
- Integration test: unmapped extension (e.g. `some-nonexistent-ext`) → `Reachable: False,
  reason: ImageUnmapped`, no StatefulSet created.
- Integration test: admin secret get-or-create is idempotent across two reconciles — assert
  byte-identical Secret data and that the running instance's actual password (verified via a
  connection using the Secret's credentials) still matches after a second reconcile.
- Integration test: editing `spec.embedded.extensions` on an existing, already-ready provider →
  `Reachable: False, reason: ExtensionChangeRequiresRecreate`, existing StatefulSet/data
  untouched (not deleted, not corrupted).
- Integration test: idle cleanup — a provider with zero referencing claims past the grace period
  has its StatefulSet/PVC/Service removed, `Reachable: False, reason: Idle`, but the
  `DatabaseProvider` object and admin secret still exist; creating a new claim against it
  re-provisions successfully using the *same* admin secret (no password change).
- No test, log line, or condition message anywhere contains the admin password.
- Confirm via code review (not just tests) that this task's code never imports `pkg/postgres`
  for anything beyond the password generator — no `pgxpool`, no DDL statement builder, no live SQL
  connection anywhere in `DatabaseProvider` reconciliation.
- Confirm via code review that `status.connection.host` for an `Embedded`-backed claim is always
  the computed Service DNS name — grep for any `podIP`/`Status.PodIPs` reference anywhere in this
  task's or the claim reconcilers' code and treat a match as a bug, not a valid shortcut.
- Integration test: `NetworkPolicy` exists once the provider is `Reachable: True`, denies a
  connection attempt from an unrelated namespace with no claim against this provider, and allows
  one from a namespace with a `Provisioned: True` claim; when that claim is deleted (and no other
  claim references the provider), a follow-up reconcile removes that namespace from the
  `NetworkPolicy`'s allow list.
- No `NetworkPolicy` is created for `External` providers (task-04) — assert this directly rather
  than only by omission.
- As part of landing these tests, the integration suite foundation is refactored to:
  - add `github.com/lburgazzoli/gomega-matchers` and prefer its `k8s` / `condition` / `jq`
    matchers where they improve readability,
  - fail in `TestMain` when shared integration prerequisites cannot be established, instead of
    keeping nil globals and skipping tests via `requireSharedSetup()`,
  - use suite/env and controller-specific harness structs that hold reusable config/identifiers
    (client, namespace, provider name, postgres config), but not live Kubernetes object pointers.
