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

r.Release = rel  // MODULE: set release from config
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
7. **r.Release**: always set after Build
8. **RBAC markers**: must match every Owns + Watches resource type

## File Organization

| File | Contains | Example |
|------|----------|---------|
| `ray_controller.go` | `NewReconciler` + RBAC markers | Builder wiring only |
| `ray.go` | `Module` struct, `NewModule`, `initialize`, `reportStatus` | Lifecycle + status |
| `ray_actions.go` | Custom actions like `setKustomizedParams` | Business logic |
| `ray_upgrade.go` | `upgradeIfNeeded`, `upgrade` | Version migrations |
| `ray_webhook.go` | Webhook registration + handlers | Admission logic |
| `ray_test.go` | Unit tests | All test functions |

## NewModule -- One-Shot Setup

Equivalent to the monolith's `Init(platform, cfg)`:

```go
func NewModule(cfg *moduleconfig.Config) (*Module, error) {
    v, err := componentApi.NewSemVer(version.Version)
    pv, _ := componentApi.NewSemVer(cfg.PlatformVersion)

    mi := odhtypes.ManifestInfo{
        Path:       cfg.ManifestsPath,
        ContextDir: componentName,
        SourcePath: "openshift",
    }

    if err := odhdeploy.ApplyParams(mi.String(), "params.env", imageParamMap); err != nil {
        return nil, fmt.Errorf("failed to update images: %w", err)
    }

    return &Module{cfg: cfg, version: v, platformVersion: pv, manifestInfo: mi}, nil
}
```

## initialize -- Per-Reconcile

```go
func (m *Module) initialize(_ context.Context, rr *odhtypes.ReconciliationRequest) error {
    rr.Manifests = append(rr.Manifests, m.manifestInfo)

    if err := odhdeploy.ApplyParams(m.manifestInfo.String(), "params.env", nil,
        map[string]string{"namespace": m.cfg.ApplicationsNamespace}); err != nil {
        return fmt.Errorf("failed to update params.env: %w", err)
    }
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
