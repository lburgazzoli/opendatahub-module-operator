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

4. **Monolith Owns not in build.** Drop only if confirmed upstream removed
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

Fetched tree contains multiple overlays. At audit time, pick overlay from
`platform-type` in `config/manager/configmap.yaml` — same mapping as monolith
`ManifestsSourcePath` / module `NewModule`:

| `platform-type` | Overlay |
|-----------------|---------|
| `OpenDataHub` | `overlays/odh` |
| `SelfManagedRhoai`, `ManagedRhoai` | `overlays/rhoai` |

**Optional (adversarial review, not blocking):** run a second
`kustomize build` for the other overlay and note Kind/RBAC diffs.

---

## B. Commands

Run from `modules/$MODULE_NAME/` after `make get-manifests`.

Set path from extraction (examples):

```bash
# Ray — fixed overlay
KUSTOMIZE_PATH="config/manifests/ray/openshift"

# Spark — platform map (ODH default in configmap)
KUSTOMIZE_PATH="config/manifests/sparkoperator/overlays/odh"
```

Gate — build must succeed:

```bash
kustomize build "${KUSTOMIZE_PATH}" >/dev/null
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
| Deployed **ClusterRole** (and **Role**) rules | Add matching `+kubebuilder:rbac` on module operator |
| Monolith **Owns** not in build | Drop only if confirmed removed upstream; else flag |

Then:

```bash
make manifests generate
```

Verify generated `config/rbac/role.yaml` includes operand permissions.

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
- [ ] Every Kind in output has `Owns` / `OwnsGVK` (except documented CRD/Namespace)
- [ ] Every ClusterRole rule in output has matching operator `+kubebuilder:rbac`
- [ ] Monolith Watches ported unchanged
- [ ] `make manifests generate` run after marker updates
