# opendatahub-modelregistry-operator

Standalone module operator for the cluster-scoped singleton
`ModelRegistry` component
(`components.platform.opendatahub.io/v1alpha1`). It renders and applies the
bundled Model Registry manifests, injects runtime routing values into
`params.env`, creates the registries namespace dependency, and reports standard
readiness and release status.

## What It Does

- Reconciles the singleton `ModelRegistry` instance named
  `default-modelregistry`
- Uses embedded manifests from `manifests/modelregistry/overlays/odh`
- Renders the additional manifest set from `overlays/odh/extras`
- Injects image parameters and static defaults into `params.env` at startup
- Computes `GATEWAY_DOMAIN`, `GATEWAY_NAME`, `GATEWAY_NAMESPACE`,
  `HTTPROUTE_NAMESPACE`, and `REGISTRIES_NAMESPACE` from the CR spec and
  operator config before kustomize renders manifests
- Creates the registries namespace dependency before deploying the workload
- Renders the embedded `openshift-config-grants` template alongside the
  kustomized manifests
- Reports `Ready`, `ProvisioningSucceeded`, `status.releases`, and
  `status.registriesNamespace`

## Routing and Registries Namespace

The main Model Registry-specific runtime behavior is in
`customizeManifests()` and `configureDependencies()`.

- `customizeManifests()` requires `spec.gateway.domain`; reconcile fails if
  that field is empty
- the gateway name and namespace are currently fixed to
  `data-science-gateway` and `openshift-ingress`
- `HTTPROUTE_NAMESPACE` comes from the configured applications namespace
- `REGISTRIES_NAMESPACE` comes from `spec.registriesNamespace` and is copied to
  status
- `configureDependencies()` adds that namespace as a managed dependency before
  deploy

## Configuration

The operator loads configuration from defaults, then a mounted ConfigMap, then
`ODH_MODULE_OPERATOR_*` environment variable overrides.

Common keys:

| Key | Default | Description |
|---|---|---|
| `applications-namespace` | `opendatahub` | Namespace where managed Model Registry resources are deployed |
| `platformType` | `OpenDataHub` | Platform identifier reported in status and Helm config |
| `platformVersion` | `""` | Platform version reported in `status.releases` |

The controller also supports `controller.*` keys for metrics, health probes,
leader election, zap logging, and pprof settings.

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
export GATEWAY_DOMAIN="$(kubectl get ingresses.config.openshift.io cluster -o jsonpath='{.spec.domain}')"
make test-e2e-run
```

## Test Flows

```bash
# Unit tests
make test

# Integration tests (prepare cluster, then run)
make test-integration

# E2E tests with deploy included
export GATEWAY_DOMAIN="$(kubectl get ingresses.config.openshift.io cluster -o jsonpath='{.spec.domain}')"
make test-e2e IMG="$IMG"

# E2E tests against an already deployed operator
make test-e2e-run
```

## API Defaults

- `spec.gateway.domain` is required
- `spec.registriesNamespace` defaults to `odh-model-registries` in ODH builds
- `spec.registriesNamespace` defaults to `rhoai-model-registries` in RHOAI
  builds
- `spec.registriesNamespace` is immutable once set

## Architecture

See `docs/index.md` in the root of this repository for the broader split plan
and design context.
