# Module Operator Split — Reference Index

## Goal

Split the monolithic opendatahub-operator into independent module operators,
one per component. Each module lives under `modules/$name/` in this repo and
follows the patterns established by the existing `mymodule` reference
implementation.

## How to Create a New Module

Use the skill: `.agents/skills/odh-component-to-module/SKILL.md`

It has a step-by-step checklist with reference docs for naming, controller
porting, CRD types, manifest scripts, testing, external CRDs, adversarial
review, and troubleshooting.

## Completed Modules

| Module | Directory | Status |
|--------|-----------|--------|
| ray | `modules/opendatahub-ray-operator/` | Unit + integration + e2e pass |
| sparkoperator | `modules/opendatahub-spark-operator/` | Unit pass; integration + e2e need manifests |

## Source References

| What | Path |
|------|------|
| Reference module operator | this repo (`internal/controller/mymodule/`) |
| Completed ray module | `modules/opendatahub-ray-operator/` |
| Component controllers | `/home/luca/work/dev/openshift-ai/opendatahub-operator/internal/controller/components/` |
| Component API types | `/home/luca/work/dev/openshift-ai/opendatahub-operator/api/components/v1alpha1/` |
| Upgrade logic | `/home/luca/work/dev/openshift-ai/opendatahub-operator/pkg/upgrade/` |
| Webhook logic | `/home/luca/work/dev/openshift-ai/opendatahub-operator/internal/webhook/` |
| Manifest fetcher | `/home/luca/work/dev/openshift-ai/opendatahub-operator/get_all_manifests.sh` |

## Documents

- [components.md](components.md) — All components with classification, dependencies, and key details
- [plan.md](plan.md) — Implementation plan and task breakdown
- [manifest-sources.md](manifest-sources.md) — Per-component manifest repo/ref/path
- [upgrade-logic.md](upgrade-logic.md) — Upgrade functions and which components they affect
- [webhook-logic.md](webhook-logic.md) — Per-component webhook mapping
- [testing-limitations.md](testing-limitations.md) — Known testing limitations (SCC on Kind, etc.)

## Skills

- `.agents/skills/odh-component-to-module/` — Monolith component → standalone module (runs on Sonnet)
- `.agents/skills/odh-module-scaffold/` — Development guide for the template module

## Complexity Classification

**Excluded**: trainingoperator, modelsasservice (maas), kueue (not a component)

**Simple** (do first): ray (DONE), sparkoperator (DONE - needs manifests), feastoperator, ogx, mlflowoperator, trustyai, trainer
**Medium**: datasciencepipelines, modelregistry, modelcontroller
**Complex** (needs tuning): kserve, dashboard, workbenches

## Key Lessons Learned (from ray module)

1. **Env prefix**: Use `ODH_OPERATOR_` for ALL modules — no component name in env vars
2. **Manifest permissions**: OpenShift assigns arbitrary UIDs. Use `chmod -R a+rX` in builder stage, init container with emptyDir for writable copy
3. **Scheme registration**: If watching CRDs, add `apiextensionsv1.AddToScheme(scheme)` in operator.go
4. **Owns must match kustomize output**: Every resource kind in `kustomize build` output needs an `Owns()` or `OwnsGVK()` — otherwise `ReaderFailOnMissingInformer` fails with "is not cached"
5. **RBAC escalation**: When deploying ClusterRoles, the module SA must hold ALL permissions those ClusterRoles grant. Derive RBAC markers from `kustomize build | yq 'select(.kind == "ClusterRole") | .rules'`
6. **Memory limits**: 128Mi is too low for kustomize rendering of large manifests. Use 512Mi
7. **No ConfigValues**: The `status.configValues` field was a template example — real modules don't need it
8. **Troubleshoot in-cluster**: Patch RBAC/memory/image directly via kubectl before rebuilding. Use `kubectl wait` not `sleep`
9. **Cleanup scripts**: Each module has `hack/scripts/cleanup-integration.sh` and `cleanup-e2e.sh` plus Makefile targets
10. **Test gates**: E2e tests check operator deployment is ready BEFORE registering subtests. Integration tests wait for cache sync in TestMain
