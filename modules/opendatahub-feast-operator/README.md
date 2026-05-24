# opendatahub-feast-operator

Standalone module operator for the FeastOperator component, split from the
monolithic opendatahub-operator.

## Status

**Scaffolding complete.** Teams owning this module must:

- [ ] Run `make get-manifests` and verify the overlay structure
  under `config/manifests/feastoperator/overlays/{odh,rhoai}/`
- [ ] Identify actual workload Deployment name from manifests and update
  test fixtures in `test/integration/` and `test/e2e/`
- [ ] Run integration tests: `make test-integration`
- [ ] Run e2e tests after Helm deploy

## Notable differences from ray/sparkoperator

- **OIDC spec**: `FeastOperatorSpec.OIDC *common.GatewayOIDCSpec` — set by
  whoever creates the CR. Standalone users set it directly on the CR.
- **setKustomizedParams**: writes `OIDC_ISSUER_URL` to params.env before
  kustomize renders manifests. Empty string when OIDC not in use.
- **migrateDeploymentSelector**: deletes `feast-operator-controller-manager`
  Deployment when its selector is stale (selector immutability migration).
- **Two image params**: `RELATED_IMAGE_FEAST_OPERATOR` and
  `RELATED_IMAGE_FEATURE_SERVER`.

## Architecture

See `docs/index.md` in the root of this repository for the full split plan.
