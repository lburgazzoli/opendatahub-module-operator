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

r.Release = rel  // MODULE ADDITION: set release from config
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
7. **r.Release**: always set after Build
8. **RBAC markers**: must match every Owns + Watches resource type

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

## NewModule — One-Shot Setup

Equivalent to the monolith's `Init(platform, cfg)`:
```go
func NewModule(cfg *moduleconfig.Config) (*Module, error) {
    // 1. Parse versions
    v, err := componentApi.NewSemVer(version.Version)
    pv, _ := componentApi.NewSemVer(cfg.PlatformVersion)

    // 2. Compute manifest info ONCE
    mi := odhtypes.ManifestInfo{
        Path:       cfg.ManifestsPath,
        ContextDir: componentName,
        SourcePath: "openshift",  // from monolith's manifestPath()
    }

    // 3. Apply image params ONCE (from monolith's Init)
    if err := odhdeploy.ApplyParams(mi.String(), "params.env", imageParamMap); err != nil {
        return nil, fmt.Errorf("failed to update images: %w", err)
    }

    return &Module{cfg: cfg, version: v, platformVersion: pv, manifestInfo: mi}, nil
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

**Rule:** `ODH_MODULE_OPERATOR_` for every module operator — identical to the
root reference. Never use `ODH_OPERATOR_`. Never embed the component name
(e.g. `ODH_RAY_OPERATOR_*`). `pkg/config.EnvPrefix` and `ConfigPathEnvVar`
must match deployment env vars and `make run`.

Copy from root [`pkg/config/config.go`](../../../../pkg/config/config.go) and
[`config/manager/manager.yaml`](../../../../config/manager/manager.yaml).

These files must stay in sync:

| File | Required env vars |
|------|-------------------|
| `pkg/config/config.go` | `EnvPrefix`, `ConfigPathEnvVar` |
| `config/manager/manager.yaml` | `_CONFIGURATION_PATH`, `_MANIFESTS_PATH`, `_APPLICATIONS_NAMESPACE` |
| `config/default/manager_metrics_patch.yaml` | `_METRICS_BIND_ADDRESS` |
| `config/chart/templates/apps_v1_deployment.yaml` | same as manager |
| `Makefile` `run` target | `ODH_MODULE_OPERATOR_MANIFESTS_PATH=...` |

`RHAI_APPLICATIONS_NAMESPACE` is separate — set in deployments for
opendatahub-operator framework compatibility, not part of module config.

Verification grep (run from module root):

```bash
rg 'ODH_OPERATOR[^_M]|ODH_OPERATOR_|ODH_[A-Z]+_OPERATOR_' . && exit 1 || true
```

## Containerfile — Manifest Permissions

OpenShift assigns arbitrary UIDs to containers. Manifests baked into the image
must be world-readable so the init container (which copies them to a writable
emptyDir) can access them regardless of the assigned UID.

**Build split:** `make container-prep` runs on the host (`manifests`,
`generate`, `get-manifests` for fetch modules). The Containerfile only runs
`make build-bin` to compile the manager — generation and manifest fetch stay
off the critical path inside the image layer cache.

In the Containerfile:
```dockerfile
# In the builder stage — set permissions before copying to runtime
RUN chmod -R a+rX config/manifests/

# In the runtime stage — copy from builder (preserves permissions)
COPY --from=builder /workspace/config/manifests/ /manifests/
```

The manager Deployment uses an init container to copy manifests to a writable
volume, because `odhdeploy.ApplyParams` writes to `params.env` in-place:

```yaml
initContainers:
- name: copy-manifests
  image: controller:latest
  command: ["cp", "-r", "/manifests/.", "/opt/manifests/"]
  volumeMounts:
  - name: manifests
    mountPath: /opt/manifests
containers:
- name: manager
  volumeMounts:
  - name: manifests
    mountPath: /opt/manifests
  env:
  - name: ODH_MODULE_OPERATOR_MANIFESTS_PATH
    value: /opt/manifests
volumes:
- name: manifests
  emptyDir: {}
```

## initialize — Per-Reconcile

Equivalent to the monolith's `initialize` action:
```go
func (m *Module) initialize(_ context.Context, rr *odhtypes.ReconciliationRequest) error {
    rr.Manifests = append(rr.Manifests, m.manifestInfo)

    // Apply namespace params (imageParamMap is nil — images were set once in NewModule)
    if err := odhdeploy.ApplyParams(m.manifestInfo.String(), "params.env", nil,
        map[string]string{"namespace": m.cfg.ApplicationsNamespace}); err != nil {
        return fmt.Errorf("failed to update params.env: %w", err)
    }
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
