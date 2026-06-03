# AGENTS.md — ODH Module Operator

Monorepo of standalone module operators under `modules/`. Each module
reconciles a cluster-scoped singleton CR using the opendatahub-operator's
action pipeline (`reconciler.ReconcilerFor`).

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
| **odh-module-migrate** | Migrating a monolith component into a standalone module |
| **odh-manifest-audit** | After `make get-manifests` or manifest changes — RBAC/Owns sync |

## Conventions

- Uses `Containerfile` (not Dockerfile) and `podman` (not docker)
- Use `local.mk` (gitignored) for local Makefile overrides
- Use `direnv` with `.envrc` (gitignored) for local environment variables
- Root `Makefile` is monorepo orchestration; run module targets from `modules/$name/`
- The `go.mod` uses a `replace` directive for `lburgazzoli/opendatahub-operator`
  (moves `internal/controller/status` → `pkg/controller/status`)

## Current Modules

`opendatahub-mymodule-operator` (example),
`opendatahub-ray-operator`,
`opendatahub-spark-operator`,
`opendatahub-feast-operator`,
`opendatahub-ogx-operator`,
`opendatahub-mlflow-operator`,
`opendatahub-trustyai-operator`,
`opendatahub-trainer-operator`,
`opendatahub-modelregistry-operator`,
`opendatahub-datasciencepipelines-operator`,
`opendatahub-workbenches-operator`

## Root Make Targets

| Target | Purpose |
|---|---|
| `make list-modules` | Print module directories |
| `make test-modules` | Unit tests across modules |
| `make lint-modules` | Lint across modules |
| `make verify-all` | Aggregate verification |
