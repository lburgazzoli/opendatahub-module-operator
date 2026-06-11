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
- `test/integration/errors`
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

Helm engine conventions:

- Set `ReleaseNamespace` and `ReleaseVersion` on `helm.Source` directly;
  do not use `namespace.EnsureDefault` or similar post-render transformers
  for values that the engine can wire natively.
- The `moduleMetadata` transformer adds `platform.opendatahub.io/part-of`
  label to all rendered resources **except CRDs** (cluster-scoped, no
  ownership). Namespaces get the label from the deploy framework with a
  different value — do not assert module-name equality on Namespace labels.

Prometheus metrics conventions:

- Register custom metrics via `prometheus.New*` + `metrics.Registry.MustRegister()`
  in `init()` (kubebuilder pattern). Do not use `promauto`.
- Metric definitions live in `*_metrics.go` alongside the controller that emits them.
- Metric names use `odh_` prefix: `odh_platform_*` for Platform controller,
  `odh_platform_operator_*` for PlatformOperator controller.
- Export metric name strings as constants (`MetricPlatformRunlevel`, etc.)
  and label names as constants (`LabelName`, `LabelRunlevel`, etc.) so tests
  can reference them without importing metric vars.
- For info-style gauge vecs that carry version labels, use `DeletePartialMatch`
  before `Set(1)` to avoid stale time series on label-value changes.
- Record metrics at the end of action methods, not in support helpers — keeps
  the metric call sites visible in the action pipeline.

Constants and labels:

- Use `configApi.ConditionReady`, `configApi.ConditionUpToDate`,
  `configApi.ReasonAdminAckRequired` — never string literals for
  condition types/reasons.
- Use `odhLabels.PlatformPartOf` for the `platform.opendatahub.io/part-of`
  label key — never the raw string.
- Use `gvk.*` constants for GVK comparisons (`gvk.Namespace`,
  `gvk.CustomResourceDefinition`, `gvk.ConfigMap`, etc.) — always full
  GVK comparison, never kind-only.

Matcher conventions:

- Use package-level `k8sm.Get(cli, obj)`, `k8sm.List(cli, list)`,
  `k8sm.Update(cli, obj, fn)`, `k8sm.NotFound(cli, obj)`,
  `k8sm.Absent(cli, obj)`, `k8sm.Events(cli, opts...)`,
  `k8sm.Upsert(cli, obj, fn)`, `k8sm.StatusUpdate(cli, obj, fn)`.
  Do not use the old `k8sm.NewResources(cli, scheme)` wrapper.
- Prefer `k8sm.Conditions()` extractor with `ContainElement(SatisfyAll(...))`
  over `jq.Match` for condition assertions.
- Keep `jq.Matchf(...)` for scalar status assertions and `jq.Extract`
  for spec field extraction.
- Use `k8sm.Data()` / `k8sm.ListItems()` / `k8sm.IsEmptyList()` /
  `k8sm.HasLabel()` / `k8sm.IsControlledBy()` for typed Kubernetes
  assertions.
- For event assertions: use `k8sm.Events(cli, k8sm.ForObject(...))`
  with `ContainElement(SatisfyAll(...))`.
- When asserting on deployed resources, iterate `status.resources` from
  the PlatformOperator and use a `switch` on `schema.FromAPIVersionAndKind`
  to apply per-resource-type matchers (skip CRD/NS from ownership,
  add data assertions for ConfigMap, etc.).
- Use `isupport.ObjectFromResourceRef(ref)` to build unstructured objects
  from `configApi.ResourceRef` for `k8sm.Get`.
- Use repo-level helpers in `test/support/matchers.go` for tracked resource
  and runlevel assertions.
