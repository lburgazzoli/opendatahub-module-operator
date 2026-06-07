# Orchestrator Architecture

## Overview

The orchestrator manages module lifecycle via two CRs and two controllers:

- **Platform** (singleton `default-platform`): declares enabled modules in `spec.modules`
- **PlatformOperator** (per-module): tracks deployed resources, chart info, distribution version

The Platform CR status is the **single source of truth** for orchestration state.
There is no in-memory state machine — all decisions are derived from CR status fields.

## Controllers

### Platform Controller

Reconciles the singleton Platform CR. Action pipeline:

```
initialize → checkAdminAcks → ensureModules → deploy → advanceRunlevel → aggregateStatus → gc
```

| Action | Purpose |
|--------|---------|
| `initialize` | Sets `status.runlevel` to first runlevel if unset |
| `ensureModules` | Builds PlatformOperator resources from `spec.modules` into `rr.Resources` |
| `deploy` | Creates/updates PlatformOperator CRs via SSA |
| `advanceRunlevel` | Advances `status.runlevel` when all enabled modules at current level match distribution version |
| `aggregateStatus` | Populates `status.modules` (sorted by runlevel+name), writes `status.distribution` when all modules match |
| `gc` | Deletes PlatformOperator CRs for modules removed from `spec.modules` |

Watches:
- `For(Platform)` — the primary reconciled type
- `Owns(PlatformOperator)` with `ResourceVersionChangedPredicate` — triggers on any PO change including status updates

### PlatformOperator Controller (Module Reconciler)

Single controller handling ALL PlatformOperator CRs. Action pipeline:

```
resolveModule → checkRunlevel → ensureNamespace → renderChart → deploy → deployments → pruneOrphans → reportStatus
```

| Action | Purpose |
|--------|---------|
| `resolveModule` | Looks up module by PO name, lazily creates/caches Helm engine |
| `checkRunlevel` | Reads Platform CR status, gates on runlevel if upgrade in progress |
| `ensureNamespace` | Prepends module namespace (managed=false, no ownerRef) |
| `renderChart` | Renders Helm chart with merged values |
| `deploy` | Applies resources via SSA with dynamic ownership |
| `deployments` | Checks Deployment readiness in module namespace |
| `pruneOrphans` | Deletes resources removed from chart (skips CRDs, Namespaces) |
| `reportStatus` | Writes resources list, runlevel, chart info, distribution version |

Watches:
- `For(PlatformOperator)` — the primary reconciled type
- `Watches(Platform)` — with predicate on `status.runlevel` and `status.distribution.version` changes; map function enqueues only eligible modules
- `WatchesGVK(module.GVK)` per module — dynamic watch (registered when CRD exists) that triggers re-reconciliation when a module CR changes

## Runlevel Progression

Runlevel gating only applies during upgrades (when `status.distribution.version` differs from `cfg.Distribution.Version`):

1. Platform detects version mismatch → stays at current runlevel
2. Only modules with `runlevel <= status.runlevel` pass `checkRunlevel`
3. Higher-runlevel modules get `PauseError` (requeue after 5s)
4. When all enabled modules at current runlevel report matching distribution version → `advanceRunlevel` bumps to next runlevel with enabled modules
5. When all modules match → `aggregateStatus` writes the new distribution version → no more mismatch → all modules reconcile freely

When there's no version mismatch (steady state), all modules reconcile regardless of runlevel.

## Module Registry

`pkg/module/ModuleRegistry` — pure data structure, no state:
- Holds registered modules with GVK, namespace, chart path, runlevel
- Computes runlevel groups (`ComputeRunlevels`)
- Provides lookups: `ModuleByName`, `ModuleByGVK`, `ModulesAtRunlevel`
- Implements `module.Registry` interface for module registration

## Resource Metadata

The `moduleMetadata` transformer stamps deployed resources:
- `platform.opendatahub.io/part-of: <module-name>` label (skipped on CRDs)
- `config.opendatahub.io/<module-name>: "true"` annotation

Namespaces are created with `opendatahub.io/managed: "false"` so the deploy action only creates them (never updates or sets ownerRef).

## Ownership Chain

```
Platform CR
  └── owns → PlatformOperator CRs (via deploy action + ownerRef)
                └── owns → Module resources (ServiceAccount, ConfigMap, CRD, etc.)
```

- Platform deletion cascades to PlatformOperators → cascades to module resources
- CRDs and Namespaces are excluded from cascade cleanup (`pruneOrphans` skips them)

## Configuration

Config uses `distribution.name` / `distribution.version` consistently:
- `pkg/config/Config.Distribution.Name` / `Config.Distribution.Version`
- Platform CR `status.distribution.name` / `status.distribution.version`
- PlatformOperator CR `status.distribution.name` / `status.distribution.version`
- Helm values: `distribution.name` / `distribution.version`

Config keys: `distribution.name`, `distribution.version` (viper mapstructure)

## Key Design Decisions

1. **No in-memory state** — Platform CR status is the single source of truth
2. **No mode** (upgrade/reconcile) — runlevel + version comparison is sufficient
3. **Single PlatformOperator controller** — no per-module dynamic spawning
4. **Predicate-based triggering** — Platform watches PO with `ResourceVersionChangedPredicate`; PO watches Platform with runlevel/version predicate
5. **Distribution version gating** — `aggregateStatus` only writes the new version when ALL modules match, preventing premature "upgrade complete" signals
6. **Dynamic module CR watches** — registered lazily via `CrdExists` predicate, since CRDs are deployed by Helm charts at runtime
