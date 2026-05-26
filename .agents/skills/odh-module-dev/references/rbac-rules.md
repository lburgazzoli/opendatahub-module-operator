# RBAC Rules

## Three RBAC Surfaces

Do not conflate these -- they are distinct:

1. **Owns / Watches** -- the operator needs
   `get;list;watch;create;update;patch;delete` on every resource type the
   reconciler owns or watches.
2. **Baseline module-operator RBAC** -- every module operator carries CRD RBAC,
   and every module that exposes `/metrics` carries protected-metrics markers.
3. **Deployed operand** -- the operator SA must hold every permission granted by
   ClusterRoles (and Roles) in the kustomize build, or deploy fails with RBAC
   escalation errors.

## Adding a Workload Resource Type

When a manifest in `config/manifests/` introduces a new resource type (e.g.,
adding a `NetworkPolicy` YAML), three things must be updated in lockstep:

1. **Manifest** -- the kustomize resource in `config/manifests/mymodule/base/`
2. **`Owns()`** -- register on the ReconcilerFor builder so changes trigger
   reconciliation:
   ```go
   Owns(&networkingv1.NetworkPolicy{})
   ```
3. **RBAC marker** -- grant permissions so the operator can manage it:
   ```go
   // +kubebuilder:rbac:groups=networking.k8s.io,resources=networkpolicies,verbs=get;list;watch;create;update;patch;delete
   ```

Missing any one of the three causes:
- No `Owns()` -- changes to the resource don't trigger reconciliation
- No RBAC marker -- `Forbidden` errors when deploying or garbage collecting
- No manifest -- the resource is never created (but `Owns` watch is harmless)

For every `Owns()` or `OwnsGVK()`, add a corresponding marker:
```go
// +kubebuilder:rbac:groups=$GROUP,resources=$RESOURCE,verbs=get;list;watch;create;update;patch;delete
```

After changes: `make manifests generate` to regenerate `config/rbac/role.yaml`.

## Baseline Module RBAC

Every split module operator keeps a small baseline RBAC set in addition to the
resource-specific markers derived from `Owns()`, `Watches()`, and operand
ClusterRoles.

**CRDs** -- every module operator includes:
```go
// +kubebuilder:rbac:groups=apiextensions.k8s.io,resources=customresourcedefinitions,verbs=get;list;watch;create;update;patch;delete
```

**Protected metrics** -- if the manager exposes `/metrics`, keep:
```go
// +kubebuilder:rbac:groups=authentication.k8s.io,resources=tokenreviews,verbs=create
// +kubebuilder:rbac:groups=authorization.k8s.io,resources=subjectaccessreviews,verbs=create
// +kubebuilder:rbac:urls=/metrics,verbs=get
```

These markers are baseline module-operator conventions. Do not wait for the
manifest audit to discover them -- `kustomize build` only reveals operand
RBAC, not the controller's protected-metrics endpoint.

After adding or changing markers, run `make manifests generate` so
`config/rbac/role.yaml` stays in sync.
