# Task 01 — Module Scaffold

See `docs/plan.md` §3 for the full rationale.

## Goal

Stand up the standard per-module directory layout and build tooling for
`opendatahub-db-operator`, with no reconcile logic yet, so later tasks have a place to land code.

## Depends on

Nothing (first task). `go.mod`/`go.sum` already exist in this module.

## Key files/packages

- `Makefile`, `Containerfile` — copy structure from `modules/opendatahub-ray-operator`, rename.
- `cmd/main.go`, `cmd/operator/operator.go`, `cmd/chartgen/` — copied pattern, retargeted to this
  module's GVKs (per `pkg/resources/gvk/gvk.go` below).
- `pkg/config/config.go` — three-layer config (compiled defaults → mounted ConfigMap →
  `ODH_MODULE_OPERATOR_` env vars), same as every module (`.agents/skills/odh-module-dev`).
  Includes, from the start, the config keys later tasks need
  (`docs/plan.md` §6/§7.1/§7.7): `DefaultPostgresImage` (compiled default should be a Red
  Hat-shipped PostgreSQL image, e.g. `registry.redhat.io/rhel9/postgresql-16` — confirm exact
  name/tag against the current Red Hat container catalog before hardcoding it as the compiled
  default), `DefaultPgvectorImage` (compiled default `pgvector/pgvector:pg16` — community image,
  no known Red Hat-shipped equivalent; verify during task-08),
  `EmbeddedIdleGracePeriod` (compiled default `10m`), `DatabaseProviderRetryInterval` (compiled
  default `2m`), and `ClaimRetryInterval` (compiled default `5m`) — added via the 5-step
  procedure in `.agents/skills/odh-module-dev/references/config-keys.md` so tasks 03/04/06/07/08
  have them ready. These are config keys, never hardcoded Go literals referenced directly from
  controller code.
- `pkg/manager/manager.go` — `modulemanager.New(ctx, cfg, moduleCfg)` wrapper wiring
  `rr.ManifestsBasePath`, scheme registration, leader election, health/ready checks.
- `pkg/resources/gvk/gvk.go` — module-local GVK registry; module code must import this, never
  `github.com/opendatahub-io/opendatahub-operator/v2/pkg/cluster/gvk` directly.
- `config/{crd/bases,rbac,manager,chart,default}` — empty/skeleton, populated by `make manifests`
  once CRD types exist (task-02).
- `assets/manifests/module.yaml`, `assets/manifests/component_metadata.yaml` — descriptor
  skeleton (see `docs/plan.md` §10 caveat about the `releases` field).
- `assets/manifests/embedded/` — placeholder directory for the `*.yaml.tmpl` files task-08 fills
  in.
- `go.mod` — add `github.com/jackc/pgx/v5` (needed by task-05); bump the
  `odh-platform-utilities`/`operator-actions-framework` `replace` directives to the `all-in`
  branch commit that adds `reconciler.WithDefaultRequeueAfter`/`ReconcilerBuilder.WatchesRawSource`
  (`docs/plan.md` §6; commit `43916a99...` at the time this was written — resolve the current tip
  of that branch rather than trusting this hash to still be current).
- `test/{integration,e2e,support}` — empty skeleton directories with `TestMain` wiring
  (`SetDefaultEventuallyTimeout`, etc.), matching `.agents/skills/odh-module-test`.

## Steps

1. Copy the directory skeleton from `modules/opendatahub-ray-operator`, strip all Ray-specific
   controller/action/CRD code (that comes in later tasks), keep the scaffold plumbing
   (`cmd/`, `pkg/config`, `pkg/manager`, `pkg/resources/gvk`, Makefile, Containerfile).
2. Rename all `ray`/`Ray` identifiers, env prefixes, and image references to
   `databaseservice`/`DatabaseService`/`db-operator` per this repo's naming convention (module directory
   is already `opendatahub-db-operator`, matching `feedback_module_naming`: skip the redundant
   `-operator` suffix only when the name already ends with it — `db` doesn't, so
   `opendatahub-db-operator` is correct as-is).
3. Add `github.com/jackc/pgx/v5` to `go.mod`; bump the framework `replace` directives to the
   `all-in` branch tip; run `go mod tidy`.
4. Verify `pkg/resources/gvk/gvk.go` exists and is the only place that imports the platform's
   cluster-scoped GVK package.
5. Verify the empty scaffold builds: `go build ./...` succeeds even with no CRD types yet (an
   empty `api/` package is fine at this stage — task-02 fills it in).
6. Add unit tests for `pkg/config`'s three-layer precedence, including the five new keys from
   step 2 above: compiled default wins with nothing else set; a mounted ConfigMap value
   overrides the compiled default; an `ODH_MODULE_OPERATOR_` env var overrides both.
7. Add an integration smoke test, run against the connected cluster (assume `kubectl`'s current
   context already points at a working cluster — do not skip or stub this waiting for "a later
   task"): start the manager via `modulemanager.New` against the real cluster and confirm it
   reaches a healthy state (`/healthz`/`/readyz` respond, leader election succeeds) even before
   any CRDs exist. This is the first proof the scaffold actually runs, not just builds.

## Acceptance criteria

- `go build ./...` succeeds.
- `pkg/resources/gvk/gvk.go` exists and no other file imports the platform GVK package directly.
- No `ray`/`Ray`/`kuberay` identifiers remain anywhere in the module.
- `Makefile` exposes the standard target set (`test`, `manifests`, `generate`, `fmt`, `vet`,
  `deps`, `lint`, `test-integration-setup/run`, `test-e2e-setup/run`, `helm`,
  `container-build`, `run`) even if some are no-ops until later tasks add content.
- **Unit test** for `pkg/config` precedence (step 6) exists and passes — this task is not
  complete without it.
- **Integration test** for manager startup against the connected cluster (step 7) exists and
  passes — this task is not complete without it.