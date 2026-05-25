# opendatahub-mlflow-operator

Standalone module operator for the MLflowOperator component, split from the
monolithic opendatahub-operator.

## Status

**Scaffolding complete.** Teams owning this module must:

- [ ] Run `make get-manifests` and verify the overlay structure
- [ ] Validate gateway domain error handling (see TODO in `mlflowoperator_actions.go`)
- [ ] Identify actual workload Deployment name from manifests and update test fixtures
- [ ] Run integration tests: `make test-integration`
- [ ] Run e2e tests after Helm deploy

## Notable differences from other modules

- **ConsoleLink + ServiceMonitor** in Owns
- **MLflow GVK** dynamic ownership (`OwnsGVK(gvk.MLflow, reconciler.Dynamic())`)
  via the module-local `pkg/resources/gvk` package
- **HTTPRoute + GatewayConfig** watches (gateway-api dependency)
- **setKustomizedParams**: reads `GatewayConfig.Status.Domain` live at reconcile time
  to compute `mlflow-url` and `section-title` for params.env. Implemented with
  unstructured client to avoid importing OpenShift service API types.
- **Three image params**: MLFLOW_IMAGE, MLFLOW_OPERATOR_IMAGE, KUBE_AUTH_PROXY_IMAGE
- **params.env written to base/**, not the overlay (matches monolith's paramsPath)
- **kustomize.NewAction()** has no labels (monolith behavior) — labels applied via
  `deploy.WithLabel(...)` on the deploy action instead

## Architecture

See `docs/index.md` in the root of this repository for the full split plan.

## Testing namespace

Integration and e2e tests must use `opendatahub` as the applications namespace.
The mlflow operator workload is configured to watch and list resources in that
namespace, so using a different namespace (e.g. `integration-test`) will cause
the deployed mlflow-operator pod to fail with RBAC watch errors.

The `--namespace` flag passed to the mlflow-operator binary is hardcoded in
the overlay patch files (`overlays/odh/manager_patch.yaml` → `opendatahub`,
`overlays/rhoai/manager_patch.yaml` → `redhat-ods-applications`). Integration
tests must therefore set `ApplicationsNamespace=opendatahub` (ODH) or
`redhat-ods-applications` (RHOAI) to match. Tests using a custom namespace
like `integration-test` will see RBAC watch errors from the mlflow-operator
pod because it hardcodes the target namespace in its startup args.
