# Manifest RBAC Audit

Mandatory after `make get-manifests`. Reconciles the controller's `Owns()` /
`+kubebuilder:rbac` markers against what kustomize actually deploys.

## Core rules

1. **Own everything kustomize deploys (except CRDs).** Every resource **Kind**
   in the `kustomize build` output must have a matching `Owns()` or
   `OwnsGVK()` on the module reconciler. Without this,
   `ReaderFailOnMissingInformer` fails at runtime with "is not cached".

   **Exceptions (document if used):**
   - `CustomResourceDefinition` — usually watched, not owned
   - `Namespace` — cluster-scoped or pre-existing; rarely owned

2. **RBAC for the deployed operand.** Every API group/resource/verb granted
   by **ClusterRoles in the build output** must appear on the **module
   operator** SA (via `+kubebuilder:rbac` markers). Kubernetes RBAC escalation
   prevention blocks the module from creating ClusterRoles that grant
   permissions the module SA does not hold.

   This is separate from Owns: Owns drives cache informers; operand RBAC
   drives whether deploy succeeds.

3. **Watches stay from the monolith.** Cross-component watches, CRD watches,
   and dynamic GVK watches are **not** inferable from kustomize — port them
   from `${component}_controller.go` verbatim.

4. **Baseline module-operator RBAC stays even if kustomize cannot infer it.**
   Every module operator keeps the CRD marker
   (`customresourcedefinitions get;list;watch;create;update;patch;delete`).
   If the manager exposes `/metrics`, also keep the protected-metrics markers
   for `tokenreviews`, `subjectaccessreviews`, and `urls=/metrics`. These are
   baseline controller requirements, not operand-RBAC discoveries from
   `kustomize build`.

5. **Monolith Owns not in build.** Drop only if confirmed upstream removed
   the resource; otherwise flag in adversarial review.

After updating markers and Owns, run `make manifests generate`.

---

## A. Resolve kustomize path

From step 1 extraction (`manifestPath` / `ManifestsSourcePath` in monolith
`${component}_support.go`):

| Field | Example ray | Example spark |
|-------|-------------|---------------|
| `ContextDir` | `ray` | `sparkoperator` |
| `SourcePath` | `openshift` (fixed) | `overlays/odh` or `overlays/rhoai` (platform map) |

**Build path:** `config/manifests/${ContextDir}/${SourcePath}`

### Fixed overlay (ray)

`SourcePath` is always the same (e.g. `openshift`). ODH vs RHOAI differs at
**fetch time** (`ODH_PLATFORM_TYPE` when running `make get-manifests`).

### Platform map (spark, ogx, dsp, …)

Fetched tree contains multiple overlays. Start with the overlay selected by
`platformType` in `config/manager/configmap.yaml` — same mapping as monolith
`ManifestsSourcePath` / module `NewModule`:

| `platformType` | Overlay |
|-----------------|---------|
| `OpenDataHub` | `overlays/odh` |
| `SelfManagedRhoai`, `ManagedRhoai` | `overlays/rhoai` |

For RBAC/permission review, this is **not enough by itself**. If the module has
multiple overlays, run `kustomize build` against **every overlay** in the
fetched tree and audit Kind/RBAC differences across all of them. Do not stop at
the configmap default overlay; the operator must hold permissions for any
operand RBAC it may deploy on every supported platform.

---

## B. Commands

Run from `modules/$MODULE_NAME/` after `make get-manifests`.

Set path from extraction (examples):

```bash
# Ray — fixed overlay
KUSTOMIZE_PATH="config/manifests/ray/openshift"

# Spark — platform map (ODH default in configmap)
KUSTOMIZE_PATH="config/manifests/sparkoperator/overlays/odh"

# Spark — additional overlay(s) that must also be audited for RBAC/permissions
KUSTOMIZE_PATH_ALT="config/manifests/sparkoperator/overlays/rhoai"
```

Gate — build must succeed:

```bash
kustomize build "${KUSTOMIZE_PATH}" >/dev/null

# If the module has multiple overlays, every overlay must build cleanly.
kustomize build "${KUSTOMIZE_PATH_ALT}" >/dev/null
```

List deployed kinds:

```bash
kustomize build "${KUSTOMIZE_PATH}" | yq e '.kind' - | sort -u
```

List permissions the **operand** ClusterRoles grant (module SA must hold these):

```bash
kustomize build "${KUSTOMIZE_PATH}" | yq e \
  'select(.kind == "ClusterRole") | .rules[] | .apiGroups[] + "/" + (.resources[]) + " " + (.verbs | join(","))' - \
  | sort -u
```

Also check **Role** resources in namespaced operand RBAC if present:

```bash
kustomize build "${KUSTOMIZE_PATH}" | yq e \
  'select(.kind == "Role") | .rules[] | .apiGroups[] + "/" + (.resources[]) + " " + (.verbs | join(","))' - \
  | sort -u
```

---

## C. Update controller from build output

| Source | Update in `${component}_controller.go` |
|--------|----------------------------------------|
| Each **Kind** in build (except CRD, Namespace) | `Owns()` or `OwnsGVK()` — OpenShift types via GVK per [controller-rules.md](controller-rules.md) |
| Monolith **Watches** / cross-deps | Keep from monolith — not inferable from kustomize |
| Baseline module-operator RBAC | Keep CRD RBAC on every module; keep protected-metrics RBAC whenever `/metrics` is exposed |
| Deployed **ClusterRole** (and **Role**) rules | Add matching `+kubebuilder:rbac` on module operator |
| Monolith **Owns** not in build | Drop only if confirmed removed upstream; else flag |

Then:

```bash
make manifests generate
```

Verify generated `config/rbac/role.yaml` includes operand permissions.

When multiple overlays exist, compare the Kind set and RBAC rules from each
overlay and size the operator's `Owns` / `+kubebuilder:rbac` markers for the
union, not just the default overlay.

---

## D. Kind → Owns mapping

| Kustomize Kind | Controller |
|----------------|------------|
| Deployment | `Owns(&appsv1.Deployment{}, reconciler.WithPredicates(predicates.DefaultDeploymentPredicate))` |
| Service | `Owns(&corev1.Service{})` |
| ConfigMap | `Owns(&corev1.ConfigMap{})` |
| Secret | `Owns(&corev1.Secret{})` |
| ServiceAccount | `Owns(&corev1.ServiceAccount{})` |
| Role | `Owns(&rbacv1.Role{})` |
| RoleBinding | `Owns(&rbacv1.RoleBinding{})` |
| ClusterRole | `Owns(&rbacv1.ClusterRole{})` |
| ClusterRoleBinding | `Owns(&rbacv1.ClusterRoleBinding{})` |
| SecurityContextConstraints | `OwnsGVK(gvk.SecurityContextConstraints)` |
| Route (OpenShift) | `OwnsGVK(gvk.Route)` |
| NetworkPolicy | `Owns(&networkingv1.NetworkPolicy{})` |
| CustomResourceDefinition | `Watches(&extv1.CustomResourceDefinition{}, ...)` — not Owns |

Add scheme registration in `cmd/operator/operator.go` for any API group not in
`clientgoscheme` (e.g. `apiextensionsv1` for CRD watches).

---

## E. Checklist

- [ ] `KUSTOMIZE_PATH` derived from extraction (`ContextDir` + `SourcePath`)
- [ ] `kustomize build "${KUSTOMIZE_PATH}"` succeeds
- [ ] If the module has multiple overlays, `kustomize build` succeeds for every
      overlay (for example both `overlays/odh` and `overlays/rhoai`)
- [ ] Every Kind in output has `Owns` / `OwnsGVK` (except documented CRD/Namespace)
- [ ] Baseline CRD marker present on the module operator
- [ ] If the module exposes `/metrics`, the protected-metrics markers are present
- [ ] Every ClusterRole rule in output has matching operator `+kubebuilder:rbac`
- [ ] Monolith Watches ported unchanged
- [ ] `make manifests generate` run after marker updates

## Multi-overlay audit

When the component has both `overlays/odh` and `overlays/rhoai`, run `kustomize build`
on **both** and take the union of resource kinds and RBAC rules:

```bash
OVERLAY_ODH=config/manifests/${COMPONENT}/overlays/odh
OVERLAY_RHOAI=config/manifests/${COMPONENT}/overlays/rhoai

# Combined kinds
{ kustomize build "${OVERLAY_ODH}" 2>/dev/null; kustomize build "${OVERLAY_RHOAI}" 2>/dev/null; } \
  | yq e '.kind' - | sort -u

# Combined RBAC rules (diff to find rhoai additions)
kustomize build "${OVERLAY_RHOAI}" 2>/dev/null | \
  yq e 'select(.kind == "ClusterRole" or .kind == "Role") | .rules[] | .apiGroups[] + "/" + .resources[] + " " + (.verbs | join(","))' - 2>/dev/null | \
  sort -u | diff - <(kustomize build "${OVERLAY_ODH}" 2>/dev/null | yq e 'select(.kind == "ClusterRole" or .kind == "Role") | .rules[] | .apiGroups[] + "/" + .resources[] + " " + (.verbs | join(","))' - 2>/dev/null | sort -u)
```
