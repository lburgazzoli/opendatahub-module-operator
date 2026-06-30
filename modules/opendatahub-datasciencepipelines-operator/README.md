# opendatahub-datasciencepipelines-operator

Standalone module operator for the cluster-scoped singleton
`DataSciencePipelines` component
(`components.platform.opendatahub.io/v1alpha1`). It renders and applies the
bundled Data Science Pipelines manifests, reports readiness through standard
component conditions, and guards reconciliation around Argo Workflows CRD
ownership.

## What It Does

- Reconciles the singleton `DataSciencePipelines` instance named
  `default-datasciencepipelines`
- Selects the embedded manifest overlay based on `platformType`
  (`OpenDataHub` uses `overlays/odh`; `SelfManagedRhoai` and `ManagedRhoai`
  use `overlays/rhoai`)
- Injects platform version, FIPS, image, and Argo controller settings into the
  embedded manifests before render/apply
- Deploys managed resources into the configured applications namespace and
  garbage collects owned resources
- Reports `Ready`, `ProvisioningSucceeded`, `ArgoWorkflowAvailable`, and
  `status.releases`

## Argo Workflows Management

The main module-specific API knob is
`spec.argoWorkflowsControllers.managementState`.

- `Managed` is the default. The module expects to manage the bundled Argo
  Workflows controllers. If `workflows.argoproj.io` already exists but is not
  labeled as ODH-owned, reconciliation stops.
- `Removed` disables management of the bundled Argo Workflows controllers. In
  this mode, the `workflows.argoproj.io` CRD must already exist or
  reconciliation stops with `ArgoWorkflowAvailable=False`.

Integration and e2e coverage includes both negative paths above, plus the
ready-path where an ODH-owned Argo Workflows CRD is present.

## Configuration

The operator loads configuration from defaults, then a mounted ConfigMap, then
`ODH_MODULE_OPERATOR_*` environment variable overrides.

Common keys:

| Key | Default | Description |
|---|---|---|
| `applications-namespace` | `opendatahub` | Namespace where managed DSP resources are deployed |
| `platformType` | `OpenDataHub` | Platform selector used to choose the ODH or RHOAI overlay |
| `platformVersion` | `""` | Platform version reported in `status.releases` |

The controller also supports `controller.*` keys for metrics, health probes,
leader election, zap logging, and pprof settings.

When deployed through Helm, `.Values.platform.type`, `.Values.platform.version`,
and `.Values.config` are written into the operator ConfigMap.

## Quick Start

```bash
# Fetch embedded DSP manifests and run local checks
make get-manifests
make test lint

# Build and publish an operator image
IMG="ttl.sh/$(uuidgen | tr '[:upper:]' '[:lower:]'):1h"
make container-build IMG="$IMG"
make container-push IMG="$IMG"

# Generate the chart and deploy the operator
make helm
make deploy-helm IMG="$IMG"

# Run cluster-backed tests
make test-integration
make test-e2e-run
```

## Test Flows

```bash
# Unit tests
make test

# Integration tests (prepare cluster, then run)
make test-integration

# E2E tests with deploy included
make test-e2e IMG="$IMG"

# E2E tests against an already deployed operator
make test-e2e-run
```

## Current Limitation

The upgrade hook is wired into reconciliation, but
`internal/controller/datasciencepipelines/datasciencepipelines_upgrade.go` does
not yet contain version-gated migrations.

## Architecture

See `docs/index.md` in the root of this repository for the broader split plan
and design context.
