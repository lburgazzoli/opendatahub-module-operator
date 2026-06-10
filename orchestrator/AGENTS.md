# Orchestrator

Platform orchestrator that manages module lifecycle via two CRs:
- **Platform** (singleton `default-platform`): declares enabled modules in `spec.modules`
- **PlatformOperator** (per-module): tracks deployed resources, chart info, runlevel

## Architecture

Single PlatformOperator controller handles all modules. The `Orchestration`
interface (`pkg/module/orchestration.go`) decouples the two controller packages.

```
Platform controller (initialize -> checkAdminAcks -> ensureModules -> deploy -> pruneModules -> advanceRunlevel -> aggregateStatus)
    |
    v
PlatformOperator CRs (created/updated by deploy, removed by pruneModules)
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
| `make test-unit` | Explicit unit-test target |
| `make test-integration` | Integration tests against the current cluster |
| `make lint` | Lint all code |
| `make manifests generate` | Regenerate CRDs and deepcopy |

## Integration Tests

Run with `make test-integration`. The target installs CRDs and runs the
integration tree serially (`go test -p 1`) against the current cluster with a
120s test binary timeout.

Integration tests are split into package-local suites:

- `test/integration/foundation`
- `test/integration/runlevel`
- `test/integration/gates`
- shared harness in `test/integration/support`

Each package owns its own module fixture definitions and passes them into the
shared harness. Shared chart fixtures live under
`test/support/testdata/charts/`.

The harness waits for manager cache sync before tests begin, but tests should
still assert observable reconciliation results with `Eventually`; do not add
arbitrary sleeps as readiness substitutes.

Stability rule: timing should not be what makes the suite pass. `WaitForCacheSync`
is a valid cache-readiness barrier, but sleeps and inflated timeouts are not
real fixes for flakes. If a test is timing-sensitive, fix the underlying watch,
readiness, cache-visibility, or cleanup behavior so the system becomes stable.
The current `go test -p 1` setting is only a shared-cluster isolation workaround,
not permission to rely on timing inside a package.

For admin gates: `checkAdminAcks` returns a `PauseError` when admin acks are
unsatisfied and records warning events for observability. No condition is set —
the PauseError message carries the details, and the ConfigMap watch re-triggers
reconciliation when acks change.

Controller design rules worth preserving:

- Keep `*_actions.go` focused on methods that are directly registered with
  `WithAction(...)`.
- Move helper logic that is not itself an action into `*_support.go` so the
  action pipeline stays easy to scan.
- Prefer narrow watches and predicates (for example named singleton resources)
  over broad watches when the controller only cares about one object.
- When the action framework watches a GVK dynamically, treat the cache as
  unstructured-backed. Typed cache reads can race or miss objects if they do
  not match the informer shape, so prefer `pkg/resources.Get` / `List` for
  watched objects instead of adding sleeps or larger test timeouts.
- Treat transient gates such as admin acks as reconcile-time blockers via
  PauseError, not as conditions. Let normal aggregation (ConditionUpToDate)
  own readiness.

Matcher conventions that improved clarity in this repo:

- Prefer `gomega-matchers` helpers first, and only fall back to custom polling
  closures when there is no helper for the assertion shape you need.
- Prefer `jq.Matchf(...)` for scalar status assertions.
- Prefer `k8sm.Data()` / `k8sm.ListItems()` / `k8sm.IsEmptyList()` /
  `suite.K.NotFound(...)` for typed Kubernetes assertions instead of ad hoc jq
  or manual `Eventually(func() ...)`.
- Use repo-level helpers in `test/support/matchers.go` for tracked resource and
  runlevel assertions.
