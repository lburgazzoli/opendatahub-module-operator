# opendatahub-trainer-operator

Standalone module operator for the cluster-scoped singleton `Trainer`
component (`components.platform.opendatahub.io/v1alpha1`). It renders and
applies the bundled Trainer manifests, injects image parameters into the
embedded kustomize overlay, blocks reconciliation until required JobSet
dependencies exist, and reports readiness plus release status.

## What It Does

- Reconciles the singleton `Trainer` instance named `default-trainer`
- Uses the embedded `rhoai` overlay for all supported platforms
- Injects image substitutions into `params.env` before render/apply
- Renders the embedded `openshift-config-grants` template alongside the
  kustomized manifests
- Waits for the JobSet operator CRD, the cluster-scoped JobSet operator CR, and
  the JobSet CRD before continuing reconciliation
- Reports `Ready`, `ProvisioningSucceeded`, `status.releases`, and the legacy
  `status.release` field

## Module-Specific Notes

Compared with some of the other split modules:

- this module ships only a single `rhoai` manifest overlay
- readiness is gated by dependency checks instead of going directly from module
  creation to manifest application
- integration and e2e tests can create lightweight JobSet stubs to satisfy
  those preconditions when the real dependency stack is absent

## Configuration

The operator loads configuration from defaults, then a mounted ConfigMap, then
`ODH_MODULE_OPERATOR_*` environment variable overrides.

Common keys:

| Key | Default | Description |
|---|---|---|
| `applications-namespace` | `opendatahub` | Namespace where managed Trainer resources are deployed |
| `platformType` | `OpenDataHub` | Platform identifier reported through status and Helm config |
| `platformVersion` | `""` | Platform version reported in `status.releases` |

The controller also supports `controller.*` keys for metrics, health probes,
leader election, zap logging, and pprof settings.

When deployed through Helm, `.Values.platform.type`, `.Values.platform.version`,
and `.Values.config` are written into the operator ConfigMap.

## Quick Start

```bash
# Fetch embedded Trainer manifests and run local checks
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
