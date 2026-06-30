# opendatahub-ogx-operator

Standalone module operator for the cluster-scoped singleton `OGX` component
(`components.platform.opendatahub.io/v1alpha1`). It renders and applies the
bundled OGX manifests, injects image parameters into the embedded kustomize
overlay, and reports standard readiness and release status.

## What It Does

- Reconciles the singleton `OGX` instance named `default-ogx`
- Selects the embedded manifest overlay based on `platformType`
  (`OpenDataHub` uses `overlays/odh`; `SelfManagedRhoai` and `ManagedRhoai`
  use `overlays/rhoai`)
- Injects image parameters into `params.env` before render/apply
- Renders the embedded `openshift-config-grants` template alongside the
  kustomized manifests
- Reports `Ready`, `ProvisioningSucceeded`, and `status.releases`

## Module-Specific Notes

Compared with some of the other split modules:

- the split operator keeps the monolith’s two OGX image params:
  `RELATED_IMAGE_ODH_OGX_OPERATOR` and `RELATED_IMAGE_RH_DISTRIBUTION`
- the old monolith precondition check is intentionally a no-op here, because
  the split operator does not reconcile through a DSC context
- there is no OIDC customization or deployment-selector migration step in this
  module
- the reconciler owns additional resources such as
  `PodDisruptionBudget` and `ValidatingWebhookConfiguration`

## Configuration

The operator loads configuration from defaults, then a mounted ConfigMap, then
`ODH_MODULE_OPERATOR_*` environment variable overrides.

Common keys:

| Key | Default | Description |
|---|---|---|
| `applications-namespace` | `opendatahub` | Namespace where managed OGX resources are deployed |
| `platformType` | `OpenDataHub` | Platform selector used to choose the ODH or RHOAI overlay |
| `platformVersion` | `""` | Platform version reported in `status.releases` |

The controller also supports `controller.*` keys for metrics, health probes,
leader election, zap logging, and pprof settings.

When deployed through Helm, `.Values.platform.type`, `.Values.platform.version`,
and `.Values.config` are written into the operator ConfigMap.

## Quick Start

```bash
# Fetch embedded OGX manifests and run local checks
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
