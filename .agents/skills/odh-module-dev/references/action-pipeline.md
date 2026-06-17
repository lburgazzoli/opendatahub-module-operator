# Action Pipeline

## Builder Setup

The `NewReconciler` function builds a sequential pipeline via
`reconciler.ReconcilerFor`. Module additions are inserted at defined positions
within the monolith's original action order.

### Module Builder Example

```go
reconciler.ReconcilerFor(mgr, &componentApi.Ray{}).
    // --- Owns: register every workload resource type ---
    Owns(&corev1.ConfigMap{}).
    Owns(&corev1.Secret{}).
    Owns(&rbacv1.ClusterRoleBinding{}).
    Owns(&rbacv1.ClusterRole{}).
    Owns(&rbacv1.Role{}).
    Owns(&rbacv1.RoleBinding{}).
    Owns(&corev1.ServiceAccount{}).
    Owns(&corev1.Service{}).
    Owns(&appsv1.Deployment{}, reconciler.WithPredicates(predicates.DefaultDeploymentPredicate)).
    OwnsGVK(gvk.SecurityContextConstraints).       // GVK variant for OpenShift types
    // --- Watches: external dependencies ---
    Watches(&extv1.CustomResourceDefinition{},
        reconciler.WithEventHandler(handlers.ToNamed(componentApi.RayInstanceName)),
        reconciler.WithPredicates(component.ForLabel(labels.ODH.Component(LegacyComponentName), labels.True)),
    ).
    WatchesGVK(gvk.CodeFlare, reconciler.Dynamic(reconciler.CrdExists(gvk.CodeFlare))).
    // --- Actions: monolith order + module additions ---
    WithAction(sanitycheck.NewAction(...)).
    WithAction(m.initialize).
    WithAction(m.upgradeIfNeeded).                  // MODULE: immediately after initialize
    WithAction(releases.NewAction()).
    WithAction(kustomize.NewAction(
        kustomize.WithLabel(labels.ODH.Component(LegacyComponentName), labels.True),
        kustomize.WithLabel(labels.K8SCommon.PartOf, LegacyComponentName),
    )).
    WithAction(deploy.NewAction(deploy.WithCache())).
    WithAction(deployments.NewAction()).
    WithAction(m.reportStatus).                     // MODULE: after deployments
    WithAction(gc.NewAction(gc.InNamespace(cfg.ApplicationsNamespace))).  // MODULE: explicit namespace
    // --- Conditions ---
    WithConditions(status.ConditionDeploymentsAvailable).
    Build(ctx)
```

## Rules

1. **Owns order**: preserve the monolith's exact order
2. **OpenShift types**: use `OwnsGVK(gvk.X)` instead of `Owns(&openshift.X{})`
   to avoid importing OpenShift API types directly
3. **Watches**: copy verbatim -- same predicates, same event handlers
4. **Action pipeline**: keep monolith order, insert `m.upgradeIfNeeded`
   **immediately** after `m.initialize` with **nothing in between**; insert
   `m.reportStatus` after `deployments.NewAction()`
5. **Kustomize labels**: copy exactly from monolith
6. **GC**: use `gc.InNamespace(cfg.ApplicationsNamespace)` (not bare `gc.NewAction()`)
7. **RBAC markers**: must match every Owns + Watches resource type
8. **Image params**: apply in `Init(ctx, reader)`, not in `NewModule` or `initialize`

## File Organization

| File | Contains | Example |
|------|----------|---------|
| `ray_controller.go` | `NewReconciler` + RBAC markers | Builder wiring only |
| `ray.go` | `Module` struct, `NewModule`, `initialize`, `reportStatus` | Lifecycle + status |
| `ray_actions.go` | Custom actions like `setKustomizedParams` | Business logic |
| `ray_upgrade.go` | `upgradeIfNeeded`, `upgrade` | Version migrations |
| `ray_webhook.go` | Webhook registration + handlers | Admission logic |
| `ray_test.go` | Unit tests | All test functions |

## NewModule -- Pure Constructor

`NewModule` builds the struct and selects the kustomize overlay. No I/O, no params.

```go
func NewModule(cfg *moduleconfig.Config) (*Module, error) {
    var overlay string
    switch odhcluster.Platform(cfg.PlatformName) {
    case odhcluster.SelfManagedRhoai, odhcluster.ManagedRhoai:
        overlay = overlayRhoai
    default:
        overlay = overlayODH
    }

    return &Module{
        cfg: cfg,
        manifestInfo: fwtypes.ManifestInfo{
            Path:       cfg.ManifestsPath,
            ContextDir: componentName,
            SourcePath: overlay,
        },
    }, nil
}
```

## Init -- Startup Lifecycle

`Init(ctx, reader)` is called once when the operator starts (by `modulemanager.New`).
It detects cluster state and writes image params + cluster-derived values to `params.env`.
This is **not** a reconcile action.

```go
func (m *Module) Init(ctx context.Context, reader client.Reader) error {
    info, err := odhcluster.DetectClusterInfo(ctx, reader)
    if err != nil {
        return fmt.Errorf("detecting cluster info: %w", err)
    }

    pp := path.Join(m.cfg.ManifestsPath, componentName, "base")

    if err := fwparams.Apply(
        pp,
        "params.env",
        fwparams.Replacement(
            fwparams.FromEnv(imageParamMap),
        ),
        fwparams.Values(map[string]string{
            platformVersionParamsKey: m.cfg.PlatformVersion,
            fipsEnabledParamsKey:     strconv.FormatBool(info.FipsEnabled),
        }),
    ); err != nil {
        return fmt.Errorf("failed to update params on path %s: %w", pp, err)
    }

    return nil
}
```

Key packages:
- `fwparams` = `github.com/opendatahub-io/odh-platform-utilities/framework/utils/params`
- `odhcluster` = `github.com/opendatahub-io/odh-platform-utilities/pkg/cluster`
- `fwtypes` = `github.com/opendatahub-io/odh-platform-utilities/framework/controller/types`

## initialize -- Per-Reconcile

Appends the manifest info to the reconciliation request. Params are already written by `Init`.

```go
func (m *Module) initialize(_ context.Context, rr *fwtypes.ReconciliationRequest) error {
    rr.Manifests = append(rr.Manifests, m.manifestInfo)
    return nil
}
```

## Scheme Registration

The operator's scheme must include ALL types the controller watches:

```go
import apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"

func init() {
    utilruntime.Must(clientgoscheme.AddToScheme(scheme))
    utilruntime.Must(apiextensionsv1.AddToScheme(scheme))  // required for CRD watches
    utilruntime.Must(componentsv1alpha1.AddToScheme(scheme))
}
```

Without this, the controller fails at startup with:
`unable to determine GVK for object: no kind is registered for the type v1.CustomResourceDefinition`
