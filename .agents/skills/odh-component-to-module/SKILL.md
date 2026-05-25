---
name: odh-component-to-module
description: >
  Scaffold a standalone module operator from an opendatahub-operator
  component on OpenShift. Use when creating modules/$name/ from the monolith
  source. Covers rename gates, cleanup, verification, and e2e workflow.
model: sonnet
user-invocable: true
allowed-tools:
  - Read
  - Glob
  - Grep
  - Bash
  - Write
  - Edit
  - Agent
---

# Scaffold Module Operator from Monolith

Split a monolith `ComponentHandler` into a standalone module under
`modules/$MODULE_NAME/`. Follow the checklist in order; read reference docs
only when a step points to them.

**Test cluster:** OpenShift (CRC, ROSA, dev). See [testing.md](references/testing.md).
When validating a module, run integration/e2e/deploy Make targets from
`modules/$MODULE_NAME/` (or set your tool `working_directory` there). Running
the same target names from the repo root acts on `opendatahub-module-operator`
instead of the module under test.

## Inputs

| Variable | Example | Derive from |
|----------|---------|-------------|
| `$COMPONENT` | `sparkoperator` | Monolith directory name |
| `$MODULE_NAME` | `opendatahub-spark-operator` | [references/naming.md](references/naming.md) |
| `$KIND` | `SparkOperator` | CRD Kind |

## Reference docs

| Doc | Use when |
|-----|----------|
| [naming.md](references/naming.md) | Deriving `$MODULE_NAME`, env prefix rules |
| [renaming.md](references/renaming.md) | After copying ray template — substitutions |
| [extraction-checklist.md](references/extraction-checklist.md) | Step 1 — recording monolith findings |
| [controller-rules.md](references/controller-rules.md) | Step 3 — pipeline, Watches, env prefix |
| [crd-types.md](references/crd-types.md) | Step 4 — API types |
| [manifest-script.md](references/manifest-script.md) | Step 5 — `get-manifests.sh` |
| [manifest-rbac-audit.md](references/manifest-rbac-audit.md) | Step 5b — Owns + operand RBAC from kustomize |
| [external-crds.md](references/external-crds.md) | Step 6 — OpenShift types (OwnsGVK only) |
| [testing.md](references/testing.md) | Step 7 — unit, integration, e2e, timeouts |
| [e2e-workflow.md](references/e2e-workflow.md) | Step 10 — IMG, helm, deploy, test targets |
| [verification-gates.md](references/verification-gates.md) | Steps 2b, 2c, 8 — grep gates |
| [adversarial-review.md](references/adversarial-review.md) | Steps 9, 9b — subagent prompts |
| [troubleshooting.md](references/troubleshooting.md) | In-cluster failures |

## Checklist

### 1. Read monolith source

Read ALL files in:

```
/home/luca/work/dev/openshift-ai/opendatahub-operator/internal/controller/components/$COMPONENT/
/home/luca/work/dev/openshift-ai/opendatahub-operator/api/components/v1alpha1/${COMPONENT}_types.go
```

Record findings per [extraction-checklist.md](references/extraction-checklist.md).

When analyzing dependencies and preconditions, do **not** default to checking
operator-installation state (`OperatorExists`, Subscription health, CSV
presence, etc.). Prefer concrete API/resource availability:

- the operator CRD and required singleton/operator CR instance, when the
  component depends on another controller-managed API
- the operand CRD types that prove the dependent API surface is installed

Example: for JobSet-backed components, gate on the JobSet operator CRD, the
`JobSetOperator` CR instance, and the JobSet workload CRD rather than on
"is the operator installed?" metadata.

### 2. Copy ray module and rename

```bash
cp -r modules/opendatahub-ray-operator/ modules/$MODULE_NAME/
```

Apply all renames per [renaming.md](references/renaming.md).

### 2b–2c. Verification gates

Run env prefix and rename completeness gates per
[verification-gates.md](references/verification-gates.md).

### 3. Port controller (pipeline, actions, Module)

Per [controller-rules.md](references/controller-rules.md). Key files:

- `${name}_controller.go` — wiring (Owns draft, Watches, pipeline, RBAC draft)
- `${name}.go` — Module struct, NewModule, initialize, reportStatus
- `${name}_actions.go` — custom actions (if any)
- `${name}_upgrade.go` — upgrade placeholder
- `${name}_webhook.go` — webhooks (if any)

Copy **Watches** and action pipeline from the monolith. Insert
`upgradeIfNeeded` **immediately** after `initialize` with **nothing in
between**. **Owns** and RBAC markers are a **draft** from the monolith here —
**finalize in step 5b** against kustomize output (module must own every
deployed resource except CRDs, and hold RBAC for everything the deployed
operand handles). Every module operator also carries baseline CRD RBAC, and
every module that exposes `/metrics` keeps the protected-metrics RBAC markers
(`tokenreviews`, `subjectaccessreviews`, and `urls=/metrics`) even when the
monolith did not make that baseline explicit.

If the monolith uses operator-level dependency checks, port the **intent** but
prefer resource-based gates in the module:

- check for dependent CRDs and required CR instances first
- use operand CRD presence as the signal that the downstream API surface exists
- avoid gating on operator-installation metadata unless there is no concrete API
  or instance to check instead

Before wiring `OwnsGVK`, `WatchesGVK`, sanity checks, or chart generation,
create `pkg/resources/gvk/gvk.go` in the module and make it the module-local
source of truth for all GVK constants. Module code must import that local
package instead of importing
`github.com/opendatahub-io/opendatahub-operator/v2/pkg/cluster/gvk`
directly.

Before wiring `OwnsGVK`, `WatchesGVK`, sanity checks, or chart generation,
create `pkg/resources/gvk/gvk.go` in the module and make it the module-local
source of truth for all GVK constants. Module code must import that local
package instead of importing
`github.com/opendatahub-io/opendatahub-operator/v2/pkg/cluster/gvk`
directly.

### 4. Port CRD types

Per [crd-types.md](references/crd-types.md). The module must expose **its own**
CRD Kind / names / schema, not the ray/template CRD copied during scaffolding.

### 5. Create manifest script and fetch

Per [manifest-script.md](references/manifest-script.md). Then:

```bash
# ODH or RHOAI — selects git repo/commit at fetch time
export ODH_PLATFORM_TYPE=OpenDataHub   # or SelfManagedRhoai / ManagedRhoai
make get-manifests
```

Do not stop here waiting for a human prompt just because the migration reached
the fetch step. Once `get-manifests.sh` is in place, run `make get-manifests`
yourself and continue to the audit/fix loop.

Script must `rm -rf config/manifests/$COMPONENT/` before copy.

### 5b. Manifest RBAC audit (mandatory)

After `make get-manifests`, run the full audit in
[manifest-rbac-audit.md](references/manifest-rbac-audit.md):

- Resolve `config/manifests/${ContextDir}/${SourcePath}` from extraction
- `kustomize build` must succeed
- If the fetched manifests have multiple overlays, run the RBAC/permissions
  audit against **every overlay** (for example `overlays/odh` and
  `overlays/rhoai`), not just the configmap default
- Add `Owns` / `OwnsGVK` for every Kind in output (except CRD / Namespace)
- Keep the baseline CRD marker on every module operator:
  `// +kubebuilder:rbac:groups=apiextensions.k8s.io,resources=customresourcedefinitions,verbs=get;list;watch;create;update;patch;delete`
- If the module exposes `/metrics`, keep the protected-metrics markers:
  `// +kubebuilder:rbac:groups=authentication.k8s.io,resources=tokenreviews,verbs=create`,
  `// +kubebuilder:rbac:groups=authorization.k8s.io,resources=subjectaccessreviews,verbs=create`,
  and `// +kubebuilder:rbac:urls=/metrics,verbs=get`
- Add operator `+kubebuilder:rbac` for every rule in deployed operand
  ClusterRoles (and Roles)
- Run `make manifests generate`

### 6. OpenShift external types (if any)

If the monolith `Owns` SCC, Route, or other OpenShift types: add `OwnsGVK` +
RBAC in the controller. **No CRD fetch on OpenShift** — see
[external-crds.md](references/external-crds.md).

### 7. Write tests and cleanup wiring

Per [testing.md](references/testing.md):

- Unit, integration, e2e tests (`testing.T` + Gomega)
- `hack/scripts/cleanup-integration.sh` and `cleanup-e2e.sh` must delete the
  module CRs and wait for them to disappear before deleting the module CRD
- Integration CRDs must be installed from `make`, not from Go test code; the
  tests should fail fast if the expected module CRD is missing
- Makefile: `prepare-integration`, `test-integration-run`, `test-e2e-run`, and
  composite targets (`test-integration` → prepare + run; `test-e2e` → cleanup
  + deploy + run)
- Execute those integration/e2e targets from `modules/$MODULE_NAME/`, not the
  repo root

### 8. Build and verify

Run all gates in [verification-gates.md](references/verification-gates.md), then
`make test` and `make lint`.

### 9–9b. Adversarial reviews

Spawn **both** subagents per [adversarial-review.md](references/adversarial-review.md).

### 10. Fix findings and cluster verify

Address all findings from steps 9 and 9b. Run **one command at a time** from
`modules/$MODULE_NAME/` per [e2e-workflow.md](references/e2e-workflow.md). If
you are using a tool that supports `working_directory`, set it to the module
path before running any integration/e2e/deploy target. This is mandatory: the
repo root exposes the same target names for `opendatahub-module-operator`, so
running `container-build`, `helm`, or `deploy-helm` from the wrong directory
can build and install the root chart instead of the module chart:

```bash
make test
make lint

make manifests generate
make cleanup-integration
make test-integration-run

export IMG="ttl.sh/${MODULE_NAME}-$(uuidgen | tr '[:upper:]' '[:lower:]'):1h"
echo "IMG=${IMG}"
make container-prep         # host: manifests generate get-manifests
make container-build IMG="${IMG}"        # binary compiled inside image
make container-push IMG="${IMG}"
make helm
make cleanup-e2e
make deploy-helm IMG="${IMG}"
make test-e2e-run
```

For the **first** integration/e2e verification pass, prefer short timeouts to
surface setup bugs quickly. If deploy/test output stalls, stop waiting and check
the operator logs instead of letting a long timeout burn down.

Keep `IMG` in shell memory. Do not write it to a temp file and later run
`make ... IMG="$(cat /tmp/img)"`; that adds unnecessary indirection and can
trigger extra command authorization in agent runs.

Do not chain targets. Do not run `make test-e2e` after manual deploy — use
`test-e2e-run` to avoid re-running cleanup and deploy.

## Troubleshooting

See [troubleshooting.md](references/troubleshooting.md).
