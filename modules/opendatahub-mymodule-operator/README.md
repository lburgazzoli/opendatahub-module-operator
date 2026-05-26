# opendatahub-mymodule-operator

Runnable example ODH module operator used as the current pilot for migrating
module controllers toward
`github.com/opendatahub-io/operator-actions-framework`.

## Framework Migration Status

The `mymodule` pilot already uses the framework directly for:

- reconciler
- deploy
- gc
- deployment status checks
- handlers
- predicates
- controller request/types
- generic resource helpers
- cluster GVK constants used by chart generation

`pkg/webhook` from `opendatahub-operator` was previously used only for
`webhookutils.NewWebhookLogConstructor(...)` and has already been replaced with
a local helper in this module.

The following `github.com/opendatahub-io/opendatahub-operator/v2` libraries are
still in use and still need replacement, local extraction, or an accepted
wrapper strategy:

- `api/common`
- `pkg/controller/precondition`
- `pkg/controller/actions/status/releases`
- `pkg/controller/actions/render/kustomize`
- `pkg/cluster`
- `pkg/manager`

## Working On This Module

Run build, test, chart, and deployment commands from this directory:

```sh
make test
make helm
make test-e2e
```

The controller image pull policy is intentionally kept at `Always` for both
kustomize and Helm deployment paths.

For local CRC/OpenShift development, you can build and deploy through the
internal cluster registry with:

```sh
make deploy-crc
```
