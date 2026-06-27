# AGENTS.md — opendatahub-trainer-operator

Standalone module operator reconciling a cluster-scoped singleton
`Trainer` CR (API group `components.opendatahub.io`).

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

## Running Tests

**Unit tests** — no cluster required:
```sh
make test
```

**Integration tests** — requires a cluster (installs CRDs, runs against real API server):
```sh
make test-integration           # setup + run in one step
# or split:
make test-integration-setup     # install CRDs, prepare cluster
make test-integration-run       # run tests only
make test-integration-cleanup   # clean up
```

**E2E tests** — requires a cluster with the operator image published:
```sh
IMG="ttl.sh/$(uuidgen | tr '[:upper:]' '[:lower:]'):1h"
make container-build IMG="$IMG"
make container-push IMG="$IMG"
make helm
make test-e2e-cleanup
make deploy-helm IMG="$IMG"
make test-e2e-run
make test-e2e-cleanup
```

## ConfigMap Fields

The operator reads configuration from a mounted ConfigMap:

| Key | Default | Description |
|-----|---------|-------------|
| `platform-name` | `unknown` | Platform identifier (e.g. `OpenDataHub`, `SelfManagedRHOAI`) |
| `platform-version` | `unknown` | Platform operator version (semver string) |
| `applications-namespace` | `opendatahub` | Namespace where module workloads are deployed |

## Key Make Targets

| Target | Purpose |
|---|---|
| `make lint` | Run golangci-lint |
| `make fmt` | Run golangci-lint formatters |
| `make manifests generate` | Regenerate CRDs, RBAC, DeepCopy |
| `make helm` | Generate Helm chart |
