# Framework Reference Notes

This document captures findings from searching the `odh-platform-utilities` framework
(module path: `github.com/opendatahub-io/odh-platform-utilities/framework`, resolved via
the `all-in` branch replace directive — see `go.mod`) so future tasks can avoid re-searching.

## Key packages

| Import path | What it provides |
|---|---|
| `framework/controller/reconciler` | `ReconcilerFor[T]`, builder API, `WithDefaultRequeueAfter`, `WithRelease`, `WithConditions`, `WithDefaultRequeueAfter` |
| `framework/controller/actions` | `actions.Fn` type alias, `actions.Getter[T]` type |
| `framework/controller/actions/errors` | `StopError`, `NewStopError`, `NewStopErrorWithRequeueAfter` |
| `framework/controller/actions/deploy` | `deploy.NewAction` (SSA apply via `rr.Resources`) |
| `framework/controller/actions/gc` | `gc.NewAction(namespaceFn, ...)` |
| `framework/controller/actions/render/kustomize` | `kustomize.NewAction(...)` — renders `rr.Manifests` → `rr.Resources` |
| `framework/controller/actions/render/template` | `fwtemplate.NewAction(...)` — renders `rr.Templates` → `rr.Resources` |
| `framework/controller/actions/status/deployments` | `deployments.NewAction(...)`, `DefaultConditionType = "DeploymentsAvailable"` |
| `framework/controller/actions/status/releases` | `releases.NewAction(...)`, `releases.ReadComponentReleases`, `releases.NormalizeComponentReleases` |
| `framework/controller/conditions` | `conditions.Manager`, `.Mark(type, status, opts...)`, `.SetCondition(cond)`, `.WithReason(...)`, `.WithMessage(...)` |
| `framework/controller/types` | `ReconciliationRequest` (has `.Conditions *conditions.Manager`, `.Instance`, `.Release`, `.Manifests`, `.Templates`, `.Resources`, `.ManifestsBasePath`) |
| `framework/controller/handlers` | `handlers.ToNamed(name)` — event handler for Watches |
| `framework/controller/predicates` | `predicates.DefaultDeploymentPredicate` |
| `framework/controller/predicates/label` | `labelpred.ForLabel(key, value)` |
| `api/common` | `common.Status`, `common.Condition`, `common.ComponentRelease`, `common.ComponentReleaseStatus`, `common.PlatformObject` (interface: `client.Object` + `WithStatus` + `ConditionsAccessor` + `WithReleases`) |
| `framework/api` | `fwapi.Release` (has `Version semver.Version`, `Name fwapi.Platform`), `fwapi.PlatformObject = common.PlatformObject` |
| `framework/manager` | `odhmanager.New(ctrlMgr)` — wraps controller-runtime manager |
| `pkg/cache` | `libcache.StripUnusedFields()` — cache transform for manager |

## `ReconcilerFor[T api.PlatformObject]` builder key methods

```go
reconciler.ReconcilerFor(mgr, &MyType{}).
    Owns(&corev1.Secret{}).                              // owns + watches
    OwnsGVK(gvk.MyGVK).                                 // owns non-registered types
    Watches(&SomeType{}, ...opts).                       // watches without owning
    WatchesGVK(gvk.Foo, reconciler.Dynamic(...)).        // conditional GVK watch
    WatchesRawSource(src).                               // arbitrary source.Source
    WithReconcilerOpts(
        reconciler.WithRelease(platformRelease),         // sets rr.Release
        reconciler.WithDefaultRequeueAfter(interval),    // fires on every successful reconcile with no explicit requeue
    ).
    WithAction(myActionFn).                              // appends an action.Fn
    WithConditions("MyConditionType").                   // registers dependent conditions
    Build(ctx)                                           // returns (*Reconciler, error)
```

## Upgrade-gating pattern (platform-version-based, not binary-version-based)

All modules compare `rr.Release.Version` (platform version from ConfigMap, injected via
`reconciler.WithRelease(platformRelease)`) against the version stored in
`status.releases[name=="platform"].version`. If `rr.Release.Version.GT(prevVersion)`, the
controller runs migrations then returns (the release-status update happens elsewhere).
This is **platform-version gated**, not binary-version gated — the annotation-based approach
proposed in the initial plan docs was replaced during implementation.

The shared `pkg/controller.UpgradeIfNeeded(fns ...MigrateFn) actions.Fn` helper encapsulates
this pattern so individual controllers don't repeat the semver boilerplate.

## Action condition setting from inside an action

```go
func myAction(_ context.Context, rr *odhtypes.ReconciliationRequest) error {
    // Mark a condition False
    rr.Conditions.Mark("MyCondition", metav1.ConditionFalse,
        conditions.WithReason("MyReason"),
        conditions.WithMessage("message here"))
    
    // Stop reconciliation (no requeue)
    return errors.NewStopError("stopping because X")
    
    // Stop and requeue after a delay
    return errors.NewStopErrorWithRequeueAfter(30*time.Second, "waiting for X")
}
```

## File conventions (per this repo)

- `<name>.go` — `Module` struct, `NewModule(cfg, ...Option) *Module`
- `<name>_controller.go` — `NewReconciler(ctx, mgr, cfg) error` **MERGED into `<name>.go`** for this module
- `<name>_actions.go` — action functions as `(m *Module)` methods
- `<name>_upgrade.go` — upgrade/migration functions (optional)
- `<name>_options.go` — `type Option func(*Module)` + `WithXxx` constructors
- `<name>_support.go` — internal helpers

## Import alias conventions (this module)

- `infraApi` for `api/infrastructure/v1alpha1` (not `infrav1alpha1`)
- `servicesv1alpha1` for `api/services/v1alpha1`
- `dbcontroller` for `pkg/controller`

## `ReconciliationRequest` fields used in actions

```go
rr.Instance           // the reconciled object (cast to concrete type)
rr.Release            // fwapi.Release{Version semver.Version, Name Platform}
rr.Client             // sigs.k8s.io/controller-runtime/pkg/client.Client
rr.Conditions         // *conditions.Manager (call .Mark/.SetCondition)
rr.Manifests          // []ManifestInfo (set by stageManifests, consumed by kustomize)
rr.Templates          // []TemplateInfo (set by custom action, consumed by fwtemplate)
rr.Resources          // UnstructuredList (populated by kustomize/fwtemplate, consumed by deploy/gc)
rr.ManifestsBasePath  // string set by modulemanager.New
```
