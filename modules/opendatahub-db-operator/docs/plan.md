# RHAI Database System Service — Implementation Plan

Source of truth: `/home/luca/work/rh/raw/arch/rhai-database-service/spec.md`. This document
translates that spec into an implementation plan for `modules/opendatahub-db-operator`, reusing
this repo's existing module-operator architecture wherever possible. Task-by-task breakdown lives
in `docs/task-01.md` … `docs/task-11.md`, summarized here. **Status is updated as each task
completes** — flip `Pending` to `Done` in this table when a task's own acceptance criteria
(including its required tests, per §11) are met, so this table stays the single at-a-glance
source of implementation progress:

| # | Task | Focus | Status |
|---|------|-------|--------|
| 01 | Module scaffold | Directory layout, Makefile/Containerfile, `cmd/`, `pkg/config` (incl. image and retry-interval config keys), `pkg/manager`, `pkg/resources/gvk`, `config/{crd,rbac,manager,default}`, module descriptor skeleton, `go.mod` additions (`pgx/v5`) | Done |
| 02 | CRD API types | `SchemaClaim`, `DatabaseClaim`, `DatabaseProvider` (`External`+`Embedded`) types, schema/CEL validation, status/condition wiring, deepcopy, generated CRD YAML | Done |
| 03 | Shared reconciler scaffolding & module enablement CR | `reconciler.ReconcilerFor` wiring for all 3 kinds; provider-resolution helper; `upgradeIfNeeded`/periodic-retry wiring; the `DatabaseService` module-enablement CR | Done |
| 04 | `DatabaseProvider` — `External` | Connectivity-check action, `Reachable` condition, admin-secret parsing | Pending |
| 05 | PostgreSQL DDL layer (`pkg/postgres`) | `pgxpool` management, identifier/literal quoting, password generation, schema/role/grant/drop statement builders | Done |
| 06 | `SchemaClaim` reconciler | Idempotent schema+user provisioning, SSA Secret write, `Retain`/`Delete` finalizer logic, conditions/status | Pending |
| 07 | `DatabaseClaim` reconciler | Dedicated-user provisioning against a pre-existing database, SSA Secret write, always-Retain finalizer, conditions/status | Pending |
| 08 | `DatabaseProvider` — `Embedded` (focus task) | Image mapping via config keys, templated StatefulSet/PVC/Service/`initdb`-ConfigMap/NetworkPolicy, admin-secret get-or-create, readiness, capability labels, idle cleanup | Pending |
| 09 | RBAC, packaging, Helm chart | Kubebuilder RBAC markers, `make manifests generate helm`, module descriptor + `component_metadata.yaml`, consumer-facing RBAC examples | Pending |
| 10 | Tests | Cross-cutting, whole-module integration scenarios not owned by any single task; cleanup scripts | Pending |
| 11 | Docs, verification & adversarial review | README/CRD examples, full verification gate, adversarial review vs. spec.md, root `CLAUDE.md` module-list update | Pending |

## 1. Context & Problem

RHOAI has 7+ components each with their own database requirement, engine, connection config
mechanism, and secret naming convention (see
[rhoai-database-landscape.md](../../../references/wiki/rhoai/rhoai-database-landscape.md) for the
fragmentation analysis this service addresses). This module models database provisioning the way
Kubernetes models storage:

| Storage layer | Database layer |
|---|---|
| `PersistentVolumeClaim` | `SchemaClaim` or `DatabaseClaim` CR |
| `PersistentVolume` | provisioned schema/database + user |
| `StorageClass` | `DatabaseProvider` CR (backend + provisioning policy) |
| Volume provisioner (CSI driver) | this module's controller |
| `PVC.status.volumeName` | `claim.status.connection` |
| Pod gates on PVC `Bound` | module gates on `claim.status.conditions[Provisioned]` |

A requester (a module's controller) declares need via a claim CR and blocks on it; this
controller resolves supply against an admin-configured `DatabaseProvider`; the binding is
surfaced in status, never injected into the requester's Deployment/ConfigMap/env vars.

## 2. Scope & Non-Goals

- **`Embedded` is a convenience, not a DBaaS**: single instance, no HA, no custom tuning, no
  backup/restore. Anything beyond that scope uses `External`.
- **Components with genuinely per-component database requirements keep their own config** —
  this service targets the common case (schema-or-database + user + credentials), not every
  possible per-component need.
- **Out of scope for this plan**: credential rotation (spec.md marks this out of scope entirely —
  the only recovery path for a lost credentials Secret is deleting and recreating the claim),
  connection pooling (PgBouncer, if ever needed, is a future `DatabaseProvider` concern), and
  cross-component schema migration on upgrade.
- **Not a design question here**: multi-tenancy isolation (GPUaaS-style tenant schema separation)
  and the DSPO MariaDB→PostgreSQL migration are advanced cases already covered by the existing
  mechanism — a tenant or component with those needs uses an `External` provider or keeps its own
  per-component database config (per §"Scope" above), not a new capability this module has to
  grow.
- **Explicitly not this module's job**: spec.md describes the ODH Operator (or the
  `DSCInitialization` reconciler) auto-creating a default `Embedded` `DatabaseProvider` the first
  time a claim appears and none exists. That auto-creation logic belongs to the ODH Operator, not
  to this module — this module's controller only reconciles `DatabaseProvider` objects that
  already exist, regardless of who or what created them. Called out explicitly here (rather than
  silently omitted) so a reader doesn't wonder whether this plan considered it.

## 3. Module Scaffold

This module follows the same per-module structure documented in
`.agents/skills/odh-module-migrate/references/migration-plan.md` and used by every module under
`modules/` (`opendatahub-ray-operator`, `opendatahub-modelregistry-operator`, etc.):

```
modules/opendatahub-db-operator/
  go.mod, go.sum                         # already scaffolded
  Makefile, Containerfile
  api/infrastructure/v1alpha1/           # SchemaClaim, DatabaseClaim, DatabaseProvider types
  api/services/v1alpha1/                 # DatabaseService module-enablement CR (§4)
  cmd/
    main.go
    operator/operator.go
    chartgen/                            # copied structure, retargeted to this module's GVKs
  internal/controller/
    databaseservice/                          # DatabaseService module-enablement reconciler (§4)
    schemaclaim/                         # SchemaClaim reconciler (§6, §8, task-06)
    databaseclaim/                       # DatabaseClaim reconciler (§6, §8, task-07)
    databaseprovider/                    # DatabaseProvider reconciler: External + Embedded (§6, §7)
  pkg/postgres/                     # pgx DDL layer (§8, task-05) — the one new kind of package
  pkg/config/, pkg/manager/
  pkg/resources/gvk/gvk.go               # module-local GVK registry
  assets/manifests/
    module.yaml, component_metadata.yaml # module descriptor (packaging, §10)
    embedded/*.yaml.tmpl                 # Embedded StatefulSet/PVC/Service/initdb-ConfigMap/NetworkPolicy (§7)
  config/{crd/bases,rbac,manager,chart,default}
  test/{integration,e2e,support}
```

The above mirrors every other module's structure exactly, like for other modules — nothing in
this layout is unique to `db-operator`.

Notes:

- `go.mod`/`go.sum` already exist with the `odh-platform-utilities`/`opendatahub-operator`
  replace directives matching every other module. Task-01 adds `github.com/jackc/pgx/v5`.
- **No `get-manifests.sh` / upstream operand fetch.** Every other module vendors an upstream OSS
  project's manifests (KubeRay, Kubeflow Model Registry, ...). This module has no upstream
  project to wrap — its "operand" is code written for it, in this repo. This is a difference in
  *content* (nothing to fetch), not in *mechanism* (packaging still goes through the same
  Kustomize/Helm/`component_metadata.yaml` flow).
- `pkg/postgres` is the only genuinely new kind of package this module introduces relative
  to existing modules — see §12 (Divergences).

**Coding style: match other modules', don't invent a new one.** Same package-layout conventions
throughout (`api/<group>/v1alpha1`, `internal/controller/<name>/<name>_controller.go` +
`<name>_actions.go` + `<name>_test.go`, `pkg/` for cross-cutting shared code — mirrors
`odh-module-migrate/references/migration-plan.md`'s "Per-Module Structure"). Prefer `switch` over
an `if`/`else if` chain for genuine multi-branch dispatch, matching the style already used
elsewhere in this repo's modules — concretely in this plan: `DatabaseProvider.spec.type`
dispatch (`External` vs `Embedded`, §6), the extensions→image-class selection (§7.1), and
`AccessMode`-driven grant-statement selection (`ReadWrite` vs `ReadOnly`, §8) are all genuine
enumerated-type dispatch and should be `switch` statements, not `if`/`else` chains.

## 4. Module Enablement CR (`DatabaseService`)

So the ODH Operator can enable/disable/gate on this module exactly like any other, a thin
singleton CR (`services.platform.opendatahub.io/v1alpha1`, `Kind: DatabaseService`) is reconciled
with the **standard** pipeline every other module uses:

```
stageManifests → upgradeIfNeeded → releases → kustomize → deploy → deployments → reportStatus → gc
```

**Correction to an earlier draft of this section**: this CR does **not** deploy "its own
Deployment" — that would be circular (the Deployment already has to be running to reconcile
anything). Bootstrap order is identical to every other module: this operator's own
Deployment/RBAC/base chart is installed via Helm/OLM packaging (task-09's `config/chart`),
independent of any CR reconcile loop, exactly like `opendatahub-modelregistry-operator`'s own
Deployment is installed before a `ModelRegistry` CR ever exists. Where this module's `DatabaseService`
CR genuinely differs from other modules' Module CRs: every other module's Module CR reconcile
deploys a **separate third-party operand** (Kubeflow Model Registry, KubeRay, ...) via
`kustomize`/`deploy` — that's the actual "work" the pipeline does. `DatabaseService` has no such
operand (§3) — the "work" is the claim/provider reconciliation already running continuously in
the same process, not something toggled by a manifest-deploy step. So `DatabaseService`'s pipeline is
correspondingly thinner: its `stageManifests`/`kustomize`/`deploy` steps apply only this module's
own CRD YAML (so CRD installation is itself gated by, and observable through, this CR — a
legitimate use of the same mechanism, just with a much smaller manifest set than other modules
have), and `deployments`/`reportStatus` reflect this Deployment's *own* rollout status (a
Deployment reading and reporting its own `status.conditions` is not circular — it's the same
self-observation every module's `deployments` action already does). Status still follows the
Module Operator Contract's `PlatformObject` shape: `observedGeneration`, `version`, `releases`,
and the four conditions `Ready` / `ProvisioningSucceeded` / `Degraded` / `Progressing`. This CR is
unrelated to the three infrastructure CRDs below — it only gates whether the controller-manager
binary that reconciles them is installed and healthy.

**Future consideration — splitting into two subcommands (not required now).** `DatabaseService`'s
job, per the correction above, is genuinely thin (CRD presence + self-rollout status) and rarely
changing. The three infrastructure reconcilers are the opposite: broad RBAC (cross-namespace
Secrets, arbitrary-host network egress to admin-specified PostgreSQL endpoints, cluster-wide
StatefulSet/PVC management), and continuously active, higher-risk work (live DB connections,
DDL execution). Running both in one process means a hang in claim reconciliation (e.g., a stuck
`pgxpool` dial to an unreachable `External` host) shares fate with the process that reports
whether this module is healthy — the exact signal the Module Operator Contract's runlevel gating
(§4 above) depends on for *every other module on the platform*, not just this one. Splitting into
two subcommands (e.g. `operator module` running only the `DatabaseService` reconciler, `operator
controllers` running the three infrastructure reconcilers) would let each have independently
scoped RBAC and an independent failure domain, directly serving the Module Operator Contract's
own "minimal RBAC, avoid aggregating ClusterRoles" and resilience principles. Weighed against
that: no other module in this repo splits this way — doing it now means inventing new packaging
(two Deployments, two RBAC ClusterRoles, two leader-election locks, changes to the Helm
chartgen/`odh-manifest-audit` tooling's one-Deployment-per-module assumption) for a benefit that
is speculative until this module is actually running under load. **Decision: do not build the
split now** — but keep the seam open at near-zero cost by continuing to keep the `DatabaseService`
reconciler (`internal/controller/databaseservice/`) and the three infrastructure reconcilers
(`internal/controller/{schemaclaim,databaseclaim,databaseprovider}/`) in separate packages with
separately-scoped RBAC markers (already the plan, §6 and task-09) rather than merging them for
convenience. If claim-reconciliation RBAC scope or reliability becomes a concrete concern later,
splitting becomes a `cmd/` wiring change — using the same Cobra subcommand pattern every module
already uses for `chartgen` (`root.AddCommand(...)`) — not a rewrite.

**Must respect the Module Operator Contract's upgrade gating, not just its status shape.** The
contract requires runlevel advancement only when `observedGeneration == metadata.generation` AND
`Ready == True` AND `Progressing == False` — `Ready` must be `False` whenever `Progressing` is
`True`. Concretely for `DatabaseService`: while a new controller-manager image is rolling out (the
standard pipeline's `deployments` action already derives `Progressing`/`Ready` from the
Deployment's own rollout status, same as every other module — reuse that, don't recompute it),
`Ready` must not flip `True` until the rollout is fully complete, `observedGeneration` matches
the current spec generation, and — per the contract's admin-acknowledgment-gate mechanism — any
declared pre-upgrade prerequisites for this module have been validated. This module doesn't
currently define any admin-acknowledgment gates (no destructive migration is expected between
early versions), but the mechanism must exist and be exercised by `upgradeIfNeeded` (task-03) so
one can be added later without restructuring the pipeline, exactly like the version-migration
hook in §6. Task-03 must test this explicitly: simulate an in-progress rollout (e.g., bump the
Deployment's `spec.template` while an old ReplicaSet is still scaling down) and assert
`DatabaseService.status.conditions[Ready]` stays `False` with `Progressing: True` until the rollout
settles — asserting the four conditions merely *exist* is not sufficient coverage.

## 5. CRD Design

Full field tables are in spec.md; summarized here with the status/condition contract expressed
via this repo's existing `common.Status` embedding pattern (reused, not reinvented). **All four
CRDs embed both `common.Status` and `common.ComponentReleaseStatus`**, not just `DatabaseService`
— `reconciler.ReconcilerFor[T]`'s generic constraint is the full `common.PlatformObject`
interface (status + conditions + releases), so `SchemaClaim`/`DatabaseClaim`/`DatabaseProvider`
need the release-status plumbing structurally to typecheck against that builder in task-03, even
though "releases" has no obvious semantic meaning for a claim (task-02).

**Validate at the CRD layer wherever possible.** Anything expressible as an OpenAPI schema
constraint or CEL `+kubebuilder:validation:XValidation` rule (required fields, enums, string
patterns, mutual exclusivity, immutability of fields that shouldn't change post-creation) is
rejected by `kube-apiserver` at admission time, before a reconcile loop ever runs — this is
strictly better than the same check written imperatively in Go, since it self-documents in the
CRD schema, needs no reconciler code path, and gives the user immediate feedback from `kubectl
apply` instead of an async `Pending` condition. Reconciler-side Go validation (task-06/07/08) is
reserved for what the CRD schema genuinely cannot express: whether a referenced object exists in
the cluster, whether a provider is reachable, whether a requested extension combination maps to a
known image — anything that depends on runtime cluster state rather than the shape of the object
itself. Task-02 enumerates the specific schema/CEL rules for each CRD.

### `SchemaClaim` (namespace-scoped)

| Field | Notes |
|---|---|
| `spec.provider.name` / `spec.provider.selector` | mutually exclusive; selector matched against `DatabaseProvider` capability labels |
| `spec.schema` | optional; defaults to `${namespace}_${name}`, sanitized to a valid PostgreSQL identifier |
| `spec.access` | `ReadWrite` (default) \| `ReadOnly` |
| `spec.deletionPolicy` | `Retain` (default) \| `Delete` |
| `status.schema` | always populated, whether from `spec.schema` or the default |
| `status.connection.secretRef` | `corev1.LocalObjectReference`, `.name == metadata.name`, no namespace field (always claim's own namespace) |
| `status.provider` | the single `DatabaseProvider` ultimately selected; populated when `spec.provider.selector` is used (task-02: singular, not a list — a claim binds to exactly one provider, so there's nothing to gain from also surfacing the candidates that lost) |

### `DatabaseClaim` (namespace-scoped)

| Field | Notes |
|---|---|
| `spec.provider.name` / `spec.provider.selector` | same as `SchemaClaim` |
| `spec.database` | **required, always** — no default; must name a pre-existing database |
| `spec.access` | `ReadWrite` (default) \| `ReadOnly` |
| (no `deletionPolicy`) | always `Retain` semantics — the database pre-exists and isn't exclusively owned by this claim |
| `status.database` | echoes `spec.database` |
| `status.connection.secretRef` | same shape as `SchemaClaim` |
| `status.provider` | same as `SchemaClaim.status.provider` |

### `DatabaseProvider` (cluster-scoped)

`spec.type: External | Embedded`, mutually exclusive sub-specs. Capability labels
(`db.infrastructure.opendatahub.io/capability-*`) advertise what a provider supports; claims
select a provider by `spec.provider.name` (exact) or `spec.provider.selector` (label match).
Multiple matches are resolved by highest `db.infrastructure.opendatahub.io/selection-priority`
annotation, ties broken alphabetically by name — `status.provider` on the claim names the winner
(task-02: a single field, not a list of every candidate — a claim binds to exactly one provider).
No `spec.provider` at all falls back to whichever provider is annotated
`db.infrastructure.opendatahub.io/is-default-provider: "true"`.

`status.conditions[Reachable]` is common to both types; everything else is type-specific (§6, §7).

### Status/condition contract for claims

`status.conditions[type=Provisioned]` is the primary machine-readable contract — consumers gate
on `Provisioned == True`, never on `status.phase`. `status.phase`
(`Pending`/`Provisioning`/`Ready`/`Failed`) is a human-readable `kubectl get` summary only. This
mirrors the framework's existing condition helpers used by every other module's CRDs; no new
condition-management code is needed.

## 6. Controller Design

One controller-manager binary (the `DatabaseService`-enabled Deployment) registers **three
independent** `reconciler.ReconcilerFor` pipelines, one per infrastructure CRD, reusing the
builder/`Owns`/`Watches` machinery every module already uses — but with **custom actions** in
place of `kustomize`/`deploy`, since claims and providers aren't manifest-deployments of an
upstream operand.

**Upgrade-aware from day one, like every other module's reconciler.** The standard pipeline every
other module uses places `upgradeIfNeeded` immediately after `stageManifests`, with nothing in
between, so a version-gated migration hook exists in a fixed position before any prior version
of the controller ever needs one (`.agents/skills/odh-module-dev`: "canonical order... nothing in
between"). All three infrastructure reconcilers follow the same principle, adapted to their
shape: each carries its own `upgradeIfNeeded`-equivalent action, positioned immediately after the
object is fetched — **before provider resolution and before any DDL or manifest-mutating
action** (migrating a claim/provider object's own on-disk shape, e.g. a renamed Secret key or a
changed default-privilege grant set, doesn't depend on which provider it's pointed at, so there's
no reason to resolve the provider first; this also mirrors the standard pipeline's own ordering,
where `upgradeIfNeeded` runs right after the cheap, local `stageManifests` step and before any
action that does real work like `kustomize`/`deploy`).
Each reconciled object (claim or provider) carries an
`infrastructure.opendatahub.io/controller-version` annotation stamped with the reconciling
controller's build version (the same `pkg/module.Version` ldflags-injected value other modules
already surface via `reportStatus`, per `.agents/skills/odh-module-dev/references/extending.md`
"Build Metadata"). On every reconcile, the `upgradeIfNeeded` action compares that annotation
against the running binary's version; on a mismatch, it runs whatever version-specific migration
that controller version requires (e.g., re-granting privileges if the default-privilege
statement set changed, migrating a claim Secret's key names, or migrating an `Embedded`
provider's StatefulSet selector/labels if the manifest template shape changed — the same kind of
selector migration `modelcontroller` already does on Deployment upgrades, per
`odh-module-migrate/references/migration-plan.md`) before falling through to the normal
steady-state actions; then stamps the annotation with the current version. For the very first
implementation there is no prior version to migrate from, so this hook starts as a no-op that
only stamps the annotation — but the hook and its pipeline position must exist and be exercised
by a test now (tasks 03/06/07/08), not bolted on retroactively the first time a real migration is
needed.

**Provider resolution** (shared helper, used by both claim reconcilers):
1. `spec.provider.name` set → exact lookup; not found → `Provisioned: False`, actionable message, requeue.
2. `spec.provider.selector` set → list `DatabaseProvider`s matching the selector; if >1, pick
   highest `selection-priority` annotation, tie-break alphabetically by name; write the winner's
   name to `status.provider`. **Task-02 deliberately diverges from spec.md's own CRD example
   here**: spec.md shows `matchedProviders: ["platform-shared"]` (a list of every candidate that
   matched), but a claim only ever binds to one provider, so a list of also-rans has no consumer
   — `status.provider` (singular) is simpler and loses nothing a real caller needs.
3. Neither set → use the provider annotated `is-default-provider: "true"`; none exists → `Pending`
   with actionable message.
4. Resolved provider not `Reachable` → propagate as the claim's condition message.

**`SchemaClaim` reconcile** (task-06): resolve provider → idempotent schema+user DDL (create
schema if absent, else provision an *additional* user against the existing schema — supports
multi-tenant reuse) → SSA-write credentials Secret (owner-referenced, name `== claim.Name`) →
`Provisioned: True` with `status.connection` populated. Deletion: `Retain` drops only the
provisioned user + Secret (schema/data persist); `Delete` drops the schema
(`DROP SCHEMA ... CASCADE`) and all its data, then the user + Secret. Both paths use a finalizer
so the claim's DDL cleanup runs before the object is removed.

**`DatabaseClaim` reconcile** (task-07): resolve provider → verify `spec.database` exists (else
`Pending` naming the missing database) → provision a dedicated user (broader privileges, can
`CREATE SCHEMA`) → SSA-write Secret → `Provisioned: True`. Always-`Retain` semantics via
finalizer: only the provisioned user (+ its Secret) is dropped on deletion, never the database.

**`DatabaseProvider` reconcile**: dispatch on `spec.type`.
- `External` (task-04): validate connectivity using `spec.external.connectionSecretRef`; set
  `Reachable`. No lifecycle management beyond that — the controller doesn't own this instance.
- `Embedded` (task-08): full sub-design in §7.

**Periodic retry, not just watch-triggered reconciliation.** All three infrastructure
reconcilers depend on state that can change without any Kubernetes object changing: an
`External` provider's database can go down and come back up, a provisioned `SchemaClaim`/
`DatabaseClaim` user or its Secret can be dropped/deleted directly against the database or
cluster outside this controller's own action, and an `Embedded` provider's idle-cleanup grace
period (§7.7) only ever elapses on a timer, never on an event. Watch-based reconciliation alone
would leave all of these stuck until something unrelated happens to touch the object. This module
configures periodic re-reconciliation using the framework's now-landed `WithDefaultRequeueAfter`
reconciler option (`odh-platform-utilities`/`operator-actions-framework`,
`framework/controller/reconciler`, commit `43916a99` on the `all-in` branch — go.mod's replace
directive needs bumping to pick it up, task-01): pass
`.WithReconcilerOpts(reconciler.WithDefaultRequeueAfter(cfg.DatabaseProviderRetryInterval))` (or
`cfg.ClaimRetryInterval` for the two claim kinds) once in each `reconciler.ReconcilerFor` builder
chain (task-03) — it fires automatically "when reconciliation succeeds and no action requested a
specific requeue," per the framework's own doc comment, so **no action anywhere returns
`ctrl.Result{RequeueAfter: ...}` manually for this purpose**; that would be redundant with, and
could inconsistently diverge from, the builder-level default. The same commit also adds
`ReconcilerBuilder.WatchesRawSource` (an arbitrary `source.Source`, e.g. a channel-backed
`source.Channel`, for non-watch-triggered events) — not needed by anything in this plan today,
but available if a future need arises (e.g. pushing an immediate reconcile on an out-of-band
admin action) without another framework change. Intervals are `pkg/config` keys, never
hardcoded, following the same pattern as every other config key in this module (§7.1, §7.7):
- `DatabaseProviderRetryInterval` (compiled default `2m`) — re-checks `External` connectivity and
  `Embedded` StatefulSet health/idle-cleanup on a cadence, so a provider that recovers or goes
  idle is noticed without a triggering event.
- `ClaimRetryInterval` (compiled default `5m`) — re-verifies a `Provisioned: True` claim's
  Secret/role still exist, catching drift (e.g. an accidentally deleted claim Secret) sooner than
  "never, until something else touches the claim," while staying coarse enough not to hammer the
  provider with idle polling.

**Boundary that must stay explicit throughout implementation**: the official `postgres` image's
env-var/init-script mechanism is used *only* to bring up a brand-new `Embedded` instance (initial
superuser + database, on first boot of an empty data directory). Every user/role creation after
that — `SchemaClaim`'s schema-scoped user, `DatabaseClaim`'s dedicated user, and any additional
user provisioned against either an `Embedded` or `External` backend — goes through the `pgx` DDL
layer (§8), never through init scripts or Jobs.

**`Embedded` connection host and `NetworkPolicy` — required for v1, scoped to `Embedded` only.**
Two related requirements, both because this controller *owns* the `Embedded` instance's network
identity and topology (neither applies to `External`, since that instance and its network
surroundings belong to the admin, not this operator):

- **Always the Service's cluster-DNS name, never a pod IP.** A claim's `status.connection.host`
  and Secret must resolve to the `Embedded` provider's headless `Service` (§7.3) by its stable
  in-cluster DNS name (`<service>.<namespace>.svc`), computed deterministically from the
  provider's name and the module operator's own namespace — never the StatefulSet pod's IP,
  which isn't stable across pod restarts/rescheduling. This falls out naturally from routing
  through the Service at all rather than the pod directly; call it out explicitly here so no
  implementation shortcut (e.g. reading `pod.status.podIP` because it's "simpler") defeats it.
- **`NetworkPolicy` scoping access to the `Embedded` instance's pod** — least-privilege network
  access, which the Module Operator Contract itself calls for ("implement NetworkPolicies for
  both control plane and workload controllers"). Concrete design: default-deny ingress on the
  `Embedded` pod except for the Postgres port, allowed only from namespaces currently holding a
  `Provisioned: True` `SchemaClaim`/`DatabaseClaim` against this provider — expressed as a
  `namespaceSelector` matching `kubernetes.io/metadata.name In {ns1, ns2, ...}` (the automatic
  namespace-name label every namespace already carries, so no extra namespace-labeling step is
  needed), computed from the **same claim-listing query idle-cleanup (§7.7) already runs**, and
  re-applied via SSA whenever that list changes. Templated alongside the other `Embedded`
  manifests (`assets/manifests/embedded/networkpolicy.yaml.tmpl`, task-08) — same
  `fwtemplate`+`deploy` mechanism, just with `WithDataFn`-computed namespace list instead of
  static spec fields.
- **Not created for `External`**: this operator has no view into, and no business restricting,
  network access to infrastructure it doesn't own — that's the admin's own network policy to
  write, if they want one, against their own instance.

## 7. `Embedded` Provider — Full Design

**Explicit deviation from spec.md's literal implementation guidance — deliberate, not an
oversight.** spec.md's own "Embedded provider implementation" section specifies two things this
plan does differently, both by explicit direction during planning rather than by drift:

1. spec.md says the controller should render "the StatefulSet, PVC, Service, and a bootstrap
   step ... directly as typed Go objects ... rather than depending on a manifest-rendering
   layer." This plan instead uses this repo's existing templated-manifest + SSA `deploy` pipeline
   (below) — reusing plumbing every other module already has, rather than building a new
   typed-object renderer this module would be the only one to own. Trade-off accepted: this
   module now depends on the framework's manifest-rendering layer for these resources, which is
   exactly what spec.md's rationale for typed objects was trying to avoid.
2. spec.md's provisioning sequence has a live, re-runnable bootstrap step ("connect as superuser,
   run `CREATE EXTENSION IF NOT EXISTS <name>` for each entry") — meaning extensions could in
   principle be applied at any time, not just at first boot. This plan instead uses the official
   image's `/docker-entrypoint-initdb.d/` first-boot-only mechanism (§7.3) to avoid a live `pgx`
   connection from `DatabaseProvider` reconciliation entirely, per explicit direction to keep
   `Embedded` reconciliation free of Jobs/`psql`/live DB connections. Trade-off accepted, and it
   is a real capability reduction versus spec.md's design, not a cosmetic one: this plan's
   `ExtensionChangeRequiresRecreate` failure mode (§7.3) — recreate the instance to change
   extensions — would not exist if the live-bootstrap approach spec.md describes were used
   instead. Both trade-offs are recorded here so a future reader can see they were weighed, not
   missed.

The `Embedded` provider deploys and owns a single-instance PostgreSQL via a StatefulSet + PVC +
headless Service, rendered as **templated manifests** (not typed Go objects) through the same
framework mechanism every other module already uses for its own resources —
`fwtemplate.NewAction` renders Go `text/template` YAML staged on `rr.Templates` into
`Unstructured` resources (template data includes `.Component`, i.e. the `DatabaseProvider`
instance itself, injected automatically — see
`odh-platform-utilities/framework/controller/actions/render/template/action_render_templates.go`),
and those resources flow through the same `deploy.NewAction` SSA path used everywhere else (see
`opendatahub-modelregistry-operator`'s `template → kustomize → deploy` pipeline). No bespoke
typed-object SSA renderer is needed.

### 7.1 Image selection

Computed once per reconcile via a `WithDataFn` callback (cache-keyed so `fwtemplate`'s resource
cacher skips re-rendering when the spec hasn't changed, per the "minimize dynamic computation"
principle in `odh-module-migrate/references/migration-plan.md`). The *mapping logic* (which
image class a given extension set requires) is fixed in Go — spec.md is explicit that this is
"No image override field, ever," since letting admins point a platform-managed instance at an
arbitrary image reopens supply-chain/support-surface problems. But the **image reference
strings themselves are never hardcoded Go literals** — they are `pkg/config` keys, following the
exact same compiled-default → mounted-ConfigMap → env-var precedence every other config key in
this module (and every other module's `RELATED_IMAGE_*`/`params.env` image parameterization,
e.g. `opendatahub-modelregistry-operator`'s `IMAGES_MODELREGISTRY_OPERATOR:
RELATED_IMAGE_ODH_MODEL_REGISTRY_OPERATOR_IMAGE`) already uses:

| `spec.embedded.extensions` contains | Image config key | Compiled default |
|---|---|---|
| `vector` | `DefaultPgvectorImage` | community `pgvector/pgvector:pg16` — no known Red Hat-shipped equivalent |
| only extensions bundled in the stock image (`pg_trgm`, `uuid-ossp`, `pgcrypto`, ...) | `DefaultPostgresImage` | community `postgres:16` |
| anything else unmapped | — | reconcile fails; `Reachable: False` condition tells the admin to use `External` |

**Considered and rejected: defaulting to `registry.redhat.io/rhel9/postgresql-16`.** A Red
Hat-shipped image would better match RHOAI's downstream support posture, but
`registry.redhat.io` requires an entitlement pull secret that a vanilla, unauthenticated `kind`
cluster doesn't have — defaulting to it would break spec.md's own constraint that this module be
"testable on a plain `kind` cluster." The compiled defaults stay the community images so the
module works out of the box on that baseline. This is exactly why both are `pkg/config` keys and
not hardcoded literals: on a platform where the entitlement pull secret *is* configured (e.g. a
connected OpenShift cluster), an admin repoints `DefaultPostgresImage` to
`registry.redhat.io/rhel9/postgresql-16` via the mounted ConfigMap — no code change, no
compiled-default change, and the `kind`-testable baseline is unaffected either way. Add both keys
via the standard 5-step procedure in `.agents/skills/odh-module-dev/references/config-keys.md`
(task-01/task-08).

### 7.2 Admin password — generate-once, never regenerate

Modeled directly on `opendatahub-io/model-registry-operator`'s `generateDeployment: true` path
(cloned to `.context/repos/opendatahub-io/model-registry-operator@main` for reference), which has
**no SQL driver dependency at all**: its `createOrUpdatePostgresSecret()` generates a password via
`utils.RandBytes(16)` only if the target Secret doesn't already have one
(`internal/controller/modelcatalog_controller.go`), then feeds it to the Postgres container via
`secretKeyRef` (`internal/controller/config/templates/catalog/catalog-postgres-deployment.yaml.tmpl`).

This module does the same, and it's also the answer to "should the admin provide this password":
**no** — spec.md's own `Embedded` CRD example has no `connectionSecretRef`-equivalent field at
all (only `External` has one, because it points at infrastructure the admin already owns).
`Embedded` exists precisely so a fresh install isn't blocked on manual PostgreSQL setup, so the
controller must generate it:

1. A custom action, first in the `Embedded` pipeline (before manifest staging), does a
   get-or-create on an admin Secret named `<providerName>-admin` in the module operator's own
   namespace (`DatabaseProvider` is cluster-scoped, so there's no claim namespace to use).
2. If the Secret is absent or lacks a password key: generate one via `crypto/rand` (the same
   generator used by the claim DDL layer, §8 — one implementation) and SSA-create/update the
   Secret with keys named to match Postgres's own env vars (`POSTGRES_USER` /
   `POSTGRES_PASSWORD` / `POSTGRES_DB`) so the StatefulSet template can `secretKeyRef` them
   directly, zero translation.
3. If it already has a password: leave it untouched. Regenerating after first boot would desync
   the Secret from the password already baked into the instance's on-disk data directory and lock
   the controller out of its own instance — this step must be strictly idempotent and must run
   before the StatefulSet is ever rendered.
4. Owner-referenced to the `DatabaseProvider`, garbage-collected on delete.
5. Downstream, this Secret is the `Embedded` provider's admin connection secret in the *same
   shape* as `External`'s `spec.external.connectionSecretRef` — claim reconcilers resolve "the
   admin secret for this provider" generically, no `Embedded`/`External` branch.
6. Same "no recovery if lost" posture as the rest of the spec: if this Secret is deleted
   independently, the running server is unaffected but the controller can no longer authenticate
   to manage schemas/roles on it — surfaced as `Reachable: False`, never silently recovered.

### 7.3 Manifests

`assets/manifests/embedded/{statefulset,pvc,service,initdb-configmap}.yaml.tmpl`:

- `appsv1.StatefulSet` — 1 replica, resources from `spec.embedded.resources`, env vars sourced
  from the admin Secret (§7.2) via `secretKeyRef`, volume mount from the PVC below, and a mounted
  `initdb-configmap` at `/docker-entrypoint-initdb.d/`.
- `corev1.PersistentVolumeClaim` — from `spec.embedded.storage.size` / `storageClassName`.
- headless `corev1.Service`.
- `corev1.ConfigMap` (`initdb-configmap`) — one `CREATE EXTENSION IF NOT EXISTS <name>;`
  statement per entry in `spec.embedded.extensions`. The official `postgres` image runs every
  script under `/docker-entrypoint-initdb.d/` automatically on first boot (empty data directory),
  so extensions are enabled with **no custom execution code, no `psql`, no `batchv1.Job`, no
  `pgx` connection from the controller** for this step.

A small custom action stages these onto `rr.Templates` (mirrors how `modelregistry`'s
`customizeManifests` stages `rr.Templates`/`rr.Manifests`); `fwtemplate.NewAction` renders them;
`deploy.NewAction` applies them via SSA — identical mechanics to every other module's
manifest-based resources.

**Documented limitation**: `docker-entrypoint-initdb.d` scripts run once, on first boot only. If
`spec.embedded.extensions` is edited after the instance already exists, the controller detects
the mismatch (desired extensions vs. capability labels already derived, §7.5) and sets
`Reachable: False, reason: ExtensionChangeRequiresRecreate` rather than attempting a live
`CREATE EXTENSION` — consistent with spec.md's explicit framing of `Embedded` as intentionally
limited (no custom tuning, no day-2 reconfiguration surface).

### 7.4 Readiness

A small custom action analogous to the framework's `deployments.NewAction`, but checking the
StatefulSet's ready-replica count instead of a Deployment's.

### 7.5 Capability labels

Once ready, a custom action (same pattern as `reportStatus` in other modules) derives
`db.infrastructure.opendatahub.io/capability-*` labels from `spec.embedded.extensions` (e.g.
`vector` present → `capability-pgvector: "true"`) and SSA-patches them onto the `DatabaseProvider`
object itself — auto-derived, never admin-set independently, so labels can't drift from reality.

### 7.6 `Reachable` condition state machine

`Provisioning` (admin secret + manifests applied, StatefulSet not yet ready) → `True`
(`InstanceRunning`, once ready and capability labels derived) → failure reasons
(`ImageUnmapped`, `ExtensionChangeRequiresRecreate`, `AdminSecretUnavailable`, `Idle` — §7.7).

### 7.7 Idle cleanup for `deletionPolicy: Delete`

Resolves spec.md's open question about exact idle-cleanup scope. Decision: on every reconcile,
list `SchemaClaim`/`DatabaseClaim` objects referencing this provider (by name or matched via
selector). If zero for longer than a grace period — `pkg/config` key
`EmbeddedIdleGracePeriod`, compiled default `10m`, overridable via ConfigMap/env like every other
config key (not a hardcoded Go constant; avoids flapping on transient states) — delete **only** the underlying
StatefulSet/PVC/Service (reusing the same `gc`-style deletion every module already does for
orphaned owned resources) — **not** the `DatabaseProvider` object itself. Set
`Reachable: False, reason: Idle`, and re-provision lazily on the next claim. This keeps capability
labels and provider identity stable across idle cycles instead of destroying and re-registering
the object. `deletionPolicy: Retain` (default): the instance persists even with zero active
claims.

## 8. PostgreSQL DDL Execution Layer (`pkg/postgres`)

Used by the `SchemaClaim`/`DatabaseClaim` reconcilers for DDL, where target schema/role names are
computed per claim at reconcile time and have no static, template-renderable form. Also used —
narrowly — by `DatabaseProvider`'s `External` type (task-04) for its short-lived connectivity
check, since that's a real `pgxpool` dial too and sharing the connection-pool/dial code avoids a
second implementation of the same thing. **The one invariant that must hold without exception**:
the `Embedded` provider's own reconciliation (§7) never imports this package for anything beyond
the `crypto/rand` password generator — no `pgxpool`, no DDL statement builder, no live SQL
connection anywhere in `Embedded` provider reconciliation, full stop. That boundary (claims +
`External`'s liveness check may use the connection/DDL code; `Embedded` may only use the password
generator) is what task-08's acceptance criteria verify by code review.

- **Driver**: [`jackc/pgx`](https://github.com/jackc/pgx) v5, `pgxpool` for connection pooling —
  the de facto standard Go PostgreSQL driver, also used internally by CloudNativePG's own
  controller.
- **Identifier quoting**: `pgx.Identifier{name}.Sanitize()` for every schema/role/user name
  interpolated into DDL (identifiers can't be bind-parameterized) — the same pattern CNPG's
  `Database`/`DatabaseRole` reconcilers use
  (`internal/management/controller/database_controller_sql.go` in cloudnative-pg/cloudnative-pg).
- **Literal quoting**: `pq.QuoteLiteral()` for any literal embedded directly in DDL (e.g. a
  password in `ALTER ROLE ... WITH PASSWORD '<literal>'`).
- **Password generation**: `crypto/rand`-backed random string generation, the same approach as
  Zalando postgres-operator's `pkg/util.RandomPassword` — shared with the `Embedded` admin-secret
  generator (§7.2), one implementation.
- **No password ever leaves controller memory except into the Secret**: generated password is
  used immediately in `ALTER ROLE`/`CREATE ROLE` and then written into the Secret via SSA — never
  logged, never cached beyond the single reconcile.
- Connection pool lifecycle: one `pgxpool.Pool` per resolved `DatabaseProvider`, cached by
  provider name + admin-secret resource version (invalidated when the admin secret changes).

## 9. Secret Management

SSA-written, owner-referenced Secret named `== claim.Name`, no `namespace` field (Secret always
lives in the claim's own namespace), garbage-collected via Kubernetes owner references — reusing
the same `controllerutil.SetControllerReference` + SSA pattern already implicit in the
framework's `deploy` action, applied here directly since claims aren't manifest-based resources.

## 10. RBAC & Packaging

Kubebuilder markers per CRD/verb, `config/rbac/role.yaml` generated via `make manifests`, Helm
chart via the reused `cmd/chartgen`, `assets/manifests/module.yaml` +
`component_metadata.yaml` descriptors — identical mechanics to every existing module (see
`.agents/skills/odh-module-dev/references/rbac-rules.md` for the Owns/RBAC lockstep rule). One
caveat worth flagging during implementation: `component_metadata.yaml`'s `releases` field is
designed to report an *upstream operand's* version; this module has no upstream operand distinct
from itself, so that field is a slightly awkward fit and should report this module's own
version/repo rather than being left to imply an external project.

## 11. Testing Strategy

**Definition of done, per task — not deferred to a later task.** A task in `docs/task-NN.md` is
only complete when it ships, itself, both of the following (where applicable to what that task
adds):

- **Unit tests** for any new pure logic (provider resolution, DDL statement builders, quoting,
  image-mapping selection, password generation, admin-secret idempotency, config precedence).
- **Integration tests** for anything that touches the Kubernetes API and/or a real PostgreSQL
  instance, run **against the connected cluster** (this module's development/verification target
  is a real `kind` cluster reachable via the current `kubectl` context throughout — assume it is
  already up; do not write tests that only run against a hypothetical future cluster, and do not
  gate a task's completion on "a later task will add the integration test").

Concretely: task-01 needs a manager-startup smoke test against the connected cluster; task-02
needs the CEL validation rule exercised as a real admission test with the CRD installed on the
connected cluster (CEL validation is enforced by `kube-apiserver`, not a fake client, so this
cannot be a pure unit test); task-03's `providerresolve` unit tests are supplemented by an
integration test creating real `DatabaseProvider`/claim fixtures on the connected cluster;
tasks 04–08 each carry the integration tests already spelled out in their own acceptance
criteria; task-09's Helm/RBAC output is verified by actually installing it on the connected
cluster. Task-10 exists only for **cross-cutting scenarios that don't belong to any single
task** (multiple CRDs interacting together, the full negative-path matrix) — it is not a
catch-all for coverage any earlier task skipped.

- **Unit** (Gomega dot-import, table-driven): provider resolution (name/selector/priority
  tie-break), DDL statement builders (identifier/literal quoting correctness), image-mapping
  logic, admin-secret get-or-create idempotency, config precedence (compiled default → ConfigMap
  → env var).
- **Integration** (against the connected cluster): a real `Embedded` provider provisioned
  end-to-end (StatefulSet ready, extensions present, a `SchemaClaim` and `DatabaseClaim` both
  successfully provisioned against it); an `External` provider tested against a
  testcontainers-launched Postgres.
- **e2e**: per `odh-module-test` conventions, plus negative-path coverage (missing provider,
  unreachable provider, unmapped extension combination, deleted admin secret) — not just the
  happy path, per this repo's convention of covering dependency-gated preconditions explicitly.
- Cleanup scripts (`cleanup-integration.sh`/`cleanup-e2e.sh`) must delete claims and wait for
  finalizer-driven DDL cleanup before deleting the module CRDs, mirroring every other module's
  cleanup ordering.

## 12. Divergences from Standard Module Pattern

Narrowed to a single item: **the `pgx`-based DDL/connection layer** (§8) — used by the
`SchemaClaim`/`DatabaseClaim` reconcilers for per-claim schema/role/grant statements, since those
identifiers are computed dynamically per claim throughout the provider's lifetime and have no
manifest-rendering equivalent, and used narrowly by `DatabaseProvider`'s `External` type for its
liveness-check dial (§4, task-04) — reusing the same connection code rather than a second
implementation. **`Embedded` provider reconciliation is the one place this layer must never be
used** (§8's invariant) — it stays 100% manifest/template-driven, matching every other module's
pattern exactly. Everything else in this module, including the `Embedded` provider end-to-end,
follows existing precedent exactly:

- StatefulSet/PVC/Service/`initdb`-ConfigMap/`NetworkPolicy`: standard `fwtemplate` + `deploy` (SSA) actions,
  same as every other module's manifest-based resources.
- Initial superuser/database bootstrap and extension enablement: the official `postgres` image's
  own first-boot behavior (env vars + `/docker-entrypoint-initdb.d/`) — the exact technique
  `model-registry-operator`'s `generateDeployment` path already uses in production, not a new
  mechanism invented for this module.
- No upstream `get-manifests.sh` fetch — a difference in *content* (nothing external to vendor),
  not in *mechanism* (packaging is unchanged).

## 13. Open Questions Carried From spec.md

Scoped down to what's actually relevant at this stage: connection pooling and cross-component
schema migration on upgrade remain open (future concerns, not blocking this implementation);
credential rotation is explicitly out of scope (no scheduled/on-demand rotation, no automatic
recovery from a deleted Secret — deleting and recreating the claim is the only path). CRD/API
group (`infrastructure.opendatahub.io/v1alpha1`) and claim CRD names (`SchemaClaim`,
`DatabaseClaim`) are already confirmed by spec.md, not open. Multi-tenancy isolation and the DSPO
MariaDB→PostgreSQL migration are explicitly **not** design questions for this module — see §2.

## 14. References

- [spec.md](file:///home/luca/work/rh/raw/arch/rhai-database-service/spec.md) — source spec
- [ODH Module Operator Contract](file:///home/luca/work/rh/raw/arch/rhai-lifecycle/ODH%20Module%20Operator%20Contract.md) — `PlatformObject` status contract used by §4
- [ODH module config projection](file:///home/luca/work/rh/raw/references/wiki/rhoai/odh-module-config-projection.md)
- [RHOAI database landscape](file:///home/luca/work/rh/raw/references/wiki/rhoai/rhoai-database-landscape.md)
- `.agents/skills/odh-module-dev/`, `.agents/skills/odh-module-migrate/references/migration-plan.md` — this repo's module scaffold conventions
- `.context/repos/opendatahub-io/model-registry-operator@main` — precedent for the `Embedded`
  provider's env-var/init-script-driven auto-provisioning (§7.2, §7.3) and admin-password
  generate-once pattern
- [CloudNativePG](https://github.com/cloudnative-pg/cloudnative-pg) — DDL-execution pattern precedent (§8)
- [Zalando postgres-operator](https://github.com/zalando/postgres-operator) — password generation precedent (§8)