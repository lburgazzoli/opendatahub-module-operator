# Testing Guide

## OpenShift assumptions

Integration and e2e tests are run against **OpenShift** (CRC, ROSA, shared dev
cluster) — not vanilla Kind. Assume:

- **Kubeconfig** points at OpenShift (`oc login` or existing context).
- **OpenShift APIs** (SCC, Route, etc.) exist on the cluster — no need to
  install synthetic CRDs before tests (see [external-crds.md](external-crds.md)).
- **Platform overlay** in tests matches the cluster: use `rhoai` or `odh` from
  `config/manager/configmap.yaml`, not a Kind-specific stub.
- **SCC workloads** reconcile normally — ray and similar components deploy SCC
  resources; this fails on Kind but works on OpenShift.
- **Image pull**: push to a registry the cluster can reach (`ttl.sh`, internal
  registry, or `imagePullSecrets` on the operator namespace if required).

Use `oc` or `kubectl` interchangeably in cleanup scripts; prefer whatever is
in the user's PATH.

For Kind-only local dev (optional, not the default path), see
`docs/testing-limitations.md`.

## Pre-test cleanup (required)

Integration and e2e tests assume a **clean cluster**. Leftover module CRs,
workload objects, cluster RBAC, or a stale CRD from a failed run cause flaky
or misleading failures. Every module must ship cleanup and run it **before**
`go test` for integration/e2e.

### Makefile targets

Add two targets (copy from `modules/opendatahub-ray-operator/Makefile` and
rename for `$COMPONENT`):

```makefile
.PHONY: cleanup-integration
cleanup-integration: ## Clean up integration test resources from the cluster.
	./hack/scripts/cleanup-integration.sh

.PHONY: cleanup-e2e
cleanup-e2e: ## Clean up e2e test resources and uninstall operator from the cluster.
	./hack/scripts/cleanup-e2e.sh
```

Wire cleanup as a **dependency** of the composite test targets:

```makefile
.PHONY: test-integration-run
test-integration-run: ## Run integration tests only (cluster must be prepared).
	go test ./test/integration/ -tags=integration -v -timeout 5m -failfast

.PHONY: test-integration
test-integration: manifests generate cleanup-integration test-integration-run ## ...

.PHONY: test-e2e-run
test-e2e-run: ## Run e2e tests only (operator must already be deployed).
	go test ./test/e2e/ -tags=e2e -v -timeout 5m -failfast

.PHONY: test-e2e
test-e2e: cleanup-e2e deploy-helm test-e2e-run ## ...
```

After manual `deploy-helm`, use **`make test-e2e-run`** — not `make test-e2e`
(which re-runs cleanup and deploy). See [e2e-workflow.md](e2e-workflow.md).

### Cleanup scripts

Copy `hack/scripts/cleanup-integration.sh` and `cleanup-e2e.sh` from the ray
module and update for `$KIND` / `$COMPONENT`:

| Script | Namespace default | Deletes |
|--------|-------------------|---------|
| `cleanup-integration.sh` | `integration-test` | Module CRs (cluster-scoped), workload + RBAC in test namespace, `part-of=$COMPONENT` ClusterRoles/Bindings, integration test RBAC, module CRD |
| `cleanup-e2e.sh` | `$MODULE_NAME-system` | Module CRs, Helm release uninstall, operator namespace, leftover cluster RBAC, module CRD |

Use `--ignore-not-found` on all `kubectl delete` calls so cleanup is idempotent.

After [renaming.md](renaming.md), grep the scripts for stale ray names — they
are easy to miss:

```bash
rg 'ray|opendatahub-ray' hack/scripts/cleanup-*.sh && exit 1 || true
```

### In-process cleanup (integration only)

`TestMain` in integration tests should **also** delete leftovers via the API
client (defense in depth if someone runs `go test` without `make`):

```go
// After EnsureNamespace + InstallCRDs, before starting the manager:
_ = directClient.DeleteAllOf(ctx, &componentsv1alpha1.${Kind}{})
_ = directClient.DeleteAllOf(ctx, &appsv1.Deployment{}, client.InNamespace(testNamespace))
_ = directClient.DeleteAllOf(ctx, &corev1.Service{}, client.InNamespace(testNamespace))
```

E2e tests use a deployed operator — cluster cleanup is the Makefile/script
responsibility, not `TestMain`.

## Container build and e2e workflow

See [e2e-workflow.md](e2e-workflow.md) for the full step-by-step sequence
(IMG export, `make helm`, `test-e2e-run` vs `make test-e2e`).

## Test timeouts (fast feedback)

Tests must fail fast — never wait 30 minutes to learn something hung.

### `go test` flags (Makefile)

Use **short** package timeouts for local and CI feedback:

```makefile
# Integration / e2e — 5m is enough when Eventually timeouts are tight
go test ./test/integration/ -tags=integration -v -timeout 5m -failfast
go test ./test/e2e/ -tags=e2e -v -timeout 5m -failfast
```

During active debugging, run an even shorter ad-hoc timeout:

```bash
go test ./test/integration/ -tags=integration -v -timeout 3m -failfast -run TestBecomesReady
```

Always pass **`-failfast`**: the first failing subtest stops the run instead of
cascading hangs.

### `Eventually` timeouts (test code)

Keep per-assertion waits short — the package timeout is a backstop, not the
primary feedback loop:

```go
const (
    timeout  = 90 * time.Second  // max wait per Eventually (Ready, Deployment, etc.)
    interval = 2 * time.Second
)
```

Use **90s–2m** for `WithTimeout()` in integration/e2e tests. If a reconcile
routinely needs longer, fix the controller or test setup — do not inflate
timeouts to mask bugs.

E2e operator gate (deployment ready) should use the same `timeout` constant so
a missing operator fails in ~90s, not after the full package timeout.

### When a test hangs

1. Note which step last printed (build / push / deploy / which `t.Run`)
2. Re-run only that step with the same `IMG`
3. Lower `-timeout` and `-run` to isolate one test
4. Check operator logs: `kubectl logs -n $MODULE_NAME-system deploy/...`

## Unit Tests (`${name}_test.go`)

### Required tests for every module

```go
TestNewModule                // version parsing, manifest info, image params applied
TestNewModuleInvalidVersion  // bad semver → error
TestInitialize               // manifest appended, correct overlay/contextDir
TestUpgradeIfNeededFreshInstall   // status version zero → no upgrade
TestUpgradeIfNeededSameVersion    // same version → no upgrade
TestReportStatus             // version, platform, sources populated
```

### If the module has upgrade logic

**Do NOT test non-trivial upgrade migrations.** The teams owning the module
own the testing of upgrade paths. Implement the `upgrade()` function with the
migration logic but leave testing to the owning team.

Only test trivial upgrades (e.g., adding an annotation) and only in unit/integration:
```go
TestUpgradeIfNeededVersionAdvance        // version increased → upgrade hook is called
TestUpgradeIfNeededPlatformVersionChange // platform version increased → upgrade hook is called
// TestUpgrade — only if the migration is trivially verifiable (e.g. annotation added)
```

Never write e2e tests for upgrade paths.

### If the module has webhooks

```go
TestWebhookMutate           // verify mutation logic (labels, annotations, etc.)
TestWebhookValidate         // verify validation rejects bad input
TestWebhookSkipsWhenNotReady // verify webhook allows when CR not found/version unset
```

Use `fake.NewClientBuilder()` with a test scheme for webhook tests that
need a client.

### If the module has custom actions

```go
TestSetKustomizedParams     // verify params.env modifications
TestCheckPreConditions      // verify precondition pass/fail behavior
```

## Integration Tests (`test/integration/integration_test.go`)

Use **stdlib `testing.T` + Gomega**. See ray module
`test/integration/integration_test.go` for the canonical pattern:
`TestMain` → `Test$Kind(t *testing.T)` → `t.Run(...)` subtests →
`g := NewWithT(t)` → `g.Eventually(k.Get(...)).Should(jq.Match(...))`.

### Structure

```go
func TestMain(m *testing.M) {
    // 1. Create context
    // 2. Get kubeconfig
    // 3. Create direct client
    // 4. Ensure test namespace
    // 5. Install CRDs (create-or-update, NOT create-only)
    // 6. Clean up leftovers from previous runs (see Pre-test cleanup)
    // 7. Set viper namespace + cluster namespace
    // 8. Read operator ConfigMap for expected values
    // 9. Create module config
    // 10. Create manager with cache + client options
    // 11. Wrap with odhmanager
    // 12. Register reconciler
    // 13. Start manager in goroutine
    // 14. Wait for cache sync — FAIL if not synced
    // 15. Create RBAC
    // 16. Run tests
}
```

### Required tests for every module

```go
testBecomesReady        // create CR, wait for phase=Ready + conditions
testModuleStatus        // version, platform.name, sources populated
testPlatformLabels      // part-of label, instance/uid/type/version annotations
testOwnerReferences     // workload resources owned by module CR
```

### Module-specific tests (tailor to what the module does)

- If the module has a **precondition**: test that it blocks when dependency
  is missing, recovers when available
- If the module deploys **specific resources** beyond Deployment: verify
  those resources exist with correct labels
- If the module has **upgrade logic**: simulate version advance by patching
  status, trigger reconcile, verify effect

### Key patterns

- **Pre-test cleanup**: `make test-integration` / `make test-e2e` must run
  `cleanup-integration` / `cleanup-e2e` first (see Pre-test cleanup section)
- **Cleanup in TestMain**: `DeleteAllOf` for the module CR type + workload
  Deployments/Services in the test namespace
- **Cache sync gate**: `mgr.GetCache().WaitForCacheSync(ctx)` after starting
  manager, fail if false
- **InstallCRDs**: must use Get+Update (not Create-only) to replace existing
  CRDs that may have a different schema
- **failfast**: always run with `-failfast` to avoid hanging on cascading
  failures

## E2E Tests (`test/e2e/e2e_test.go`)

Same style as integration: **`testing.T` + Gomega**. E2e runs
against a **deployed operator on OpenShift** (Helm install into
`$MODULE_NAME-system`).

### Structure

```go
func TestMain(m *testing.M) {
    // 1. Create context
    // 2. Get kubeconfig
    // 3. Create direct client (no in-process manager)
    // 4. Run tests
}
```

Assumes the operator is already deployed. For manual step-by-step deploy, use
`test-e2e-run` after `cleanup-e2e` and `deploy-helm`. Composite `make test-e2e`
runs cleanup, deploy, and tests in one target.

```go
func Test$Kind(t *testing.T) {
    // Gate: verify operator deployment is running BEFORE registering subtests.
    // If the operator isn't deployed, fail immediately — don't let subtests hang.
    g := NewWithT(t)
    g.Eventually(k.Get(rt.operatorDeploy)).WithContext(ctx).WithTimeout(timeout).WithPolling(interval).Should(
        jq.Match(`.status.readyReplicas >= 1`),
    )

    t.Run("should have operator ConfigMap deployed", ...)
    t.Run("should become ready", ...)
    // ... module-specific tests
}
```

### Required tests for every module

```go
// Gate (not a subtest — runs before subtests, fails the whole test if operator not running)
testOperatorConfigMap   // ConfigMap has platform-type and platform-version
testOperatorEnvPrefix   // deployment env uses ODH_MODULE_OPERATOR_CONFIGURATION_PATH
testBecomesReady        // create CR, wait for Ready
testModuleStatus        // version, platform, sources
testPlatformLabels      // labels and annotations on workload resources
testOwnerReferences     // owner refs on workload resources
```

`testOperatorEnvPrefix` assertion (mirror root e2e):

```go
g.Eventually(k.Get(rt.operatorDeploy)).WithContext(ctx).WithTimeout(timeout).WithPolling(interval).Should(
    jq.Match(`.spec.template.spec.containers[0].env[] | select(.name == "ODH_MODULE_OPERATOR_CONFIGURATION_PATH") | .value == "/etc/controller/config"`),
)
```

### Module-specific e2e tests

Tailor to what the module actually does. Examples:
- Ray: no upgrade logic today → no upgrade tests
- A module with webhooks: verify webhook-injected labels
- A module with preconditions: test blocking and recovery

Do NOT copy upgrade/fault-injection tests from the mymodule template
unless the module actually has upgrade logic.

## Workload Resource Names

The test fixtures (`workloadDeploy`, etc.) must use the actual names from
the component's kustomize manifests, not template placeholders. Check the
downloaded manifests in `config/manifests/$COMPONENT/` to find the correct
Deployment and Service names.

## Common Pitfalls

1. **Dirty cluster**: running integration/e2e without cleanup first — always
   wire `cleanup-integration` / `cleanup-e2e` into Makefile test targets
2. **Long hangs**: `-timeout 30m` or inflated `Eventually` waits hide bugs — use
   `-timeout 5m -failfast` in Makefile and `90s–2m` per assertion; run build/
   push/deploy/test as separate steps (see Container build and e2e workflow)
3. **Chained make one-liners** or **`make test-e2e` after manual deploy** —
   use [e2e-workflow.md](e2e-workflow.md); run `test-e2e-run` after deploy
4. **CRD conflicts**: on clusters with monolith CRDs, `InstallCRDs` must
   update (not skip) to get the new `status.module` field
5. **Unused struct fields**: remove `workloadService` etc. from test structs
   if no test asserts on them
6. **Operator gate in e2e**: check operator deployment BEFORE subtests, not
   as a subtest — a failed subtest doesn't stop subsequent subtests
7. **Wrong env prefix**: deployment sets `ODH_OPERATOR_*` but `pkg/config`
   uses `ODH_MODULE_OPERATOR` — viper silently ignores mismatched vars
   during `Unmarshal()`. All env vars must use `ODH_MODULE_OPERATOR_`
   (see `controller-rules.md`).
