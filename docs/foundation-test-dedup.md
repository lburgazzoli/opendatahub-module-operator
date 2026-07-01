# Foundation Test Dedup: Shared `test/support/foundation/` Package

## Goal

Eliminate near-duplicate foundation test files (`integration_foundation_test.go` and `e2e_foundation_test.go`) across all 10 modules by extracting the shared logic into each module's `test/support/foundation/` package.

## Design

### Shared config struct

Each module's `test/support/foundation/foundation.go` will expose:

```go
package foundation

type Config struct {
    Client             client.Client
    WorkloadNamespace  string
    PlatformVersion    string
    PlatformType       string
    ModuleCRDName      string
    ManagedDeployment  string
    ComponentName      string
    // Factory for creating a fresh module CR instance
    NewModule          func() client.Object
    // GVK info for owner reference checks
    ModuleGVK          schema.GroupVersionKind
}

type Tests struct {
    Config
}

func (ft *Tests) Execute(t *testing.T) {
    t.Run("should have module CRD installed", ft.testModuleCRDInstalled)
    t.Run("should become ready", ft.testBecomesReady)
    t.Run("should report release version and platform", ft.testReleaseStatus)
    t.Run("should set platform labels and annotations", ft.testPlatformLabels)
    t.Run("should set owner references", ft.testOwnerReferences)
}
```

The struct fields replace all the differences between integration and e2e:
- `WorkloadNamespace` -- integration passes `IntegrationTestNamespace()`, e2e passes `OperatorNamespace()`
- `PlatformVersion` / `PlatformType` -- integration gets from `loadOperatorConfig()`, e2e reads from the live ConfigMap

### Integration side (`test/integration/integration_test.go`)

`TestMain` stays as-is (starts manager in-process). The test function becomes:

```go
func TestRay(t *testing.T) {
    cfg, _ := loadOperatorConfig()
    k8sClient, _ := support.NewClient()

    t.Run("foundation", (&foundation.Tests{Config: foundation.Config{
        Client:            k8sClient,
        WorkloadNamespace: support.IntegrationTestNamespace(),
        PlatformVersion:   cfg.ComponentRelease().Version,
        // ...
    }}).Execute)
}
```

Delete `integration_foundation_test.go`.

### E2E side (`test/e2e/e2e_test.go`)

`TestMain` stays as-is (checks deployed operator pod). The test function becomes:

```go
func TestRay(t *testing.T) {
    k8sClient, _ := support.NewClient()
    // read version from deployed ConfigMap
    version := readVersionFromConfigMap(t, k8sClient)

    t.Run("foundation", (&foundation.Tests{Config: foundation.Config{
        Client:            k8sClient,
        WorkloadNamespace: support.OperatorNamespace(),
        PlatformVersion:   version,
        // ...
    }}).Execute)
}
```

Delete `e2e_foundation_test.go`.

### E2E-only tests remain separate

- "Operator ConfigMap deployed" -- stays in `e2e_test.go` (only meaningful for deployed operator)
- Workbenches webhook tests -- stay in `e2e_webhook_test.go`

### Module-specific tests use descriptive per-concern file names

Module-specific tests stay in their tier directory with names that describe what they test:

| Module | File | Tests |
|--------|------|-------|
| datasciencepipelines | `test/integration/integration_argo_test.go` | Argo Workflow CRD ownership/missing scenarios |
| modelregistry | `test/integration/integration_gateway_test.go` | Runtime params immutability, registries namespace status |
| workbenches | `test/e2e/e2e_webhook_test.go` (already exists) | Connection + HardwareProfile webhook mutation/denial |
| workbenches | `test/upgrade/` (already exists, unaffected) | Container size to HardwareProfile migration |

## Modules by complexity

### Simple (identical pattern, straightforward extraction):
- `ray`, `spark`, `feast`, `ogx`, `trainer`, `trustyai`, `mlflow`

These 7 modules have identical foundation test structure. The shared package will be the same shape for all of them, just with different module types/constants.

### Custom (need extra handling):

- **datasciencepipelines**: shared foundation package for the common 5 tests. The DSP-specific Argo CRD tests (`testForeignOwnedArgoWorkflowCRD`, `testMissingArgoWorkflowCRD`) stay in `test/integration/` as a separate test struct or are composed on top of the foundation. DSP's `ensureReadyModule` also seeds the Argo CRD, so the foundation's `ensureReadyModule` needs a hook or DSP overrides it.
- **modelregistry**: shared foundation package for the common 5 tests. The `testRuntimeParamsWithoutMutatingSource` and `testRegistriesNamespaceStatus` stay in `test/integration/` as extras. MR's module creation includes a `Gateway.Domain` spec field, so `NewModule` factory handles this.
- **workbenches**: shared foundation for common tests. Webhook tests stay e2e-only. Upgrade tests are a separate `test/upgrade/` package, unaffected.

## File changes per module

For each of the 10 modules:

| Action | File |
|--------|------|
| **Create** | `test/support/foundation/foundation.go` |
| **Delete** | `test/integration/integration_foundation_test.go` |
| **Delete** | `test/e2e/e2e_foundation_test.go` |
| **Modify** | `test/integration/integration_test.go` (wire foundation.Config) |
| **Modify** | `test/e2e/e2e_test.go` (wire foundation.Config) |

For DSP, modelregistry, workbenches: extra tests move into dedicated files in their tier directory (e.g. `integration_argo_test.go` for DSP).

## Validation

After refactoring each module:
- `make test` should pass (unit tests unaffected)
- Foundation tests should compile in both `integration` and `e2e` packages
- No behavioral change -- same assertions, same test cases

## Skill and Reference Updates

The following skill files describe the current test structure and need to reflect the new `test/support/foundation/` pattern:

### `.claude/skills/odh-module-test/references/testing-structure.md`

Changes needed:
- **Integration Tests section**: replace the "Required tests for every module" list (`testBecomesReady`, `testModuleStatus`, etc.) with a reference to the shared `test/support/foundation/` package. Explain that foundation tests are parameterized via `foundation.Config` and called from both integration and e2e.
- **E2E Tests section**: same -- replace the "Required tests for every module" list with a reference to the shared foundation package. Keep the E2E-only tests (operator ConfigMap, operator env prefix) documented as e2e-specific.
- **Add a new "Foundation Tests" section** between Unit and Integration explaining the shared `test/support/foundation/foundation.go` pattern, the `Config` struct fields, and how both tiers call `Execute()`.
- **Module-specific tests subsection**: update to reference descriptive file naming (`integration_argo_test.go`, `integration_gateway_test.go`, `e2e_webhook_test.go`).

### `.agents/skills/odh-module-test/references/testing-structure.md`

Mirror the same changes as the `.claude/` version above (these two files are kept in sync).

### `.claude/skills/odh-module-test/references/testing-ops.md` and `.agents/skills/odh-module-test/references/testing-ops.md`

No structural changes needed. The cleanup scripts and Makefile targets are unaffected.

### `.agents/skills/odh-module-migrate/references/verification-gates.md`

The "Cleanup and test targets" section references `test-integration-run` and `test-e2e-run` but not specific test file names, so no change needed. If migration skills reference the old `integration_foundation_test.go` / `e2e_foundation_test.go` file pattern as something to create, update to reference the `test/support/foundation/` package instead.
