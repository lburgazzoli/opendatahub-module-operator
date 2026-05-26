# Module Operator Split — Reference Index

## Goal

Split the monolithic opendatahub-operator into independent module operators,
one per component. Each module lives under `modules/$name/` in this repo. The
runnable example operator also lives under
`modules/opendatahub-mymodule-operator/`.

## How to Create a New Module

Use the skill: `.agents/skills/odh-component-to-module/SKILL.md`

It has a step-by-step checklist; detailed docs live in
`.agents/skills/odh-component-to-module/references/` (naming, controller
porting, CRD types, manifest scripts, testing, e2e workflow, verification
gates, adversarial review, troubleshooting).

**Default test cluster:** OpenShift (CRC, ROSA, dev). Use a connected cluster
or CRC via kubeconfig.

## Current Modules

Current split modules live under `modules/`:

- `opendatahub-mymodule-operator`
- `opendatahub-modelregistry-operator`
- `opendatahub-trustyai-operator`
- `opendatahub-trainer-operator`
- `opendatahub-mlflow-operator`
- `opendatahub-feast-operator`
- `opendatahub-ogx-operator`
- `opendatahub-ray-operator`
- `opendatahub-spark-operator`
- `opendatahub-datasciencepipelines-operator`
- `opendatahub-workbenches-operator`

All current modules follow the shared Helm and CRC deployment model:

- module-local `hack/scripts/push-crc-image.sh` and `resolve-image-ref.sh`
- Helm image input via `image.fullRef`
- deployment `imagePullPolicy: Always`
- shorter base object naming with `operator` instead of `controller-manager`
- operator namespace wiring through `ODH_MODULE_OPERATOR_NAMESPACE`
- e2e/integration namespace overrides through `OPERATOR_NAMESPACE` and
  `INTEGRATION_TEST_NAMESPACE`

## Source References

| What | Path |
|------|------|
| Example module operator | `modules/opendatahub-mymodule-operator/` |
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
- [testing-limitations.md](testing-limitations.md) — OpenShift testing assumptions and limits

## Skills

- `.agents/skills/odh-component-to-module/` — Monolith component → standalone module (runs on Sonnet)
- `.agents/skills/odh-module-scaffold/` — Development guide for the template module

## Complexity Classification

**Excluded**: trainingoperator, modelsasservice (maas), kueue (not a component)

**Simple** (do first): ray (DONE), sparkoperator (DONE - needs manifests), feastoperator, ogx, mlflowoperator, trustyai, trainer
**Medium**: datasciencepipelines, modelregistry, modelcontroller
**Complex** (needs tuning): kserve, dashboard, workbenches

## Key Lessons Learned (from ray module)

1. **Env prefix**: Use `ODH_MODULE_OPERATOR_` for ALL modules — same as the example module; no component name in env vars
2. **Manifest permissions**: OpenShift assigns arbitrary UIDs. Use `chmod -R a+rX` in builder stage, init container with emptyDir for writable copy
3. **Scheme registration**: If watching CRDs, add `apiextensionsv1.AddToScheme(scheme)` in operator.go
4. **Owns must match kustomize output**: Every resource kind in `kustomize build`
   output needs an `Owns()` or `OwnsGVK()` (except CRDs) — see
   `manifest-rbac-audit.md`. Without Owns, `ReaderFailOnMissingInformer` fails.
5. **Module-local GVKs**: Each split module keeps its own
   `pkg/resources/gvk/gvk.go`; controllers and `cmd/chartgen/` import that
   local package instead of importing upstream `pkg/cluster/gvk` directly.
6. **RBAC for deployed operand**: Module SA must hold ALL permissions operand
   ClusterRoles grant. Derive markers from kustomize build — see
   `manifest-rbac-audit.md` and step 5b in the component-to-module skill.
7. **Memory limits**: 128Mi is too low for kustomize rendering of large manifests. Use 512Mi
8. **No ConfigValues**: The `status.configValues` field was a template example — real modules don't need it
9. **Troubleshoot in-cluster**: Patch RBAC/memory/image directly via kubectl before rebuilding. Use `kubectl wait` not `sleep`
10. **get-manifests wipes component dir**: Script must `rm -rf config/manifests/$COMPONENT/`
   before copy so stale YAML does not break kustomize or Owns/RBAC audits
11. **Cleanup scripts**: Each module has `cleanup-*.sh`, Makefile `test-*-run`
   targets, and composite `test-integration` / `test-e2e` — see e2e-workflow.md
12. **Test gates**: E2e uses `test-e2e-run` after manual deploy; operator gate
    before subtests; integration waits for cache sync in TestMain
13. **CRC-first local deploys**: On CRC, prefer `make deploy-crc`. It pushes the
    current `IMG` to the OpenShift internal registry through the module-local
    `hack/scripts/push-crc-image.sh` helper and deploys with a cluster-reachable
    pullspec.
14. **Helm image model**: Split modules use Helm `image.fullRef` rather than
    repository/tag pairs. Chart helpers render the full image reference and
    deployments keep `imagePullPolicy: Always`.
15. **Short operator naming**: Use base resource name `operator` instead of
    `controller-manager`; let the module `namePrefix` produce the full resource
    names.
16. **Namespace wiring**: Set `ODH_MODULE_OPERATOR_NAMESPACE` from
    `metadata.namespace` in the operator Deployment. E2e and integration tests
    should read `OPERATOR_NAMESPACE` and `INTEGRATION_TEST_NAMESPACE` with sane
    defaults rather than hardcoding namespaces.
17. **Chart sync**: After manifest/manager/RBAC changes, run `make helm`. A
    final `make helm` pass across all modules should leave the repo clean.
18. **OpenShift only**: Module integration/e2e assume OpenShift APIs are available
