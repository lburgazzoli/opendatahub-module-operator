# opendatahub-trustyai-operator

Standalone module operator for the TrustyAI component.

## Status

**Scaffolding complete.** Teams owning this module must:

- [ ] Run `make get-manifests` and verify overlays (odh, rhoai, mcp-guardrails)
- [ ] Identify actual workload Deployment name and update test fixtures
- [ ] Run integration tests: `make test-integration`
- [ ] Run e2e tests after Helm deploy

## Notable differences from other modules

- **checkPreConditions**: requires both InferenceServices CRD (`inferenceservices.serving.kserve.io`)
  AND the Kserve module CRD (`kserves.components.platform.opendatahub.io`).
  Uses `odherrors.NewStopError` to halt reconciliation until both are present.
- **MCPGuardrailsMode**: `TrustyAISpec.MCPGuardrailsMode bool` selects the
  `/overlays/mcp-guardrails` overlay instead of the platform overlay.
- **createConfigMap**: injects `trustyai-dsc-config` ConfigMap with eval
  permission settings (`eval.lmeval.permitCodeExecution`, `eval.lmeval.permitOnline`).
- **Custom CRD predicate**: CRD watch reacts only to InferenceServices CRD
  create/delete events (not updates), matching the monolith behaviour.
- **12 image params** in imageParamMap.
- **ServiceMonitor** in Owns.

## Architecture

See `docs/index.md` in the root of this repository for the full split plan.
