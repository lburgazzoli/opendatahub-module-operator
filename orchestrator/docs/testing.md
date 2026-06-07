# Testing Guide

## Test Structure

### Unit Tests

Location: alongside source files (`*_test.go`)

- `pkg/module/module_registry_test.go` — ModuleRegistry lookups, runlevel grouping, namespace dedup
- `internal/controller/platform/platform_test.go` — Platform action logic (advanceRunlevel, runlevelComplete) using fake client

Unit tests use `fake.NewClientBuilder()` for the k8s client — no cluster needed.

### Integration Tests

Location: `test/integration/`

Uses k3s-envtest (testcontainers) for a real k8s cluster. Requires Docker (`DOCKER_HOST=unix:///var/run/docker.sock`).

```bash
make test-integration
```

## Integration Test Architecture

### Test Client

The test client is a **direct** (non-caching) client with a dynamic REST mapper:

```go
mapper, _ := apiutil.NewDynamicRESTMapper(kubeConfig, httpClient)
cli, _ := client.New(kubeConfig, client.Options{Scheme: testScheme, Mapper: mapper})
```

This avoids cache staleness issues. The test client is stored in the `orchestratorTest` suite struct, NOT as a package-level var.

A `dynamic.Interface` client is also available for creating module CRs (test CRDs deployed at runtime by Helm charts, not in the scheme).

### Test Independence

**Each test must be self-contained**: set up its own state, verify, clean up via `t.Cleanup`.

Tests that check multiple aspects of the same deployed state use **inner `t.Run` subtests** within a parent that owns the setup/cleanup:

```go
func (ft *foundationTests) testModuleDeployment(t *testing.T) {
    g := NewWithT(t)
    ft.createPlatformWithModules(t, g)  // setup + t.Cleanup

    t.Run("resources tracked", func(t *testing.T) { ... })
    t.Run("chart info reported", func(t *testing.T) { ... })
    t.Run("owner references set", func(t *testing.T) { ... })
}
```

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

Each runlevel test calls `setupUpgradeScenario` which:
- Resets `cfg.Distribution.Version` to `"1.0.0"`
- Creates Platform CR, waits for initial reconciliation
- Changes version to `"2.0.0"` to trigger upgrade
- Enables modules, waits for runlevel 1 deployment

### Creating Module CRs

Module CRDs are deployed by Helm charts at runtime. The test scheme doesn't know about them. Use the `dynamic.Interface` client:

```go
gvr := schema.GroupVersionResource{
    Group: mod.GVK.Group, Version: mod.GVK.Version,
    Resource: strings.ToLower(mod.GVK.Kind) + "s",
}
rt.suite.dynamic.Resource(gvr).Create(ctx, cr, metav1.CreateOptions{})
```

Wrap in `Eventually` since the CRD might not be deployed yet:

```go
g.Eventually(func() error {
    _, err := rt.suite.dynamic.Resource(gvr).Create(ctx, cr, metav1.CreateOptions{})
    return err
}).WithContext(ctx).Should(Succeed())
```

### Gomega Patterns

Use `WithTransform(jq.Extract(...), matcher)` for structured assertions on unstructured objects:

```go
g.Eventually(ft.suite.k.Get(po)).WithContext(ctx).Should(
    WithTransform(jq.Extract(`.status.distribution.version`), Equal("2.0.0")),
)
```

Use `ft.suite.k.Absent(obj)` to verify an object has been deleted:

```go
g.Eventually(ft.suite.k.Absent(p)).WithContext(ctx).Should(BeTrue())
```

### Timeouts

- Default Eventually timeout: 30s
- Default polling interval: 250ms
- `Consistently` for negative assertions: `timeout / 3` (~10s)
- Go test binary timeout: 60-120s (includes k3s startup ~14s)

### Known Issues

- **Dynamic REST mapper caching**: The mapper caches API group discovery. After deploying CRDs via Helm charts, the mapper might not discover them immediately. Use `Eventually` for Create/Get operations on test CRDs.
- **Module CR visibility**: `readModuleVersion` uses the manager's cached client. Module CRs created by the test's dynamic client might not be visible immediately due to informer sync delay.
- **k3s in Podman**: k3s containers fail in rootless Podman. Use Docker (`DOCKER_HOST=unix:///var/run/docker.sock`).
