---
name: odh-module-scaffold
description: >
  Development guide for the ODH Module Operator. Covers the action pipeline,
  config loading, chart generation, manager wrapper, testing patterns, and
  how to extend the operator. Use when modifying the reconciler, adding
  actions, changing config, debugging failures, or adding new module kinds.
user-invocable: true
allowed-tools:
  - Read
  - Glob
  - Grep
  - Bash
  - Write
  - Edit
---

# ODH Module Operator — Development Guide

## Action Pipeline

The controller builds a sequential pipeline via `reconciler.ReconcilerFor`.
Each action is `func(context.Context, *ReconciliationRequest) error`.

```
initialize -> releases -> reportStatus -> kustomize -> deploy -> deployments -> gc
```

| Action | What it does |
|---|---|
| `initialize` | Sets manifest paths on `rr.Manifests` based on platform overlay (`overlays/odh` or `overlays/rhoai`) |
| `releases` | Reads `component_metadata.yaml` and populates release info in CR status |
| `reportStatus` | Writes module version, platform, config values, and manifest sources to CR status |
| `kustomize` | Renders manifests to `rr.Resources`. Calls `cluster.ApplicationNamespace()` for the target namespace |
| `deploy` | Applies `rr.Resources` via SSA. Sets owner references, platform labels, instance annotations |
| `deployments` | Checks Deployment readiness. Sets `DeploymentsAvailable` condition |
| `gc` | Deletes resources with the `platform.opendatahub.io/part-of` label that are no longer in `rr.Resources` |

The `ReconciliationRequest` is a shared state bag:

- `rr.Manifests` — populated by `initialize`, consumed by `kustomize`
- `rr.Resources` — populated by `kustomize`, consumed by `deploy` and `gc`
- `rr.Instance` — the Module CR (cast to `*MyModule` in actions)
- `rr.Client` — k8s client
- `rr.ManifestsBasePath` — from the manager wrapper (see below)

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

1. **Compiled defaults** — `setDefaults()` in `pkg/config/config.go`
2. **Mounted ConfigMap** — files at `ODH_MODULE_OPERATOR_CONFIGURATION_PATH`
3. **Environment variables** — `ODH_MODULE_OPERATOR_` prefix

Config is loaded once at startup via `moduleconfig.Load()` and passed to
`NewReconciler(ctx, mgr, cfg)`.

**Viper workaround**: `AutomaticEnv()` only works with `Get()`, not
`Unmarshal()`. The `bindEnv()` function explicitly calls `v.BindEnv()` for
every key in `v.AllKeys()`. A key must be known to viper (via `SetDefault`,
config file load, or explicit `BindEnv`) before `bindEnv()` runs, otherwise
the env var will be silently ignored during `Unmarshal()`.

**Platform namespace**: the operator's `cluster.ApplicationNamespace()` reads
viper key `rhai-applications-namespace`. The module operator sets this from
its own config:

```go
viper.Set("rhai-applications-namespace", cfg.ApplicationsNamespace)
cluster.SetRHAIApplicationNamespace(cfg.ApplicationsNamespace)
```

Both calls are needed — viper for the kustomize render action, the cluster
function for any code that calls `cluster.GetApplicationNamespace()`.

## Cache and Client

`cmd/operator/operator.go` configures:

- **`DefaultNamespaces`**: watches are scoped to the applications namespace
  and cluster-scoped resources. Adding resources in a different namespace
  requires updating this map.
- **`DefaultTransform`**: `StripUnusedFields()` removes `managedFields` and
  `last-applied-configuration` from cached objects.
- **`DisableFor`**: ConfigMaps and Secrets bypass the cache (always read fresh).
- **`Unstructured: true`**: cache-backed reads for unstructured objects.
- **`ReaderFailOnMissingInformer: true`**: a `Get` or `List` for a resource
  type with no running informer returns `ErrResourceNotCached` instead of
  silently falling through to a live API call. This catches missing `Owns()`
  or `Watches()` declarations at runtime.

## Helm Chart Generation

`cmd/chartgen/` reads multi-doc YAML from stdin and generates a Helm chart.
Resources are grouped by GVK into files named `<group>_<version>_<kind>.yaml`.

Transformations:

- **Deployment**: image, resources, replicas, serviceAccountName, imagePullSecrets from values
- **ServiceAccount**: name from values, annotations from values
- **ConfigMap**: merges `.Values.config` and `.Values.imagePullSecret` into data
- **RoleBinding/ClusterRoleBinding**: subjects namespace and SA name from values
- **Namespaced resources**: `metadata.namespace` → `{{ .Release.Namespace }}`
- **Everything else**: passed through as-is

`values.schema.json` is generated via `invopop/jsonschema` reflection on the
`Values` struct in `cmd/chartgen/values.go`. Adding a field to `Values`
automatically updates the schema.

### Maintaining the Chart Generator

The `chartgen` subcommand in `cmd/chartgen/` must be updated when:

- **New Helm values are needed** — add a field to `Values` struct in
  `values.go`. The schema updates automatically. Add a `jsonschema` struct
  tag for description/enum/default.
- **New resource types need special templating** — add a case to
  `transformResource()` in `chart.go`. Tier-1 resources get value injection,
  tier-2 resources get namespace templating only.
- **Namespace templating logic changes** — the `replaceNamespace()`,
  `replaceSubjectsNamespace()`, and `replaceSubjectsServiceAccount()`
  functions in `chart.go` handle string-level YAML manipulation. Note:
  YAML list items starting with `- ` require the list-item check in the
  subjects section scanner (see the `!strings.HasPrefix(trimmed, "-")`
  guard).
- **`_helpers.tpl` or `Chart.yaml` template changes** — edit the constants
  in `helpers.go`. `Chart.yaml` is only generated if missing (existing
  files are preserved).

After changes: `make helm` regenerates the chart and verifies it lints.

## Testing

Tests use Ginkgo BDD (`Describe`/`It`) with `gomega-matchers`
(`github.com/lburgazzoli/gomega-matchers`):

- `k8sm.New(cli, scheme)` creates a matcher wrapping the k8s client
- `k.Get(obj)` returns a function for `Eventually()` that fetches and returns `*unstructured.Unstructured`
- `jq.Match(expr)` evaluates a JQ expression against the result

```go
import (
    k8sm "github.com/lburgazzoli/gomega-matchers/pkg/matchers/k8s"
    "github.com/lburgazzoli/gomega-matchers/pkg/matchers/jq"
)

k := k8sm.New(k8sClient, scheme)
Eventually(k.Get(module)).WithContext(ctx).Should(jq.Match(`.status.phase == "Ready"`))
```

**Integration tests** derive expected values from `config/manager/configmap.yaml`
via `support.MustReadConfigMapData()`. Changes to the ConfigMap must be
reflected in test expectations (they are coupled).

## Extending

### Adding a Workload Resource Type

When a manifest in `config/manifests/` introduces a new resource type (e.g.,
adding a `NetworkPolicy` YAML), three things must be updated in lockstep:

1. **Manifest** — the kustomize resource in `config/manifests/mymodule/base/`
2. **`Owns()`** — register on the ReconcilerFor builder so changes to the
   resource trigger reconciliation:
   ```go
   Owns(&networkingv1.NetworkPolicy{})
   ```
3. **RBAC marker** — grant permissions so the operator can manage it:
   ```go
   // +kubebuilder:rbac:groups=networking.k8s.io,resources=networkpolicies,verbs=get;list;watch;create;update;patch;delete
   ```

Missing any one of the three causes:
- No `Owns()` → changes to the resource don't trigger reconciliation
- No RBAC marker → `Forbidden` errors when deploying or garbage collecting
- No manifest → the resource is never created (but `Owns` watch is harmless)

After changes: `make manifests generate` to regenerate `config/rbac/role.yaml`.

### Adding a Config Key

1. Add constant: `KeyMyField = "my-field"` in `pkg/config/config.go`
2. Add field: `MyField string \`mapstructure:"my-field"\`` in `Config` struct
3. Add default: `v.SetDefault(KeyMyField, "default-value")` in `setDefaults()`
4. Add to `config/manager/configmap.yaml` data
5. Env var becomes `ODH_MODULE_OPERATOR_MY_FIELD` automatically

### Adding a Custom Action

```go
func myAction(ctx context.Context, rr *odhtypes.ReconciliationRequest) error {
    module := rr.Instance.(*componentApi.MyModule)
    // modify module.Status, rr.Resources, etc.
    return nil
}
```

Insert via `WithAction(myAction)` at the desired pipeline position.

### Adding a New Module Kind

1. `kubebuilder create api --group components --version v1alpha1 --kind NewModule --namespaced=false`
2. Replace types with PlatformObject contract (embed `common.Status` + `common.ComponentReleaseStatus`)
3. Add CEL singleton validation marker
4. Create `internal/controller/components/newmodule/` (controller, actions, support)
5. Pass `*moduleconfig.Config` to `NewReconciler(ctx, mgr, cfg)`
6. Register in `cmd/operator/operator.go`
7. `make manifests generate build test`

### Build Metadata

The Makefile injects version info via `-ldflags` into `pkg/version`:

```
-X pkg/version.Version=$(VERSION)
-X pkg/version.Commit=$(GIT_COMMIT)
-X pkg/version.Branch=$(GIT_BRANCH)
-X pkg/version.Repo=$(GIT_REPO)
```

These surface in the `reportStatus` action as `status.module.version` and
`status.module.buildSource`.
