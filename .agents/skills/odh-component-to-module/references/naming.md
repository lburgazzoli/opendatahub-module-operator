# Naming Convention

Module name: `opendatahub-$COMPONENT-operator`. If `$COMPONENT` already ends
with `operator`, split at that boundary and re-join with a dash.

| Component | Module Name | Leader Election ID |
|-----------|-------------|-------------------|
| ray | opendatahub-ray-operator | opendatahub-ray-operator-lock |
| sparkoperator | opendatahub-spark-operator | opendatahub-spark-operator-lock |
| feastoperator | opendatahub-feast-operator | opendatahub-feast-operator-lock |
| mlflowoperator | opendatahub-mlflow-operator | opendatahub-mlflow-operator-lock |
| trustyai | opendatahub-trustyai-operator | opendatahub-trustyai-operator-lock |
| trainer | opendatahub-trainer-operator | opendatahub-trainer-operator-lock |
| ogx | opendatahub-ogx-operator | opendatahub-ogx-operator-lock |

## Env prefix

The env prefix is **`ODH_MODULE_OPERATOR_`** for ALL modules — same as the
root reference operator. It does NOT contain the component name:
- `ODH_MODULE_OPERATOR_CONFIGURATION_PATH`
- `ODH_MODULE_OPERATOR_MANIFESTS_PATH`
- `ODH_MODULE_OPERATOR_APPLICATIONS_NAMESPACE`
- `ODH_MODULE_OPERATOR_PLATFORM_TYPE`

## What uses the module name

The module name is used for: directory, go.mod module path, image name, Helm
release, Kubernetes namespace, Kind cluster name, ConfigMap name,
`app.kubernetes.io/name` label, and leader election ID.

The env prefix does NOT use the module name.

## Hack script names

Use **hyphens**, not underscores, under `hack/scripts/`:

| Script | Purpose |
|--------|---------|
| `get-manifests.sh` | Fetch component manifests |
| `fetch-external-crds.sh` | Kind-only external CRD generation |
| `cleanup-integration.sh` | Pre-integration test cleanup |
| `cleanup-e2e.sh` | Pre-e2e test cleanup |
| `kind-setup.sh` | Kind cluster + cert-manager |

Makefile targets may use hyphens too (`get-manifests`, `fetch-external-crds`,
`container-prep`, `build-bin`).

**Container build:** `container-prep` (host) → `container-build` (image tag;
only `build-bin` runs inside the Containerfile).
