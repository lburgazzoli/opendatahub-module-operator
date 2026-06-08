# Testing Guide

## Test Structure

### Unit Tests

Location: alongside source files (`*_test.go`)

- `pkg/module/module_registry_test.go` — ModuleRegistry lookups, runlevel grouping, namespace dedup
- `internal/controller/platform/platform_test.go` — Platform action logic (advanceRunlevel, runlevelComplete) using fake client

Unit tests use `fake.NewClientBuilder()` for the k8s client — no cluster needed.

### Integration Tests

Location: `test/integration/`

Integration tests run against the current cluster.

```bash
make test-integration
```

The `Makefile` target runs the integration tree serially:

```bash
go test -p 1 -v -tags=integration -count=1 -timeout=60s ./test/integration/...
```

Serial execution matters because the packages share one cluster and can
interfere if started in parallel.

## Integration Test Architecture

### Package Layout

Integration coverage is split into dedicated packages with isolated managers:

- `test/integration/foundation`
- `test/integration/runlevel`
- `test/integration/gates`
- `test/integration/support` for the shared harness/runtime

Each package defines its own module fixtures in a local `*_support_test.go`
file and passes the full module definitions into the shared harness via
`isupport.Run(...)`. Generic test helpers and reusable matchers remain in
`test/support/`.

Shared chart fixtures live under `test/support/testdata/charts/`.

### Test Client

The test client is a **direct** (non-caching) client with a dynamic REST mapper:

```go
mapper, _ := apiutil.NewDynamicRESTMapper(kubeConfig, httpClient)
cli, _ := client.New(kubeConfig, client.Options{Scheme: testScheme, Mapper: mapper})
```

This avoids cache staleness issues. The test client is stored in the package
suite (`isupport.Suite`), NOT as a package-level var.

The shared harness waits for manager cache sync before the tests start creating
resources. That is only a cache-readiness barrier; tests must still wait on
observable reconciliation results with `Eventually` instead of adding sleeps.

### Stability Principle

Timing should not be the mechanism that makes the integration suite pass.

- Do not add arbitrary sleeps to make startup or reconciliation "settle".
- Do not treat larger timeouts as a real fix for nondeterministic behavior.
- If a test flakes, prefer fixing readiness, watch coverage, cache visibility, or
  cleanup semantics so the system becomes stable under normal scheduling.
- `WaitForCacheSync` is an acceptable startup barrier for the manager cache, but
  it is not a substitute for asserting real controller outcomes.

The current `go test -p 1` setting is a shared-cluster isolation workaround: the
packages mutate one cluster and must not race each other. It should not be
interpreted as a license to rely on timing within a package.

### Test Independence

**Each test must be self-contained**: set up its own state, verify, clean up via `t.Cleanup`.

Tests that check multiple aspects of the same deployed state use **inner `t.Run` subtests** within a parent that owns the setup/cleanup:

Top-level tests keep the scenario setup while inner `t.Run(...)` blocks verify
different aspects of the same deployed state.

### Foundation Tests

Test the basic module deployment lifecycle:

1. **Empty platform** — Create Platform CR with no modules, verify it reconciles
2. **Module deployment** — Enable modules, verify resources, labels, ownerRefs, chart info, config
3. **Version propagation** — Create module CR with version, verify PlatformOperator picks it up
4. **Disable modules** — Remove modules from spec, verify cleanup (CRDs and Namespaces survive)

### Runlevel Tests

Test upgrade progression:

1. **Upgrade triggered** — Version mismatch gates higher-runlevel modules
2. **Wrong version blocks** — Module CR with wrong version doesn't advance runlevel
3. **Correct version advances** — Correct version advances to next runlevel
4. **All modules ready** — All modules reporting correct version sets distribution version

Each runlevel test calls `prepareUpgradeScenario` which:
- Resets `cfg.Distribution.Version` to `"1.0.0"`
- Creates Platform CR, waits for initial reconciliation
- Changes version to `"2.0.0"` to trigger upgrade
- Enables modules, waits for runlevel 1 deployment

### Gates Tests

Admin-ack coverage lives under `test/integration/gates`.

The expected behavior is:

1. Missing admin-acks `ConfigMap` blocks reconciliation
2. Present but `false` ack blocks reconciliation
3. Updating the ack to `true` unblocks reconciliation

`ModulesReady` should only carry admin-ack details while the gate is blocking
(`Reason=AdminAcksRequired`). Once the gate is satisfied, that temporary
condition should disappear and normal `PlatformOperator` aggregation should own
the readiness status again.

### Creating Module CRs

Module CRDs are deployed by Helm charts at runtime and are handled in tests as
`unstructured.Unstructured` objects. The shared helper
`isupport.UpsertModuleCRWithVersion(...)` creates or updates the test CR and, if
requested, patches `status.release.version`.

### Gomega Patterns

Prefer `gomega-matchers` helpers as the first choice. Reach for custom polling
closures only when the matcher library does not already express the assertion
cleanly.

Prefer `jq.Matchf(...)` for scalar assertions:

```go
g.Eventually(ctx, suite.K.Get(po)).Should(
    jq.Matchf(`.status.distribution.version == %q`, "2.0.0"),
)
```

Prefer typed k8s extractors from `gomega-matchers` when the object is already typed:

```go
g.Eventually(ctx, suite.K.Get(cm)).Should(
    WithTransform(k8sm.Data(), HaveKeyWithValue("module-name", Equal("alpha"))),
)
```

```go
g.Eventually(ctx, suite.K.List(&configApi.PlatformOperatorList{})).Should(
    k8sm.IsEmptyList(),
)
```

Use repo-level matchers from `test/support/matchers.go` for higher-level
assertions such as tracked resources, chart info, distribution version, and
runlevel.

For delete checks, prefer the typed helpers:

```go
g.Eventually(ctx, suite.K.NotFound(obj)).Should(BeTrue())
```

In practice, prefer:

- typed helpers from `suite.K` / `k8sm`
- then repo-level matchers from `test/support/matchers.go`
- then `jq.Matchf(...)` / `jq.Match(...)`
- and only then a manual `Eventually(func() ...)` closure

### Timeouts

- Default Eventually timeout: 30s
- Default polling interval: 250ms
- `Consistently` for negative assertions: `timeout / 3` (~10s)
- `make test-integration` uses a 60s Go test binary timeout per package

### Known Issues

- **Dynamic REST mapper caching**: The mapper caches API group discovery. After deploying CRDs via Helm charts, the mapper might not discover them immediately. Use `Eventually` for Create/Get operations on test CRDs.
- **Module CR visibility**: `readModuleVersion` uses the manager's cached client. Module CRs created by the test's dynamic client might not be visible immediately due to informer sync delay.
- **Shared cluster state**: package-local integration suites must still run
  serially against one cluster.
- **Workaround boundary**: if a test only passes after adding sleeps or inflating
  timeouts, treat that as a bug in the harness or controller behavior, not as an
  acceptable long-term testing pattern.
