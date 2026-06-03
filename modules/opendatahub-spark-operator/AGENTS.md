# AGENTS.md — opendatahub-spark-operator

Standalone module operator reconciling a cluster-scoped singleton
`SparkOperator` CR (API group `components.opendatahub.io`).

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

**E2E tests** — requires a cluster (deploys the operator via Helm, then tests):
```sh
make test-e2e                   # cleanup + deploy + run in one step
# or split:
make test-e2e-setup             # prepare cluster
make deploy-helm                # deploy operator
make test-e2e-run               # run tests only
make test-e2e-cleanup           # clean up and undeploy
```

## Key Make Targets

| Target | Purpose |
|---|---|
| `make lint` | Run golangci-lint |
| `make fmt` | Run golangci-lint formatters |
| `make manifests generate` | Regenerate CRDs, RBAC, DeepCopy |
| `make helm` | Generate Helm chart |
| `make deploy-openshift` | Build, push to OpenShift registry, deploy |
