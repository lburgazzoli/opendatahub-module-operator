# opendatahub-mlflow-operator

Standalone module operator for the cluster-scoped singleton
`MLflowOperator` component
(`components.platform.opendatahub.io/v1alpha1`). It renders and applies the
bundled MLflow manifests, computes gateway-backed runtime parameters for the
console link, patches the rendered deployment namespace, and reports standard
readiness and release status.

## What It Does

- Reconciles the singleton `MLflowOperator` instance named
  `default-mlflowoperator`
- Selects the embedded manifest overlay based on `platformType`
  (`OpenDataHub` uses `overlays/odh`; `SelfManagedRhoai` and `ManagedRhoai`
  use `overlays/rhoai`)
- Injects image parameters into `base/params.env` before render/apply
- Computes `mlflow-url` and `section-title` from `GatewayConfig.status.domain`
  and platform type at reconcile time
- Renders the embedded `openshift-config-grants` template alongside the
  kustomized manifests
- Rewrites the rendered mlflow Deployment `--namespace=` argument to the
  configured applications namespace before deploy
- Watches the MLflow CRD dynamically via the module-local `pkg/resources/gvk`
  package
- Reports `Ready`, `ProvisioningSucceeded`, and `status.releases`

## Gateway Domain and Namespace Handling

The main MLflow-specific runtime behavior is in `customizeManifests()` and
`fixDeploymentNamespace()`.

- `customizeManifests()` reads `GatewayConfig.status.domain` using an
  unstructured client and writes `mlflow-url` / `section-title` into
  `base/params.env`
- if `GatewayConfig` is missing, its CRD is absent, or `status.domain` is
  empty, reconciliation currently skips those params rather than requeueing
- `fixDeploymentNamespace()` patches the rendered Deployment args so tests and
  custom installs can use the configured applications namespace instead of the
  overlay’s hardcoded default

## Configuration

The operator loads configuration from defaults, then a mounted ConfigMap, then
`ODH_MODULE_OPERATOR_*` environment variable overrides.

Common keys:

| Key | Default | Description |
|---|---|---|
| `applications-namespace` | `opendatahub` | Namespace where managed MLflow resources are deployed |
| `platformType` | `OpenDataHub` | Platform selector used to choose the ODH or RHOAI overlay |
| `platformVersion` | `""` | Platform version reported in `status.releases` |

The controller also supports `controller.*` keys for metrics, health probes,
leader election, zap logging, and pprof settings.

When deployed through Helm, `.Values.platform.type`, `.Values.platform.version`,
and `.Values.config` are written into the operator ConfigMap.

## Quick Start

```bash
# Fetch embedded MLflow manifests and run local checks
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

## Current Limitations

- `internal/controller/mlflowoperator/mlflowoperator_actions.go` still has
  TODOs around whether missing/empty `GatewayConfig.status.domain` should cause
  a requeue instead of silently skipping console-link params
- `internal/controller/mlflowoperator/mlflowoperator_upgrade.go` is wired into
  reconciliation, but does not yet contain version-gated migrations

## Architecture

See `docs/index.md` in the root of this repository for the broader split plan
and design context.
