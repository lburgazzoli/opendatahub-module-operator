# AGENTS.md — ODH Module Operator

## Module Split

This repo is a monorepo of module operators under `modules/`. The runnable
example operator lives at `modules/opendatahub-mymodule-operator/`. See
`docs/index.md` for the full plan, completed modules, lessons learned, and
links to all reference docs.

To create a new module from a monolith component, use the
`odh-component-to-module` skill.

## Architecture

This is an ODH Module Operator — a standalone controller that reconciles a
cluster-scoped singleton CR using the opendatahub-operator's action pipeline.
It externalizes what was previously a `ComponentHandler` inside the monolithic
ODH Operator.

The controller does NOT use a hand-written `Reconcile()` loop. It uses
`reconciler.ReconcilerFor` from the operator's `pkg/controller/reconciler/`
which runs a sequential action pipeline: each action is a
`func(context.Context, *ReconciliationRequest) error`.

## Critical Rules

### Auto-Generated — Never Edit

| Path | Generator |
|---|---|
| `config/crd/bases/*.yaml` | `make manifests` (controller-gen) |
| `config/rbac/role.yaml` | `make manifests` (controller-gen) |
| `**/zz_generated.deepcopy.go` | `make generate` (controller-gen) |
| `config/chart/` | `make helm` (chartgen subcommand) |
| `config/manager/kustomization.yaml` | `kustomize edit` |

### After Changing Types or Markers

```
make manifests generate
```

Always run both. `manifests` regenerates CRDs and RBAC from markers.
`generate` regenerates DeepCopy methods.

### Scaffold Markers

Do NOT delete `// +kubebuilder:scaffold:*` comments.

## Project Layout

Each module is a multi-group kubebuilder project (`multigroup: true` in
`PROJECT`). The example lives under
`modules/opendatahub-mymodule-operator/`, and other split modules follow the
same shape:

```
modules/$name/
  api/components/v1alpha1/     CRD types
  cmd/main.go                  Cobra root command
  cmd/operator/                Operator subcommand (manager lifecycle)
  cmd/chartgen/                Helm chart generator
  internal/controller/$name/   Controller (ReconcilerFor + actions)
  pkg/cache/                   Cache transform (StripUnusedFields)
  pkg/config/                  Operator config (viper + ConfigMap loading)
  pkg/resources/gvk/           Module-local GVK registry for controllers and chartgen
  pkg/version/                 Build metadata (ldflags)
  config/manifests/            Workload manifests (kustomize overlays per platform)
  config/manager/              Operator Deployment + ConfigMap
  hack/scripts/                Cluster/test helper scripts
  test/integration/            In-process manager tests
  test/e2e/                    Deployed operator tests
  test/support/                Shared test helpers
```

Uses `Containerfile` (not Dockerfile) and `podman` (not docker).
Use `local.mk` (gitignored) for local Makefile overrides.

The root `Makefile` is monorepo orchestration only. Run operator-specific
targets from the module directory.

## Action Pipeline

Split modules under `modules/` follow the monolith order with module
additions — see
`.agents/skills/odh-component-to-module/references/controller-rules.md`.

The example **mymodule** controller under
`modules/opendatahub-mymodule-operator/` is kept as a runnable example. Use
the split-module pipeline rules, not legacy root layout assumptions.

```
[component actions] -> initialize -> upgradeIfNeeded -> releases -> kustomize
-> deploy -> deployments -> reportStatus -> gc
```

See `.agents/skills/odh-module-scaffold/SKILL.md` for action descriptions.

## RBAC / Owns / Manifests Consistency

Three things must stay in sync whenever the workload manifests change:

1. **`config/manifests/`** — the kustomize resources the controller deploys
2. **`Owns()` calls** on the `ReconcilerFor` builder in the controller
3. **`+kubebuilder:rbac` markers** on the controller file

If a manifest introduces a new resource type (e.g., `NetworkPolicy`), the
controller must also `Owns()` it (so changes trigger reconciliation) and
have the RBAC marker (so the operator has permission to manage it). Missing
any one of the three causes silent failures: permission errors, missed
reconciliation triggers, or orphaned resources.

After changing manifests, review all three and run `make manifests generate`.

In addition to manifest-derived RBAC, every module operator keeps baseline
controller RBAC:

- **CRDs** — include `customresourcedefinitions`
  `get;list;watch;create;update;patch;delete`
- **Protected metrics** — when the manager exposes `/metrics`, include
  `tokenreviews create`, `subjectaccessreviews create`, and `urls=/metrics get`

Treat these as default module-operator markers even when they are not obvious
from the fetched operand manifests alone.

The cache is configured with `ReaderFailOnMissingInformer: true`. A `Get`
or `List` for a resource type with no running informer returns
`ErrResourceNotCached` instead of silently doing a live API call. This
catches missing `Owns()` or `Watches()` declarations at runtime.

## GVK Rule

Each split module defines its own GVK package at `pkg/resources/gvk/gvk.go`.
Inside a module, controllers and `cmd/chartgen/` must import that local
package instead of importing
`github.com/opendatahub-io/opendatahub-operator/v2/pkg/cluster/gvk`
directly.

The local package is the module's internal source of truth for GVK values:

- Re-export upstream GVKs when the upstream package already defines them
- Define module-only GVKs locally when upstream has no constant
- Keep the shared chartgen GVKs there too, so chart generation and controller
  code use the same import path

## Configuration

See `modules/opendatahub-mymodule-operator/pkg/config/config.go` and SKILL.md
for the three-layer config model.
Env var prefix: `ODH_MODULE_OPERATOR_`.

## Operator Dependency

The `go.mod` uses a `replace` directive pointing to a fork
(`lburgazzoli/opendatahub-operator`) that moves `internal/controller/status`
constants to `pkg/controller/status/` to make them importable. Track upstream
PR to remove this replace.

The operator's `cluster.ApplicationNamespace()` reads from the viper key
`rhai-applications-namespace`. The module operator sets this in
`cmd/operator/operator.go` via both `viper.Set()` and
`cluster.SetRHAIApplicationNamespace()` to avoid requiring DSCI.

## Manager Wrapper

The `ctrl.Manager` is wrapped by `odhmanager.New()` with
`odhmanager.WithManifestsBasePath()`. This wrapper is what makes
`rr.ManifestsBasePath` available inside actions. If you pass the raw
`ctrlMgr` instead of the wrapped `mgr` to `NewReconciler`, manifest paths
will be empty and rendering will fail. Integration tests must use the same
wrapper pattern.

## Testing

Default cluster for module integration/e2e: **OpenShift** (CRC, ROSA, dev).
Kind is optional — see `docs/testing-limitations.md`.

| Target | Scope | Requires |
|---|---|---|
| `make test` | Unit tests (compilation, vet) | Nothing |
| `make test-integration` | In-process manager against cluster | OpenShift kubeconfig; runs cleanup first |
| `make test-e2e` | Against deployed operator | OpenShift; `cleanup-e2e deploy-helm test-e2e-run` |

Tests use the Go `testing` package with Gomega and `gomega-matchers`
(`github.com/lburgazzoli/gomega-matchers`) for JQ-based k8s assertions:
`TestMain`, `func TestX(t *testing.T)`, `t.Run` subtests, and
`g := NewWithT(t)` with `g.Eventually(k.Get(obj)).Should(jq.Match(...))`.
Integration tests derive expected values from `config/manager/configmap.yaml`
via `support.MustReadConfigMapData()` — no hardcoded assertion values.


## Image Caching

Both the kustomize and Helm deploy paths set `imagePullPolicy: Always`.
When iterating locally with the same tag, Kubernetes still re-pulls. For
extra safety (or if the policy is overridden), use a unique tag per build:

```sh
cd modules/opendatahub-mymodule-operator
IMG=ttl.sh/opendatahub-mymodule-operator-$(uuidgen):1h \
  make container-build container-push deploy-helm
```

For e2e verification, this ephemeral `ttl.sh` flow is the preferred default.
Using a fresh short-lived tag per run avoids stale image cache issues and makes
debugging deploy/test failures much more predictable.

## Make Targets

All tools use `go run <module>@<version>` — no local binary downloads.
Run module-local `make help` for operator targets and root `make help` for
aggregate monorepo targets. Key root targets:

| Target | Purpose |
|---|---|
| `make list-modules` | Print module directories covered by aggregate targets |
| `make test-modules` | Run unit-test workflows across tracked modules |
| `make lint-modules` | Run lint across tracked modules |
| `make verify-all` | Run aggregate verification across tracked modules |
