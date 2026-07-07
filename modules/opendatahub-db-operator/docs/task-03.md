# Task 03 — Shared Reconciler Scaffolding & Module Enablement CR

See `docs/plan.md` §4 and §6.

## Goal

Wire up the three `reconciler.ReconcilerFor` pipelines (empty actions for now — later tasks fill
them in) and the `DatabaseService` module-enablement reconciler using the standard action pipeline.
Implement the shared provider-resolution helper used by both claim reconcilers.

## Depends on

Task-02 (CRD types).

## Key files/packages

- `internal/controller/databaseservice/databaseservice_controller.go` — standard pipeline wiring
  (`stageManifests → upgradeIfNeeded → releases → kustomize → deploy → deployments →
  reportStatus → gc`). This module's own Deployment/RBAC/base chart are installed via Helm/OLM
  packaging (task-09), independent of this CR — same bootstrap order as every other module. This
  CR's `kustomize`/`deploy` steps apply only the module's own CRD YAML (there is no separate
  third-party operand to deploy, unlike other modules — `docs/plan.md` §4's "correction" note);
  `deployments`/`reportStatus` reflect this Deployment's own rollout status.
- `internal/controller/schemaclaim/schemaclaim_controller.go`,
  `internal/controller/databaseclaim/databaseclaim_controller.go`,
  `internal/controller/databaseprovider/databaseprovider_controller.go` — `ReconcilerFor` wiring
  with placeholder actions (`WithAction` stubs that just set a `Pending` condition), `Owns`
  declarations (`corev1.Secret` for the two claims; StatefulSet/PVC/Service for
  `DatabaseProvider`).
- `internal/controller/providerresolve/resolve.go` — shared provider-resolution helper.
- `internal/controller/upgrade/upgrade.go` — shared `NeedsUpgrade(obj)`/`StampVersion(obj)`
  helpers wrapping the `infrastructure.opendatahub.io/controller-version` annotation
  (`docs/plan.md` §6), used by all three infrastructure reconcilers' `upgradeIfNeeded` actions.

## Steps

1. `databaseservice_controller.go`: copy the standard pipeline shape from
   `opendatahub-modelregistry-operator`'s controller, but scale the manifest set down to just this
   module's own CRD YAML — do not point `stageManifests` at a Deployment/RBAC manifest for this
   operator itself; that's installed by Helm/OLM before this CR ever reconciles (`docs/plan.md`
   §4). Do not shortcut the `deployments`/`reportStatus` actions' `Ready`/`Progressing`
   computation — reuse them as-is, pointed at this Deployment's own rollout status, so
   `DatabaseService` gets the same upgrade-gating behavior (`Ready: False` while `Progressing: True`,
   `observedGeneration` tracking) every other module gets from this pipeline.
2. `providerresolve.Resolve(ctx, cli, ref ProviderRef) (*DatabaseProvider, error)`:
   - `ref.Name` set → `Get`; not found → typed not-found error the caller turns into a `Pending`
     condition.
   - `ref.Selector` set → `List` with the selector; zero matches → typed error; one match →
     return it; multiple → pick highest `db.infrastructure.opendatahub.io/selection-priority`
     annotation (parse as int, missing/invalid treated as `0`), tie-break alphabetically by
     `metadata.name`; return the winner only. The caller writes its name to `status.provider`
     (task-02's singular field, not spec.md's literal `matchedProviders` list — `docs/plan.md`
     §6 records this as a deliberate, disclosed divergence).
   - Neither set → `List` all providers, filter to
     `db.infrastructure.opendatahub.io/is-default-provider: "true"`; none → typed error.
   - Caller (task-06/07) is responsible for checking the resolved provider's `Reachable`
     condition and propagating it.
3. `internal/controller/upgrade/upgrade.go`: `NeedsUpgrade(obj client.Object) bool` (true when
   the `infrastructure.opendatahub.io/controller-version` annotation is missing or doesn't match
   the running binary's `pkg/module.Version`) and `StampVersion(obj client.Object)`
   (SSA-patches the annotation to the current version). No migration logic yet — that's added
   per-reconciler in task-06/07/08 whenever a real migration is needed; this task only builds the
   detect-and-stamp mechanism and wires its pipeline position.
4. Wire `SchemaClaim`/`DatabaseClaim`/`DatabaseProvider` reconcilers with `Owns`/`Watches` per
   `docs/plan.md` §6. Pipeline order per reconciler: object fetch (implicit) → `upgradeIfNeeded`
   (calls `NeedsUpgrade`/`StampVersion`, no actual migration behavior yet) → a placeholder action
   that calls `providerresolve.Resolve` and sets `Provisioned: False, reason: NotImplemented`.
   **`upgradeIfNeeded` runs before provider resolution, not after** — migrating an object's own
   on-disk shape doesn't depend on which provider it's pointed at, and this mirrors the standard
   pipeline's own ordering (`upgradeIfNeeded` right after the cheap `stageManifests` step, before
   any action that does real work) — `docs/plan.md` §6. Real DDL/manifest logic lands in
   task-04/06/07/08.
5. Configure periodic retry for all three infrastructure reconcilers using the framework's
   `reconciler.WithDefaultRequeueAfter` option (now landed — bump the `odh-platform-utilities`/
   `operator-actions-framework` replace directive in `go.mod` to the `all-in` branch commit that
   introduces it, per `docs/plan.md` §6, and `go mod tidy`): add
   `.WithReconcilerOpts(reconciler.WithDefaultRequeueAfter(cfg.DatabaseProviderRetryInterval))`
   to the `DatabaseProvider` builder and `...WithDefaultRequeueAfter(cfg.ClaimRetryInterval))` to
   the `SchemaClaim`/`DatabaseClaim` builders. This is a **one-time, builder-level setting** — it
   fires automatically on any successful reconcile that didn't already request a specific
   requeue, so no action in task-04/06/07/08 needs to return `RequeueAfter` itself for this
   purpose; do not add redundant manual requeues there.
6. Unit tests for `providerresolve.Resolve` covering: exact name hit/miss, selector single/zero/
   multi match, priority tie-break, alphabetical tie-break on equal priority, default-provider
   fallback, no-provider-at-all case.
7. Unit tests for `upgrade.NeedsUpgrade`/`StampVersion`: missing annotation → needs upgrade;
   annotation matching current version → does not; annotation with an older/different version →
   does; `StampVersion` sets exactly the current running version, nothing else.
8. Integration test, run against the connected cluster: apply the task-02 CRDs, create real
   `DatabaseProvider` fixtures (varying names, labels, priority annotations) and a `SchemaClaim`/
   `DatabaseClaim` referencing them by both `name` and `selector`, start the wired-up manager
   against the real cluster, and confirm: (a) the placeholder actions actually run and set
   `Provisioned: False, reason: NotImplemented` (plus `status.provider` where a selector was
   used) on the real objects; (b) the `infrastructure.opendatahub.io/controller-version`
   annotation is stamped on every reconciled object with the running binary's version; (c) the
   reconcile result carries the configured `RequeueAfter` from step 5 — not just an in-memory
   fake-client assertion.
9. Integration test for `DatabaseService` upgrade-gating, run against the connected cluster (per
   `docs/plan.md` §4): deploy `DatabaseService`, wait for `Ready: True, Progressing: False`; then
   trigger a rollout (patch the Deployment's `spec.template` — e.g. a changed env var — the same
   way a real image bump would) and assert `status.conditions[Ready]` becomes `False` with
   `Progressing: True` for the duration of the rollout, only returning to `Ready: True` once the
   new ReplicaSet is fully available and `status.observedGeneration == metadata.generation`.
   Asserting the four conditions merely exist is not sufficient — this test must actually observe
   the gate holding during a real rollout.

## Acceptance criteria

- `providerresolve_test.go` covers every branch above (Gomega dot-import, table-driven) — this
  task is not complete without it.
- `upgrade_test.go` covers every branch above — this task is not complete without it.
- **Integration tests** (steps 8 and 9) exist and pass against the connected cluster — this task
  is not complete without them.
- All three infrastructure reconcilers and the `DatabaseService` reconciler register successfully
  against a fake/envtest manager without panicking.
- Every infrastructure reconciler's pipeline places `upgradeIfNeeded` immediately after object
  fetch and **before** provider resolution and any DDL/manifest action — verify by reading the
  `WithAction` call order in each `*_controller.go`, not just that the action exists somewhere.
- `DatabaseService` observably holds `Ready: False` for the full duration of an in-progress rollout
  (step 9) — not just at steady state.
- Periodic retry (step 5) is wired for all three infrastructure reconcilers' placeholder
  actions — this task is not complete without it, since tasks 04/06/07/08 build on this being
  already in place rather than adding it themselves.
- `go build ./... && go vet ./...` succeed.
