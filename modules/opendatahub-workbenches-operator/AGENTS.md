# AGENTS.md — opendatahub-workbenches-operator

Standalone module operator reconciling the cluster-scoped singleton
`Workbenches` CR (API group `components.platform.opendatahub.io`).

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

- Reconciles only the singleton instance `default-workbenches`
- Renders three embedded manifest bundles: `odh-notebook-controller`,
  `kf-notebook-controller`, and notebook image resources
- Uses the embedded ODH render base and computes per-reconcile params for
  `gateway-url`, `section-title`, and `mlflow-enabled`
- Renders the `openshift-config-grants` template from embedded manifests
- Creates the target workbench namespace with
  `opendatahub.io/generated-namespace=true`
- Uses `spec.workbenchNamespace` when set, otherwise defaults to `opendatahub`
  on ODH and `rhods-notebooks` on RHOAI platforms
- Watches the MLflow module CRD and singleton CR so notebook image rendering can
  react when MLflow becomes enabled or disabled
- Runs upgrade migrations that convert legacy notebook accelerator and
  container-size annotations into HardwareProfile resources
- Reports deployment readiness, image stream readiness, `status.releases`, and
  `status.workbenchNamespace`

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

**Upgrade tests** — requires a cluster:
```sh
make test-upgrade               # cleanup + setup + run in one step
# or split:
make test-upgrade-setup         # install CRDs, prepare cluster
make test-upgrade-run           # run tests only
make test-upgrade-cleanup       # clean up
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
| `platformType` | `OpenDataHub` | Platform identifier used for section-title and namespace defaults |
| `platformVersion` | `""` | Platform version reported in `status.releases` |

Supported `controller.*` keys:

| Key | Default |
|---|---|
| `controller.metrics.bind-address` | `:8080` |
| `controller.health.bind-address` | `:8081` |
| `controller.leader-election.enabled` | `true` |
| `controller.leader-election.id` | `odh-workbenches-lock` |
| `controller.zap.level` | `info` |
| `controller.zap.dev-mode` | `false` |
| `controller.zap.encoder` | `""` |
| `controller.pprof.enabled` | `false` |
| `controller.pprof.bind-address` | `""` |
| `controller.webhook.enabled` | `true` |

When deployed through Helm, `.Values.platform.type`, `.Values.platform.version`,
and `.Values.config` are written into the operator ConfigMap.

## Key Make Targets

| Target | Purpose |
|---|---|
| `make test` | Run unit tests |
| `make test-integration` | Prepare cluster and run integration tests |
| `make test-e2e` | Clean up, deploy via Helm, and run e2e tests |
| `make test-e2e-run` | Run e2e tests against an existing deployment |
| `make test-upgrade` | Prepare cluster and run upgrade tests |
| `make lint` | Run golangci-lint |
| `make fmt` | Run golangci-lint formatters |
| `make manifests generate` | Regenerate CRDs, RBAC, and DeepCopy |
| `make get-manifests` | Download embedded workbenches manifests |
| `make helm` | Generate Helm chart |
