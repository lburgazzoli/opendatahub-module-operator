# Orchestrator

Platform orchestrator that manages module lifecycle via two CRs:
- **Platform** (singleton `default-platform`): declares enabled modules in `spec.modules`
- **PlatformOperator** (per-module): tracks deployed resources, chart info, runlevel

## Architecture

Single PlatformOperator controller handles all modules. The `Orchestration`
interface (`pkg/module/orchestration.go`) decouples the two controller packages.

```
Platform controller (ensureModules -> deploy -> GC)
    |
    v
PlatformOperator CRs (created/deleted by deploy+GC)
    |
    v
Module reconciler (resolveModule -> checkRunlevel -> ensureNamespace -> renderChart -> deploy -> pruneOrphans -> reportStatus)
```

State changes (mode/runlevel) push to a channel source that re-triggers
module reconciliation.

## Make Targets

| Target | Purpose |
|--------|---------|
| `make test` | Unit tests (no cluster) |
| `make test-integration` | Integration tests (requires Docker for k3s) |
| `make lint` | Lint all code including integration/e2e tags |
| `make manifests generate` | Regenerate CRDs and deepcopy |

## Integration Tests

Run with `make test-integration`. Requires Docker (k3s-envtest uses
testcontainers). Tests create an empty Platform, enable modules, verify
deployment/ownership/labels/config, then disable modules and verify cleanup.

The test client is a direct (non-caching) client stored in the test suite
struct. Objects are always loaded fresh from the cluster, never cached.
