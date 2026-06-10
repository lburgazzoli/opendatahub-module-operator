# Matcher Migration Rollout Plan

**Date:** 2026-06-10

**Goal:** Move all Kubernetes-focused test usage in this repo to the current `gomega-matchers` style from `/home/luca/work/dev/gomega-matchers`, starting with `orchestrator` and then completing the migration module-by-module.

**What changed upstream:** the relevant upstream change is the recent k8s API unification (`443c098`, followed by nil-safety fix `6a5ad9c`). The target style is package-level helpers that take `client.Client` directly.

## Key Callsite Migrations

- `k8sm.New(...)` or `k8sm.NewResources(...)` -> remove wrapper state and use the `client.Client` directly
- `Eventually(k.Get(obj)).WithContext(ctx)...` -> `Eventually(ctx, k8sm.Get(cli, obj))...`
- `suite.K.NotFound(obj)` / `k.Gone(obj)` -> `k8sm.NotFound(cli, obj)` or `k8sm.Absent(cli, obj)` depending on intent
- `suite.K.HasEvent(...)` -> `k8sm.Events(cli, k8sm.ForObject(...))` plus `gstruct` / element matchers
- keep `jq.Match`, `jq.Extract`, `k8sm.Data()`, `k8sm.ListItems()`, `k8sm.IsEmptyList()` unless a clearer package helper exists
- opportunistically normalize formatted jq assertions to `jq.Matchf(...)`

Representative files to update first:

- `orchestrator/test/integration/support/runtime.go`
- `orchestrator/test/integration/support/harness.go`
- `orchestrator/test/integration/foundation/foundation_test.go`
- `orchestrator/test/integration/runlevel/runlevel_test.go`
- `orchestrator/test/integration/gates/gates_test.go`

Representative old vs target style:

```go
// current
suite.K.Get(obj)
Eventually(k.Get(obj)).WithContext(ctx).Should(jq.Match(`...`))
suite.K.NotFound(obj)

// target
k8sm.Get(suite.Client, obj)
Eventually(ctx, k8sm.Get(k8sClient, obj)).Should(jq.Match(`...`))
k8sm.NotFound(suite.Client, obj)
```

## Scope Map

- `orchestrator` is partially migrated already: it uses `k8sm.NewResources(...)` in the integration harness and many `suite.K.*` callsites.
- `modules/opendatahub-workbenches-operator` is on a newer matcher version and has extra manual work around upgrade tests and event assertions.
- The other modules mostly still use the older `k8sm.New(...)` + `k.Get(...)` pattern and should be largely mechanical once the first conversions are proven.
- `modules/opendatahub-datasciencepipelines-operator` also has one legacy jq import path that should be folded into the same migration.

## Rollout Order

1. **Pin the target matcher revision in execution**
   - Use the current `gomega-matchers` HEAD style as the target, based on `443c098` plus follow-up `6a5ad9c`.
   - During implementation, update each affected `go.mod` to the same chosen pseudo-version so the repo converges on one matcher API.

2. **Migrate `orchestrator` first**
   - Replace harness-owned matcher wrapper state in `orchestrator/test/integration/support/runtime.go` and `orchestrator/test/integration/support/harness.go`.
   - Convert test callsites in:
     - `orchestrator/test/integration/foundation/foundation_test.go`
     - `orchestrator/test/integration/runlevel/runlevel_test.go`
     - `orchestrator/test/integration/runlevel/runlevel_support_test.go`
     - `orchestrator/test/integration/gates/gates_test.go`
     - `orchestrator/test/integration/gates/gates_support_test.go`
   - Keep repo-local matcher helpers in `orchestrator/test/support/matchers.go` unless they can be simplified without changing test intent.
   - Rewrite `HasEvent` assertions to `k8sm.Events(...)`-based polling rather than recreating a wrapper.

3. **Migrate the mechanical module template next**
   - Use one low-risk module as the codemod template, preferably `modules/opendatahub-spark-operator`.
   - Apply the same pattern across:
     - `modules/opendatahub-feast-operator`
     - `modules/opendatahub-ray-operator`
     - `modules/opendatahub-ogx-operator`
     - `modules/opendatahub-mlflow-operator`
   - In each module, start from:
     - `integration_test.go`
     - `integration_foundation_test.go`
     - `e2e_test.go`
     - `e2e_foundation_test.go`

4. **Migrate the medium-complexity modules**
   - `modules/opendatahub-modelregistry-operator`
   - `modules/opendatahub-trustyai-operator`
   - `modules/opendatahub-trainer-operator`

5. **Finish with the manual/heavy modules**
   - `modules/opendatahub-datasciencepipelines-operator`
     - convert matcher usage in its integration/e2e tests
     - replace the legacy jq import in its unit/action tests with `github.com/lburgazzoli/gomega-matchers/pkg/matchers/jq`
   - `modules/opendatahub-mymodule-operator`
   - `modules/opendatahub-workbenches-operator`
     - leave for last because its upgrade tests use dual client/matcher variables and custom event assertions that need manual rework

## Verification Strategy

- Validate `orchestrator` in isolation first with `make test-integration` inside `orchestrator/`.
- Then verify each migrated module before moving on.
- Because the environment is connected to an OpenShift cluster, prefer module-local integration/e2e verification during implementation rather than a repo-wide sweep after every module.
- Keep sequential/shared-cluster constraints from the repo `AGENTS.md` in mind when scheduling module verification.

## Risks To Watch

- `NotFound` vs `Absent`: use `NotFound` when the test really expects a 404 from an existing CRD, and `Absent` when the resource type may not exist or the CRD itself may be gone.
- Event assertions: replacing `HasEvent` requires preserving namespace/object filtering so tests do not accidentally match unrelated cluster events.
- Over-converting jq assertions: do not rewrite broad jq-based condition checks unless the new helper makes the test strictly clearer.
- Mixed matcher versions across modules: avoid partial dependency drift once implementation starts; converge all touched modules onto the same matcher revision.
