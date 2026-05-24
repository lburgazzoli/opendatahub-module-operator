# opendatahub-ogx-operator

Standalone module operator for the OGX component, split from the monolithic
opendatahub-operator.

## Status

**Scaffolding complete.** Teams owning this module must:

- [ ] Run `make get-manifests` and verify the overlay structure
- [ ] Identify actual workload Deployment name from manifests and update
  test fixtures
- [ ] Run integration tests: `make test-integration`
- [ ] Run e2e tests after Helm deploy

## Notable differences from ray/spark/feast

- **PodDisruptionBudget + ValidatingWebhookConfiguration** in Owns
- **Two image params**: `RELATED_IMAGE_ODH_OGX_OPERATOR` and
  `RELATED_IMAGE_RH_DISTRIBUTION`
- **checkPreConditions**: no-op in the module (monolith checked that
  LlamaStackOperator was not Managed via DSC — no DSC in module context)
- **No OIDC, no migrateDeploymentSelector**

## Architecture

See `docs/index.md` in the root of this repository for the full split plan.
