---
name: odh-module-dev
description: >
  Development guide for the ODH Module Operator. Covers the action pipeline,
  config loading, manager wrapper, cache settings, and how to extend the
  operator. Use when modifying the reconciler, adding actions, changing config,
  or adding new module kinds.
user-invocable: true
allowed-tools:
  - Read
  - Glob
  - Grep
  - Bash
  - Write
  - Edit
---

# ODH Module Operator -- Development Guide

Creating a new module from a monolith component: use
[odh-module-migrate](../odh-module-migrate/SKILL.md).

## Action Pipeline

Split module operators build a sequential pipeline via `reconciler.ReconcilerFor`.
Each action is `func(context.Context, *ReconciliationRequest) error`.

**Canonical order** (`upgradeIfNeeded` must come immediately after `initialize`,
with nothing in between):

```
[component actions e.g. sanitycheck]
-> initialize -> upgradeIfNeeded -> releases -> kustomize -> deploy
-> deployments -> reportStatus -> gc
```

| Action | What it does |
|---|---|
| `initialize` | Sets manifest paths on `rr.Manifests` based on platform overlay |
| `upgradeIfNeeded` | Module-only: version/platform migration hook |
| `releases` | Reads `component_metadata.yaml` and populates release info in CR status |
| `kustomize` | Renders manifests to `rr.Resources` |
| `deploy` | Applies `rr.Resources` via SSA |
| `deployments` | Checks Deployment readiness |
| `reportStatus` | Writes module version, platform, manifest sources to CR status |
| `gc` | Deletes orphaned resources with `part-of` label |

The `ReconciliationRequest` is a shared state bag:

- `rr.Manifests` -- populated by `initialize`, consumed by `kustomize`
- `rr.Resources` -- populated by `kustomize`, consumed by `deploy` and `gc`
- `rr.Instance` -- the Module CR (cast to `*MyModule` in actions)
- `rr.Client` -- k8s client
- `rr.ManifestsBasePath` -- from the manager wrapper (see below)

Full pipeline rules, builder setup, and file organization:
[references/action-pipeline.md](references/action-pipeline.md).

## Manager Wrapper

The `ctrl.Manager` is wrapped by `odhmanager.New()`:

```go
mgr := odhmanager.New(ctrlMgr, odhmanager.WithManifestsBasePath(cfg.ManifestsPath))
```

This wrapper provides `rr.ManifestsBasePath` to the action pipeline. Without
it, `initialize` produces paths like `mymodule/overlays/odh` (no base) and
kustomize fails with `lstat /mymodule: no such file or directory`.

Integration tests must use the same wrapper pattern.

## Config Loading

Three-layer precedence (later wins):

1. **Compiled defaults** -- `setDefaults()` in `pkg/config/config.go`
2. **Mounted ConfigMap** -- files at `ODH_MODULE_OPERATOR_CONFIGURATION_PATH`
3. **Environment variables** -- `ODH_MODULE_OPERATOR_` prefix

**Env prefix rule:** canonical prefix is `ODH_MODULE_OPERATOR_`. Never use
`ODH_OPERATOR_` or component-specific prefixes. The prefix in manifests must
match `EnvPrefix` in `pkg/config/config.go`, or viper silently ignores env
vars during `Unmarshal()`.

**Viper workaround:** `AutomaticEnv()` only works with `Get()`, not
`Unmarshal()`. The `bindEnv()` function explicitly calls `v.BindEnv()` for
every key in `v.AllKeys()`. A key must be known to viper before `bindEnv()`
runs, otherwise the env var is silently ignored.

**Platform namespace:** the operator sets `rhai-applications-namespace` via
both `viper.Set()` and `cluster.SetRHAIApplicationNamespace()` -- viper for
kustomize render, the cluster function for `cluster.GetApplicationNamespace()`.

Adding a new config key: [references/config-keys.md](references/config-keys.md).

## Cache and Client

`cmd/operator/operator.go` configures:

- **`DefaultNamespaces`**: watches scoped to the applications namespace and
  cluster-scoped resources. New namespaces require updating this map.
- **`DefaultTransform`**: `StripUnusedFields()` removes `managedFields` and
  `last-applied-configuration` from cached objects.
- **`DisableFor`**: ConfigMaps and Secrets bypass the cache (always fresh).
- **`ReaderFailOnMissingInformer: true`**: `Get`/`List` for a resource type
  with no running informer returns `ErrResourceNotCached` instead of falling
  through to a live API call. This catches missing `Owns()` or `Watches()`.

## Extending

What you can do and where to find the details:

- **Add a workload resource type** -- update manifest, `Owns()`, and RBAC
  marker in lockstep: [references/rbac-rules.md](references/rbac-rules.md)
- **Add a config key** -- 5-step procedure:
  [references/config-keys.md](references/config-keys.md)
- **Add a custom action** -- `WithAction(myAction)` pattern:
  [references/extending.md](references/extending.md)
- **Add a new module kind** -- kubebuilder scaffold procedure:
  [references/extending.md](references/extending.md)
- **GVK package rule** -- module-local GVK imports:
  [references/gvk-rule.md](references/gvk-rule.md)
- **Build metadata** -- ldflags and reportStatus:
  [references/extending.md](references/extending.md)
