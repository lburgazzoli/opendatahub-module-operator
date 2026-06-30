# opendatahub-ray-operator

Standalone module operator for the cluster-scoped singleton `Ray` component
(`components.platform.opendatahub.io/v1alpha1`). It renders and applies the
bundled Ray manifests, injects image and namespace parameters into the embedded
kustomize overlay, and reports standard readiness and release status.

## What It Does

- Reconciles the singleton `Ray` instance named `default-ray`
- Uses the embedded `openshift` manifest overlay
- Injects image parameters into `params.env` before render/apply
- Updates the rendered `namespace` kustomize param to the configured
  applications namespace during `initialize()`
- Renders the embedded `openshift-config-grants` template alongside the
  kustomized manifests
- Reports `Ready`, `ProvisioningSucceeded`, and `status.releases`

## Module-Specific Notes

Compared with some other split modules:

- this module always uses the `openshift` overlay rather than switching between
  ODH and RHOAI overlays
- the `platformType` value is still loaded and reported in status / Helm config
- the Ray operator image is injected through the
  `RELATED_IMAGE_ODH_KUBERAY_OPERATOR_CONTROLLER_IMAGE` environment variable
- `initialize()` writes the target workload namespace into `params.env` on each
  reconcile so the rendered resources follow `applications-namespace`

## Configuration

The operator loads configuration from defaults, then a mounted ConfigMap, then
`ODH_MODULE_OPERATOR_*` environment variable overrides.

Common keys:

| Key | Default | Description |
|---|---|---|
| `applications-namespace` | `opendatahub` | Namespace where managed Ray resources are deployed |
| `platformType` | `OpenDataHub` | Platform identifier reported in status and Helm config |
| `platformVersion` | `""` | Platform version reported in `status.releases` |

The controller also supports `controller.*` keys for metrics, health probes,
leader election, zap logging, and pprof settings.

When deployed through Helm, `.Values.platform.type`, `.Values.platform.version`,
and `.Values.config` are written into the operator ConfigMap.

## Quick Start

```bash
# Fetch embedded Ray manifests and run local checks
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

## Architecture

See `docs/index.md` in the root of this repository for the broader split plan
and design context.
