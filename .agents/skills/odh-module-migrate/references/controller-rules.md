# Controller Porting Rules

## Builder Setup — Match the Monolith

The `NewReconciler` function must replicate the monolith's fluent builder
setup as closely as possible. The only additions are module-specific actions
(`upgradeIfNeeded`, `reportStatus`) inserted at defined positions.

### Example: ray monolith → ray module

**Monolith** (`ray_controller.go`):
```go
reconciler.ReconcilerFor(mgr, &componentApi.Ray{}).
    Owns(&corev1.ConfigMap{}).
    Owns(&corev1.Secret{}).
    Owns(&rbacv1.ClusterRoleBinding{}).
    Owns(&rbacv1.ClusterRole{}).
    Owns(&rbacv1.Role{}).
    Owns(&rbacv1.RoleBinding{}).
    Owns(&corev1.ServiceAccount{}).
    Owns(&corev1.Service{}).
    Owns(&appsv1.Deployment{}, reconciler.WithPredicates(predicates.DefaultDeploymentPredicate)).
    Owns(&securityv1.SecurityContextConstraints{}).
    Watches(&extv1.CustomResourceDefinition{},
        reconciler.WithEventHandler(handlers.ToNamed(componentApi.RayInstanceName)),
        reconciler.WithPredicates(component.ForLabel(labels.ODH.Component(LegacyComponentName), labels.True)),
    ).
    WatchesGVK(gvk.CodeFlare, reconciler.Dynamic(reconciler.CrdExists(gvk.CodeFlare))).
    WithAction(sanitycheck.NewAction(sanitycheck.WithUnwantedResource(gvk.CodeFlare, status.CodeFlarePresentMessage))).
    WithAction(initialize).
    WithAction(releases.NewAction()).
    WithAction(kustomize.NewAction(
        kustomize.WithLabel(labels.ODH.Component(LegacyComponentName), labels.True),
        kustomize.WithLabel(labels.K8SCommon.PartOf, LegacyComponentName),
    )).
    WithAction(deploy.NewAction(deploy.WithCache())).
    WithAction(deployments.NewAction()).
    WithAction(gc.NewAction()).
    WithConditions(conditionTypes...).
    Build(ctx)
```

**Module** — same structure, with module additions marked:
```go
reconciler.ReconcilerFor(mgr, &componentApi.Ray{}).
    // --- Owns: IDENTICAL to monolith ---
    Owns(&corev1.ConfigMap{}).
    Owns(&corev1.Secret{}).
    Owns(&rbacv1.ClusterRoleBinding{}).
    Owns(&rbacv1.ClusterRole{}).
    Owns(&rbacv1.Role{}).
    Owns(&rbacv1.RoleBinding{}).
    Owns(&corev1.ServiceAccount{}).
    Owns(&corev1.Service{}).
    Owns(&appsv1.Deployment{}, reconciler.WithPredicates(predicates.DefaultDeploymentPredicate)).
    OwnsGVK(gvk.SecurityContextConstraints).  // GVK variant, same effect
    // --- Watches: IDENTICAL to monolith ---
    Watches(&extv1.CustomResourceDefinition{},
        reconciler.WithEventHandler(handlers.ToNamed(componentApi.RayInstanceName)),
        reconciler.WithPredicates(component.ForLabel(labels.ODH.Component(LegacyComponentName), labels.True)),
    ).
    WatchesGVK(gvk.CodeFlare, reconciler.Dynamic(reconciler.CrdExists(gvk.CodeFlare))).
    // --- Actions: monolith order preserved, module additions inserted ---
    WithAction(sanitycheck.NewAction(sanitycheck.WithUnwantedResource(gvk.CodeFlare, status.CodeFlarePresentMessage))).
    WithAction(m.initialize).
    WithAction(m.upgradeIfNeeded).              // MODULE ADDITION: immediately after initialize
    WithAction(releases.NewAction()).
    WithAction(kustomize.NewAction(
        kustomize.WithLabel(labels.ODH.Component(LegacyComponentName), labels.True),
        kustomize.WithLabel(labels.K8SCommon.PartOf, LegacyComponentName),
    )).
    WithAction(deploy.NewAction(deploy.WithCache())).
    WithAction(deployments.NewAction()).
    WithAction(m.reportStatus).                 // MODULE ADDITION: after deployments
    WithAction(gc.NewAction(gc.InNamespace(cfg.ApplicationsNamespace))).  // MODULE CHANGE: explicit namespace
    // --- Conditions: IDENTICAL ---
    WithConditions(status.ConditionDeploymentsAvailable).
    Build(ctx)
```

### Rules

1. **Owns order**: preserve the monolith's exact order
2. **OpenShift types**: use `OwnsGVK(gvk.X)` instead of `Owns(&openshift.X{})`
   to avoid importing OpenShift API types directly
3. **Watches**: copy verbatim — same predicates, same event handlers
4. **Action pipeline**: keep monolith order, insert `m.upgradeIfNeeded`
   **immediately** after `m.initialize` with **nothing in between**; insert
   `m.reportStatus` after `deployments.NewAction()`
5. **Kustomize labels**: copy exactly from monolith
6. **GC**: use `gc.InNamespace(cfg.ApplicationsNamespace)` (not bare `gc.NewAction()`)
7. **RBAC markers**: must match every Owns + Watches resource type

## GVK Package Rule

See the **odh-module-dev** skill's
[gvk-rule.md](../../odh-module-dev/references/gvk-rule.md) for the full rule.
In short: each module defines `pkg/resources/gvk/gvk.go`; controller and
chartgen code import that local package, never upstream `pkg/cluster/gvk`
directly.

## Dependency Checks — Prefer Types and Instances

When porting preconditions and dependency logic, prefer checking for concrete
API types and required instances instead of checking whether some "operator" is
installed.

Default order of preference:

1. **Required CRD types exist**
2. **Required CR instances exist** (for singleton/operator-managed resources)
3. **Operand CRD types exist** to prove the dependent API surface is installed

Avoid using operator-installation metadata such as Subscription state, CSV
presence, or generic operator-health checks as the primary gate when a concrete
CRD or CR can be checked directly.

### Example

For a JobSet-backed dependency, prefer:

- JobSet operator CRD exists
- `JobSetOperator` CR instance exists
- JobSet workload CRD exists

instead of:

- "JobSet operator is installed"

### Porting Guidance

- If the monolith uses `MonitorOperator(...)`, `OperatorExists(...)`, or other
  operator-level checks, treat that as a hint about the dependency, not as the
  required module implementation.
- In the module, re-express the dependency in terms of the concrete CRDs and
  CRs the component actually needs in order to reconcile safely.
- Tests should cover missing-CRD and missing-CR cases explicitly.

### RBAC Marker Checklist

Three RBAC surfaces — do not conflate them:

1. **Owns / Watches** — operator needs get/list/watch/create/update/patch/delete
   on every resource type the reconciler owns or watches.
2. **Baseline module-operator RBAC** — every module operator carries CRD RBAC,
   and every module that exposes `/metrics` carries the protected-metrics RBAC
   markers even if the monolith did not call them out explicitly.
3. **Deployed operand** — operator SA must hold every permission granted by
   ClusterRoles (and Roles) in the kustomize build, or deploy fails with RBAC
   escalation errors.

After `make get-manifests`, run [manifest-rbac-audit.md](manifest-rbac-audit.md).
The module must **own every resource the build deploys (except CRDs)** and
**hold RBAC for everything the deployed operand handles**.

For every `Owns()` or `OwnsGVK()`, add a corresponding RBAC marker:
```go
// +kubebuilder:rbac:groups=$GROUP,resources=$RESOURCE,verbs=get;list;watch;create;update;patch;delete
```

Every module operator keeps the baseline CRD marker:
```go
// +kubebuilder:rbac:groups=apiextensions.k8s.io,resources=customresourcedefinitions,verbs=get;list;watch;create;update;patch;delete
```

If the module exposes `/metrics`, keep the protected-metrics markers:
```go
// +kubebuilder:rbac:groups=authentication.k8s.io,resources=tokenreviews,verbs=create
// +kubebuilder:rbac:groups=authorization.k8s.io,resources=subjectaccessreviews,verbs=create
// +kubebuilder:rbac:urls=/metrics,verbs=get
```

## File Organization

| File | Contains | Example |
|------|----------|---------|
| `ray_controller.go` | `NewReconciler` + RBAC markers | Builder wiring only |
| `ray.go` | `Module` struct, `NewModule`, `initialize`, `reportStatus` | Lifecycle + status |
| `ray_actions.go` | Custom actions like `setKustomizedParams` | Business logic |
| `ray_upgrade.go` | `upgradeIfNeeded`, `upgrade` | Version migrations |
| `ray_webhook.go` | Webhook registration + handlers | Admission logic |
| `ray_test.go` | Unit tests | All test functions |

## NewModule — Pure Constructor

`NewModule` is a pure constructor: selects the overlay via a `switch` and builds
the struct. No I/O, no params.

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

## Init — Startup Lifecycle

`Init(ctx, reader)` is called once at operator startup by `modulemanager.New`.
It detects cluster state and writes image params + cluster-derived values to
`params.env`. Do **not** call `fwparams.Apply` in `NewModule` or `initialize`.

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
        fwparams.Replacement(fwparams.FromEnv(imageParamMap)),
        fwparams.Values(map[string]string{
            platformVersionParamsKey: m.cfg.PlatformVersion,
            // add component-specific cluster-derived values here
        }),
    ); err != nil {
        return fmt.Errorf("failed to update params on path %s: %w", pp, err)
    }

    return nil
}
```

## Scheme Registration

The operator's scheme must include ALL types the controller watches. If the
controller watches `CustomResourceDefinition`, add `apiextensionsv1` to the
scheme in `cmd/operator/operator.go`:

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

## Env prefix

See [env-prefix.md](env-prefix.md) for the full rule and file sync table.

## Containerfile — Manifest Permissions

See the **odh-module-deploy** skill's
[containerfile.md](../../odh-module-deploy/references/containerfile.md).

## initialize — Per-Reconcile

Appends manifest info to the reconciliation request. Params are already written
by `Init` at startup — do not call `fwparams.Apply` here.

```go
func (m *Module) initialize(_ context.Context, rr *fwtypes.ReconciliationRequest) error {
    rr.Manifests = append(rr.Manifests, m.manifestInfo)
    return nil
}
```

## Webhook Server — Remove for all modules

The monolith registers webhooks on the `DataScienceCluster` and
`DSCInitialization` resources. These have no equivalent in the module
operator, so **remove the WebhookServer** from `cmd/operator/operator.go`
for every module unless the component has its own resource-specific webhooks
(e.g., a webhook that mutates the component CR itself or its deployed workloads).

For the simple components (ray, sparkoperator, feastoperator, ogx,
mlflowoperator, trustyai, trainer) — none have such webhooks.

**Remove from `cmd/operator/operator.go`**:
```go
// DELETE these lines:
webhookserver "sigs.k8s.io/controller-runtime/pkg/webhook"
...
WebhookServer: webhookserver.NewServer(webhookserver.Options{
    Port:    cfg.WebhookPort,
    CertDir: cfg.WebhookCertDir,
}),
```

**Also clear** `config/webhook/manifests.yaml` (leave it empty or with a comment)
and **remove** `- ../webhook` from `config/default/kustomization.yaml`.

Failing to do this causes: `Error: open /tmp/k8s-webhook-server/serving-certs/tls.crt: no such file or directory`
