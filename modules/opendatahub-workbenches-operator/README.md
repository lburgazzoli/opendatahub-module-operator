# opendatahub-workbenches-operator

Standalone module operator for the cluster-scoped singleton `Workbenches`
component (`components.platform.opendatahub.io/v1alpha1`). It renders the
embedded notebook-controller, kf-notebook-controller, and notebook image
manifests, computes dynamic kustomize parameters for gateway integration and
MLflow awareness, manages the target workbench namespace, and reports standard
module status.

## What It Does

- Reconciles the singleton `Workbenches` instance named `default-workbenches`
- Renders three embedded manifest bundles for the notebook controllers and
  notebook images
- Uses the embedded ODH render base by design and injects dynamic params for
  `gateway-url`, `section-title`, and `mlflow-enabled`
- Renders the embedded `openshift-config-grants` template alongside the
  kustomized manifests
- Creates the workbench namespace with
  `opendatahub.io/generated-namespace=true`
- Uses `spec.workbenchNamespace` when set, otherwise defaults to `opendatahub`
  on ODH and `rhods-notebooks` on RHOAI platforms
- Watches MLflow installation state so notebook image rendering can reflect
  whether MLflow support should be enabled
- Runs upgrade migrations that convert legacy notebook hardware annotations into
  HardwareProfile resources
- Reports `Ready`, `ProvisioningSucceeded`, `ImageStreamsAvailable`,
  `status.releases`, and `status.workbenchNamespace`

## Module-Specific Notes

Compared with some of the other split modules:

- the controller renders three manifest roots instead of a single component
  overlay
- `platformType` is used for namespace defaults and UI text, but the render
  base itself is fixed to the embedded ODH layout
- the controller watches the MLflow module CRD and singleton resource so the
  `mlflow-enabled` param can be recomputed when MLflow is installed or removed
- the upgrade path includes notebook metadata migrations and emits an
  `UpgradeStarted` event when a platform version advance triggers those
  migrations
- webhook resources are part of the managed operand set when
  `controller.webhook.enabled=true`

## Configuration

The operator loads configuration from defaults, then a mounted ConfigMap, then
`ODH_MODULE_OPERATOR_*` environment variable overrides.

Common keys:

| Key | Default | Description |
|---|---|---|
| `applications-namespace` | `opendatahub` | Namespace where controller-managed resources are deployed |
| `platformType` | `OpenDataHub` | Platform selector used for section-title and namespace defaults |
| `platformVersion` | `""` | Platform version reported in `status.releases` |

The controller also supports `controller.*` keys for metrics, health probes,
leader election, zap logging, pprof, and webhook enablement.

When deployed through Helm, `.Values.platform.type`, `.Values.platform.version`,
and `.Values.config` are written into the operator ConfigMap.

## Quick Start

```bash
# Fetch embedded manifests and run local checks
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
make test-upgrade-run
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

# Upgrade tests
make test-upgrade
```

If you prefer using the internal OpenShift registry for e2e images, use the
split flow (`cleanup-e2e`, build/push, `deploy-helm`, `test-e2e-run`) instead
of `make test-e2e`, because the cleanup step can remove the namespace or image
stream where the image was published.

## Architecture

See `docs/index.md` in the root of this repository for the broader split plan
and design context.
