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

Creating a new module from a monolith component → use
[odh-component-to-module](../odh-component-to-module/SKILL.md).

## Action Pipeline

Split module operators build a sequential pipeline via `reconciler.ReconcilerFor`.
Each action is `func(context.Context, *ReconciliationRequest) error`.

**Canonical order** (split modules — match monolith + module additions;
`upgradeIfNeeded` must come immediately after `initialize`, with nothing in
between):

```
[component actions e.g. sanitycheck]
-> initialize -> upgradeIfNeeded -> releases -> kustomize -> deploy
-> deployments -> reportStatus -> gc
```

Full porting rules and monolith diff: [controller-rules.md](../odh-component-to-module/references/controller-rules.md).

The root **mymodule** reference uses a different action order — do **not** copy
its pipeline when scaffolding split modules under `modules/`.

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

### Env prefix rule

- **Canonical prefix:** `ODH_MODULE_OPERATOR_` (set via `EnvPrefix` in `pkg/config/config.go`)
- **Forbidden:** `ODH_OPERATOR_`, component-specific prefixes (`ODH_RAY_OPERATOR_*`, etc.)
- **Must match:** deployment env vars in `config/manager/manager.yaml`, Helm chart
  templates, `manager_metrics_patch.yaml`, and `Makefile` `run` target must all
  use the same prefix as `pkg/config`

If the prefix in manifests does not match `EnvPrefix`, viper silently ignores
env vars during `Unmarshal()` — config appears to load but values stay at defaults.

Verification (from module root):

```bash
rg 'ODH_OPERATOR[^_M]|ODH_OPERATOR_|ODH_[A-Z]+_OPERATOR_' . && exit 1 || true
```

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

**Target cluster: OpenShift** (CRC, ROSA, dev cluster). Integration and e2e
tests assume OpenShift APIs (e.g. SCC) are available on the cluster — not a
vanilla Kind cluster. See [testing.md](../odh-component-to-module/references/testing.md)
(OpenShift assumptions) and `docs/testing-limitations.md` for Kind caveats.

Tests use the **Go `testing` package** with **Gomega** and **gomega-matchers**.

| Layer | What we use |
|-------|-------------|
| Runner | `testing.T` — `func TestRay(t *testing.T)`, `TestMain(m *testing.M)` |
| Structure | `t.Run("should become ready", fn)` subtests |
| Assertions | Gomega via `g := NewWithT(t)` inside each subtest |
| K8s polling | `g.Eventually(k.Get(obj)).WithContext(ctx).Should(jq.Match(...))` |
| Matchers | `github.com/lburgazzoli/gomega-matchers` — `k8sm.New`, `jq.Match` |

```go
import (
    . "github.com/onsi/gomega"

    k8sm "github.com/lburgazzoli/gomega-matchers/pkg/matchers/k8s"
    "github.com/lburgazzoli/gomega-matchers/pkg/matchers/jq"
)

func TestRay(t *testing.T) {
    rt := &rayTest{ /* fixtures */ }
    t.Cleanup(func() { _ = k8sClient.Delete(ctx, rt.module) })

    t.Run("should become ready", rt.testBecomesReady)
    t.Run("should report module version and platform", rt.testModuleStatus)
}

func (rt *rayTest) testBecomesReady(t *testing.T) {
    g := NewWithT(t)
    g.Expect(k8sClient.Create(ctx, rt.module)).To(Succeed())
    g.Eventually(k.Get(rt.module)).WithContext(ctx).
        WithTimeout(timeout).WithPolling(interval).
        Should(jq.Match(`.status.phase == "Ready"`))
}
```

Match the ray module pattern above. Use `-run TestRay/should_become_ready`
to isolate a subtest.

**Integration tests** derive expected values from `config/manager/configmap.yaml`
via `support.MustReadConfigMapData()`. Changes to the ConfigMap must be
reflected in test expectations (they are coupled).

**Pre-test cleanup:** integration and e2e require a clean OpenShift cluster.
Module Makefiles define cleanup scripts and `test-integration-run` /
`test-e2e-run` targets. See
`.agents/skills/odh-component-to-module/references/e2e-workflow.md`.

Integration CRDs should be installed by `make prepare-integration` (or
`make test-integration`), not by Go test code. The integration `TestMain`
should fail fast if the expected module CRD is missing, and the top-level
integration/e2e tests should use `Eventually` / `Consistently` to verify stale
singleton CRs are gone before creating a fresh one.

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

### Baseline Module RBAC

Every split module operator keeps a small baseline RBAC set in addition to the
resource-specific markers derived from `Owns()`, `Watches()`, and operand
ClusterRoles:

1. **CRDs** — every module operator includes:
   ```go
   // +kubebuilder:rbac:groups=apiextensions.k8s.io,resources=customresourcedefinitions,verbs=get;list;watch;create;update;patch;delete
   ```
2. **Protected metrics** — if the manager exposes `/metrics`, keep:
   ```go
   // +kubebuilder:rbac:groups=authentication.k8s.io,resources=tokenreviews,verbs=create
   // +kubebuilder:rbac:groups=authorization.k8s.io,resources=subjectaccessreviews,verbs=create
   // +kubebuilder:rbac:urls=/metrics,verbs=get
   ```

These markers are baseline module-operator conventions. Do not wait for the
manifest audit to discover them, because `kustomize build` only reveals operand
RBAC and does not infer the controller's protected-metrics endpoint.

After adding or changing these markers, run `make manifests generate` so
`config/rbac/role.yaml` stays in sync.

### Adding a Config Key

1. Add constant: `KeyMyField = "my-field"` in `pkg/config/config.go`
2. Add field: `MyField string \`mapstructure:"my-field"\`` in `Config` struct
3. Add default: `v.SetDefault(KeyMyField, "default-value")` in `setDefaults()`
4. Add to `config/manager/configmap.yaml` data
5. Env var becomes `ODH_MODULE_OPERATOR_MY_FIELD` automatically — never
   hand-craft a different prefix

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

For a **split module** under `modules/`, use
[odh-component-to-module](../odh-component-to-module/SKILL.md) — do not
hand-scaffold from mymodule.

To extend the **root reference operator** only:

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
