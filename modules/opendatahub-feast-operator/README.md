# opendatahub-feast-operator

Standalone module operator for the cluster-scoped singleton
`FeastOperator` component
(`components.platform.opendatahub.io/v1alpha1`). It renders and applies the
bundled Feast manifests, injects runtime OIDC configuration into the render
pipeline, and reports standard readiness and release status.

## What It Does

- Reconciles the singleton `FeastOperator` instance named
  `default-feastoperator`
- Selects the embedded manifest overlay based on `platformType`
  (`OpenDataHub` uses `overlays/odh`; `SelfManagedRhoai` and `ManagedRhoai`
  use `overlays/rhoai`)
- Injects image parameters and `spec.oidc.issuerURL` into the embedded
  manifests before render/apply
- Renders the embedded `openshift-config-grants` template alongside the
  kustomized manifests
- Deletes the stale `feast-operator-controller-manager` Deployment when a
  selector migration is required
- Deploys managed resources into the configured applications namespace and
  garbage collects owned resources
- Reports `Ready`, `ProvisioningSucceeded`, and `status.releases`

## OIDC and Selector Migration

The main Feast-specific API input is `spec.oidc.issuerURL`.

- When `spec.oidc` is unset, the module renders an empty `OIDC_ISSUER_URL`
  value into `params.env`
- When `spec.oidc.issuerURL` is set, the module validates that it is an
  absolute HTTPS URL and writes the normalized value into the rendered
  manifests

The module also includes a one-time selector migration for the
`feast-operator-controller-manager` Deployment. If the existing Deployment
selector is missing `app.kubernetes.io/name=feast-operator`, the operator
deletes it so reconciliation can recreate it with the correct immutable
selector.

## Configuration

The operator loads configuration from defaults, then a mounted ConfigMap, then
`ODH_MODULE_OPERATOR_*` environment variable overrides.

Common keys:

| Key | Default | Description |
|---|---|---|
| `applications-namespace` | `opendatahub` | Namespace where managed Feast resources are deployed |
| `platformType` | `OpenDataHub` | Platform selector used to choose the ODH or RHOAI overlay |
| `platformVersion` | `""` | Platform version reported in `status.releases` |

The controller also supports `controller.*` keys for metrics, health probes,
leader election, zap logging, and pprof settings.

When deployed through Helm, `.Values.platform.type`, `.Values.platform.version`,
and `.Values.config` are written into the operator ConfigMap.

## Quick Start

```bash
# Fetch embedded Feast manifests and run local checks
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
`internal/controller/feastoperator/feastoperator_upgrade.go` does not yet
contain version-gated migrations.

## Architecture

See `docs/index.md` in the root of this repository for the broader split plan
and design context.
