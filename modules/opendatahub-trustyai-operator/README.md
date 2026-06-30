# opendatahub-trustyai-operator

Standalone module operator for the cluster-scoped singleton `TrustyAI`
component (`components.platform.opendatahub.io/v1alpha1`). It renders and
applies the bundled TrustyAI manifests, injects image parameters into the
embedded kustomize overlays, creates the TrustyAI DSC config map from the CR
spec, gates reconciliation on KServe prerequisites, and reports standard
release status.

## What It Does

- Reconciles the singleton `TrustyAI` instance named `default-trustyai`
- Selects the embedded manifest overlay based on `platformType`
  (`OpenDataHub` uses `overlays/odh`; `SelfManagedRhoai` and `ManagedRhoai`
  use `overlays/rhoai`)
- Switches to `overlays/mcp-guardrails` when `spec.mcpGuardrailsMode` is set
- Injects 12 image substitutions into `params.env` before render/apply
- Renders the embedded `openshift-config-grants` template alongside the
  kustomized manifests
- Creates a `trustyai-dsc-config` ConfigMap that mirrors the CR's eval
  permission settings
- Uses explicit named `bind`/`escalate` permissions so the operator can manage
  privileged rendered `ClusterRole`s without broader cluster-wide RBAC
- Stops reconciliation with `DependenciesAvailable=False` until KServe
  prerequisites are present
- Reports `Ready`, `ProvisioningSucceeded`, and `status.releases`

## Module-Specific Notes

Compared with some of the other split modules:

- readiness is gated on three KServe-related prerequisites: the KServe module
  CRD, the KServe singleton CR, and the InferenceServices CRD
- `spec.mcpGuardrailsMode` swaps the normal platform overlay for the dedicated
  MCP guardrails overlay
- tests can create lightweight KServe stubs so the module can be exercised on a
  cluster without the full dependency stack
- the controller also owns the generated `trustyai-dsc-config` ConfigMap in
  addition to the rendered workload resources

## Configuration

The operator loads configuration from defaults, then a mounted ConfigMap, then
`ODH_MODULE_OPERATOR_*` environment variable overrides.

Common keys:

| Key | Default | Description |
|---|---|---|
| `applications-namespace` | `opendatahub` | Namespace where managed TrustyAI resources are deployed |
| `platformType` | `OpenDataHub` | Platform selector used to choose the ODH or RHOAI overlay |
| `platformVersion` | `""` | Platform version reported in `status.releases` |

The controller also supports `controller.*` keys for metrics, health probes,
leader election, zap logging, and pprof settings.

When deployed through Helm, `.Values.platform.type`, `.Values.platform.version`,
and `.Values.config` are written into the operator ConfigMap.

## Quick Start

```bash
# Fetch embedded TrustyAI manifests and run local checks
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
