# AGENTS.md — opendatahub-ray-operator

Standalone module operator reconciling the cluster-scoped singleton
`Ray` CR (API group `components.platform.opendatahub.io`).

## Auto-Generated — Never Edit

| Path | Generator |
|---|---|
| `config/crd/bases/*.yaml` | `make manifests` (controller-gen) |
| `config/rbac/role.yaml` | `make manifests` (controller-gen) |
| `**/zz_generated.deepcopy.go` | `make generate` (controller-gen) |
| `config/chart/` | `make helm` (chartgen subcommand) |

After changing types or markers: `make manifests generate` (always both).
Do NOT delete `// +kubebuilder:scaffold:*` comments.

## Skills

| Skill | Use when |
|-------|----------|
| **odh-module-dev** | Modifying the reconciler, adding actions, config keys, extending modules |
| **odh-module-test** | Writing or running tests, debugging test failures |
| **odh-module-deploy** | Building images, Helm charts, deploying to OpenShift |
| **odh-manifest-audit** | After `make get-manifests` or manifest changes — RBAC/Owns sync |

## Conventions

- Uses `Containerfile` (not Dockerfile) and `podman` (not docker)
- Use `local.mk` (gitignored) for local Makefile overrides
- Use `direnv` with `.envrc` (gitignored) for local environment variables
- The `go.mod` uses a `replace` directive for `lburgazzoli/opendatahub-operator`
  (moves `internal/controller/status` → `pkg/controller/status`)

## Module Behavior

- Reconciles only the singleton instance `default-ray`
- Uses the embedded `openshift` overlay for all platforms
- Applies image substitutions to `params.env` once at startup from environment
  variables
- Updates the rendered `namespace` kustomize param during `initialize()` to
  match the configured applications namespace
- Renders the `openshift-config-grants` template from embedded manifests
- Reports `Ready`, `ProvisioningSucceeded`, and `status.releases`

## Running Tests

**Unit tests** — no cluster required:
```sh
make test
```

**Integration tests** — requires a cluster:
```sh
make test-integration           # setup + run in one step
# or split:
make test-integration-setup     # install CRDs, prepare cluster
make test-integration-run       # run tests only
make test-integration-cleanup   # clean up
```

**E2E tests** — requires a cluster and a published operator image:
```sh
IMG="ttl.sh/$(uuidgen | tr '[:upper:]' '[:lower:]'):1h"
make container-build IMG="$IMG"
make container-push IMG="$IMG"
make helm
make test-e2e IMG="$IMG"        # cleanup + deploy + run

# or, if the operator is already deployed:
make test-e2e-run
```

## Operator Configuration

Configuration precedence:

1. Built-in defaults
2. Mounted ConfigMap files from `ODH_MODULE_OPERATOR_CONFIGURATION_PATH`
3. `ODH_MODULE_OPERATOR_*` environment variables

Core keys:

| Key | Default | Description |
|-----|---------|-------------|
| `applications-namespace` | `opendatahub` | Namespace where module workloads are deployed |
| `platformType` | `OpenDataHub` | Platform identifier reported in status and Helm config |
| `platformVersion` | `""` | Platform version reported in `status.releases` |

Supported `controller.*` keys:

| Key | Default |
|---|---|
| `controller.metrics.bind-address` | `:8080` |
| `controller.health.bind-address` | `:8081` |
| `controller.leader-election.enabled` | `true` |
| `controller.leader-election.id` | `odh-ray-lock` |
| `controller.zap.level` | `info` |
| `controller.zap.dev-mode` | `false` |
| `controller.zap.encoder` | `""` |
| `controller.pprof.enabled` | `false` |
| `controller.pprof.bind-address` | `""` |

When deployed through Helm, `.Values.platform.type`, `.Values.platform.version`,
and `.Values.config` are written into the operator ConfigMap.

## Key Make Targets

| Target | Purpose |
|---|---|
| `make test` | Run unit tests |
| `make test-integration` | Prepare cluster and run integration tests |
| `make test-e2e` | Clean up, deploy via Helm, and run e2e tests |
| `make test-e2e-run` | Run e2e tests against an existing deployment |
| `make lint` | Run golangci-lint |
| `make fmt` | Run golangci-lint formatters |
| `make manifests generate` | Regenerate CRDs, RBAC, and DeepCopy |
| `make get-manifests` | Download embedded Ray manifests |
| `make helm` | Generate Helm chart |
