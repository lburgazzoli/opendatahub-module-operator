# Test Code Structure

## Unit Tests (`${name}_test.go`)

### Required tests for every module

```go
TestNewModule                // version parsing, manifest info, image params applied
TestNewModuleInvalidVersion  // bad semver -> error
TestInitialize               // manifest appended, correct overlay/contextDir
TestUpgradeIfNeededFreshInstall   // status version zero -> no upgrade
TestUpgradeIfNeededSameVersion    // same version -> no upgrade
TestReportStatus             // version, platform, sources populated
```

### If the module has upgrade logic

**Do NOT test non-trivial upgrade migrations.** The teams owning the module
own the testing of upgrade paths. Implement the `upgrade()` function with the
migration logic but leave testing to the owning team.

Only test trivial upgrades (e.g., adding an annotation) and only in unit/integration:
```go
TestUpgradeIfNeededVersionAdvance        // version increased -> upgrade hook is called
TestUpgradeIfNeededPlatformVersionChange // platform version increased -> upgrade hook is called
// TestUpgrade -- only if the migration is trivially verifiable (e.g. annotation added)
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
`TestMain` -> `Test$Kind(t *testing.T)` -> `t.Run(...)` subtests ->
`g := NewWithT(t)` -> `g.Eventually(k.Get(...)).Should(jq.Match(...))`.

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
    // 14. Wait for cache sync -- FAIL if not synced
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

For dependency-gated modules, write tests around the concrete dependency
resources, not around generic operator-installation metadata. Prefer explicit
cases for:

- missing required controller/operator CRD
- missing required controller/operator CR instance
- missing required operand CRD type
- all required dependency resources present

### Key patterns

- **Pre-test cleanup**: `make test-integration` / `make test-e2e` must run
  `cleanup-integration` / `cleanup-e2e` first (see testing-ops.md)
- **CRD install path**: integration CRDs come from `make prepare-integration` /
  `make test-integration`, not from Go test code
- **Cleanup in TestMain**: `DeleteAllOf` for the module CR type + workload
  Deployments/Services in the test namespace, plus `Eventually` / `Consistently`
  checks in the top-level test to verify stale singleton objects are gone
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
`test-e2e-run` after `cleanup-e2e` and `deploy-crc` / `deploy-helm`. Composite
`make test-e2e` runs cleanup, deploy, and tests in one target.

```go
func Test$Kind(t *testing.T) {
    // Gate: verify operator deployment is running BEFORE registering subtests.
    // If the operator isn't deployed, fail immediately -- don't let subtests hang.
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
// Gate (not a subtest -- runs before subtests, fails the whole test if operator not running)
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

Also assert the deployment carries:

```go
jq.Match(`.spec.template.spec.containers[0].env[] | select(.name == "ODH_MODULE_OPERATOR_NAMESPACE") | .valueFrom.fieldRef.fieldPath == "metadata.namespace"`)
```

### Module-specific e2e tests

Tailor to what the module actually does. Examples:
- Ray: no upgrade logic today -> no upgrade tests
- A module with webhooks: verify webhook-injected labels
- A module with preconditions: test blocking and recovery

Do NOT copy upgrade/fault-injection tests from the mymodule template
unless the module actually has upgrade logic.

## Workload Resource Names

The test fixtures (`workloadDeploy`, etc.) must use the actual names from
the component's kustomize manifests, not template placeholders. Check the
downloaded manifests in `config/manifests/$COMPONENT/` to find the correct
Deployment and Service names.
