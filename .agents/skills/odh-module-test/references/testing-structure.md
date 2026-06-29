# Test Code Structure

## Unit Tests (`${name}_test.go`)

### Required tests for every module

```go
TestNewModule                        // manifest info fields (Path/ContextDir/SourcePath) + config stored; default overlay
TestNewModuleSelectsRhoaiOverlay     // RHOAI platform name -> rhoai overlay selected
TestInitialize                       // manifest appended to rr.Manifests, correct path/contextDir/overlay
TestUpgradeIfNeededFreshInstall      // status version zero -> no upgrade
TestUpgradeIfNeededSameVersion       // same version -> no upgrade
TestReportStatus                     // version, platform, sources populated
```

`Init(ctx, reader)` requires a live cluster reader (`DetectClusterInfo`) and is **not**
unit-tested. Cover it through integration tests.

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
    os.Exit(runTestMain(m))
}

func runTestMain(m *testing.M) int {
    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()

    // 1. Load Gomega config (EventuallyTimeout, polling intervals)
    gomegaCfg, _ := support.LoadGomegaConfig()
    SetDefaultEventuallyTimeout(gomegaCfg.EventuallyTimeout)
    SetDefaultEventuallyPollingInterval(gomegaCfg.EventuallyPollingInterval)

    // 2. Build config and module config from support helpers
    moduleCfg, _ := loadOperatorConfig()
    moduleCfg.ApplicationsNamespace = support.IntegrationTestNamespace()
    // ... disable metrics/leader election/pprof for test

    // 3. Create direct client; ensure test namespace; pre-clean leftovers
    cli, _ := support.NewClient()
    support.EnsureNamespace(ctx, cli, moduleCfg.ApplicationsNamespace)
    _ = cli.DeleteAllOf(ctx, &componentsv1alpha1.MyModule{})
    _ = cli.DeleteAllOf(ctx, &appsv1.Deployment{}, client.InNamespace(moduleCfg.ApplicationsNamespace))
    // ... delete other workload types

    // 4. Gate: fail fast if module CRD is not pre-installed
    if err := cli.Get(ctx, client.ObjectKeyFromObject(moduleCRD), moduleCRD); err != nil {
        fmt.Fprintf(os.Stderr, "Expected CRD %s to be installed: %v\n", crdName, err)
        return 1
    }

    // 5. Create manager (calls module.Init internally, registers reconciler)
    mgr, _ := modulemanager.New(ctx, cfg, moduleCfg)
    go mgr.Start(ctx)

    // 6. Wait for cache sync
    if !mgr.GetCache().WaitForCacheSync(ctx) {
        fmt.Fprintf(os.Stderr, "Failed to sync manager cache\n")
        return 1
    }

    return m.Run()
}
```

CRDs must be installed from `make test-integration-setup` **before** running tests.
Tests fail fast (not install) if the module CRD is missing. Do not install CRDs from Go test code.

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
- **CRD install path**: integration CRDs come from `make test-integration-setup` /
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
`test-e2e-run` after `cleanup-e2e` and `deploy-helm`. Composite
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
testOperatorConfigMap   // ConfigMap has platformType and platformVersion
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

Do NOT add upgrade/fault-injection tests unless the module actually has upgrade logic.

## Workload Resource Names

The test fixtures (`workloadDeploy`, etc.) must use the actual names from
the component's kustomize manifests, not template placeholders. Check the
downloaded manifests in `config/manifests/$COMPONENT/` to find the correct
Deployment and Service names.
