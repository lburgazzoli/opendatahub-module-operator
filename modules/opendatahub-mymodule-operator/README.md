# opendatahub-mymodule-operator

Runnable example ODH module operator used as the pilot for migrating module
controllers toward the shared framework in
`github.com/opendatahub-io/odh-platform-utilities/framework`.

## Framework Migration Status

The `mymodule` pilot already uses the shared framework directly for:

- reconciler
- deploy
- gc
- deployment status checks
- handlers
- predicates
- controller request/types
- generic resource helpers
- cluster GVK constants used by chart generation

The remaining `github.com/opendatahub-io/opendatahub-operator/v2`
dependency in this module is `pkg/manager`, which is still used by the
runtime manager wrapper and the integration test harness.

## Working On This Module

Run build, test, chart, and deployment commands from this directory:

```sh
make test
make helm
make test-e2e
```

The controller image pull policy is intentionally kept at `Always` for both
kustomize and Helm deployment paths.

For local OpenShift development, you can build and deploy through the
internal cluster registry with:

```sh
make deploy-openshift
```
