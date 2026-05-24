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

The env prefix is **`ODH_OPERATOR_`** for ALL modules — it does NOT contain
the component name. This keeps env vars consistent across modules:
- `ODH_OPERATOR_CONFIGURATION_PATH`
- `ODH_OPERATOR_MANIFESTS_PATH`
- `ODH_OPERATOR_APPLICATIONS_NAMESPACE`
- `ODH_OPERATOR_PLATFORM_TYPE`

## What uses the module name

The module name is used for: directory, go.mod module path, image name, Helm
release, Kubernetes namespace, Kind cluster name, ConfigMap name,
`app.kubernetes.io/name` label, and leader election ID.

The env prefix does NOT use the module name.
