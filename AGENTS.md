# AGENTS.md — ODH Module Operator

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

Multi-group kubebuilder project (`multigroup: true` in PROJECT).

```
api/components/v1alpha1/       CRD types (MyModule, ModuleStatus, etc.)
cmd/main.go                    Cobra root command
cmd/operator/                  Operator subcommand (manager lifecycle)
cmd/chartgen/                  Helm chart generator (reads kustomize YAML from stdin)
internal/controller/components/mymodule/   Controller (ReconcilerFor + actions)
pkg/cache/                     Cache transform (StripUnusedFields)
pkg/config/                    Operator config (viper + ConfigMap loading)
pkg/version/                   Build metadata (ldflags)
config/manifests/              Workload manifests (kustomize overlays per platform)
config/manager/                Operator Deployment + ConfigMap
hack/scripts/                  kind-setup.sh
test/integration/              In-process manager against Kind
test/e2e/                      Against deployed operator
test/support/                  Shared test helpers
```

Uses `Containerfile` (not Dockerfile) and `podman` (not docker).
Use `local.mk` (gitignored) for local Makefile overrides.

## Action Pipeline

The controller pipeline in `mymodule_controller.go`:

```
initialize -> releases -> reportStatus -> kustomize -> deploy -> deployments -> gc
```

See SKILL.md for detailed action descriptions and how to add custom actions.

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

The cache is configured with `ReaderFailOnMissingInformer: true`. A `Get`
or `List` for a resource type with no running informer returns
`ErrResourceNotCached` instead of silently doing a live API call. This
catches missing `Owns()` or `Watches()` declarations at runtime.

## Configuration

See `pkg/config/config.go` and SKILL.md for the three-layer config model.
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

| Target | Scope | Requires |
|---|---|---|
| `make test` | Unit tests (compilation, vet) | Nothing |
| `make test-integration` | In-process manager against Kind | `make kind-create` |
| `make test-e2e` | Against deployed operator | Operator deployed via Helm |

Tests use Ginkgo BDD style (`Describe`/`It`) with `gomega-matchers`
(`github.com/lburgazzoli/gomega-matchers`) for JQ-based k8s assertions.
Integration tests derive expected values from `config/manager/configmap.yaml`
via `support.MustReadConfigMapData()` — no hardcoded assertion values.

## Make Targets

All tools use `go run <module>@<version>` — no local binary downloads.
Run `make help` for the full list. Key non-obvious targets:

| Target | Purpose |
|---|---|
| `make helm` | Generate Helm chart from kustomize via `chartgen` subcommand |
| `make test-integration` | In-process manager against Kind cluster |
| `make test-e2e` | Against deployed operator (requires Helm deploy first) |
| `make kind-create` | Create Kind cluster with podman + cert-manager |
| `make deploy-helm` | Deploy via Helm (`--set-string` for image tag) |
