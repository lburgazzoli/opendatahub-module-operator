# Verification Gates

Run these from `modules/$MODULE_NAME/` after rename and before/alongside
`make test`. All gates must pass.

## Env prefix (steps 2b, 8)

Every module uses **`ODH_MODULE_OPERATOR_`** — identical to the example module
in `pkg/config/config.go`.

```bash
rg 'ODH_OPERATOR[^_M]|ODH_OPERATOR_|ODH_[A-Z]+_OPERATOR_' . && exit 1 || true
rg 'EnvPrefix|ConfigPathEnvVar' pkg/config/config.go
# Expect: ODH_MODULE_OPERATOR
```

See [controller-rules.md](controller-rules.md) (Env prefix section) for the
full file sync list.

## Rename completeness (steps 2c, 8)

Step 2 copies the **ray** template. After [renaming.md](renaming.md), no
ray-era names, paths, or imports may remain (e.g. spark must not reference
`modules/ray` or `Ray` types).

```bash
# Template leftovers — must return no matches (exclude fetched manifests)
rg -n 'opendatahub-ray-operator|modules/ray/|internal/controller/ray\b|package ray\b' . \
  --glob '!config/manifests/**' && exit 1 || true

rg -n '\bRay\b|\bdefault-ray\b|components_v1alpha1_ray\b|components_ray_' . \
  --glob '!config/manifests/**' && exit 1 || true

rg -n 'LegacyComponentName = "ray"|Component\("ray"\)|"default-ray"|part-of.*\bray\b' . \
  --glob '!config/manifests/**' && exit 1 || true
```

Target names must appear in `go.mod`, `PROJECT`, `Makefile`, and
`pkg/config/config.go`. See step **9b** in [adversarial-review.md](adversarial-review.md).

## GVK import path (steps 3, 8)

Module controller and chartgen code must import the module-local
`pkg/resources/gvk/gvk.go` package. Direct imports of upstream
`pkg/cluster/gvk` are only allowed inside that wrapper package.

```bash
# Must be empty outside pkg/resources/gvk
rg -n 'opendatahub-operator/v2/pkg/cluster/gvk' . \
  --glob '!pkg/resources/gvk/**' && exit 1 || true

# The wrapper package itself must exist and be referenced
test -f pkg/resources/gvk/gvk.go
rg -n 'pkg/resources/gvk' internal cmd || exit 1
```

## GVK import path (steps 3, 8)

Module controller and chartgen code must import the module-local
`pkg/resources/gvk/gvk.go` package. Direct imports of upstream
`pkg/cluster/gvk` are only allowed inside that wrapper package.

```bash
# Must be empty outside pkg/resources/gvk
rg -n 'opendatahub-operator/v2/pkg/cluster/gvk' . \
  --glob '!pkg/resources/gvk/**' && exit 1 || true

# The wrapper package itself must exist and be referenced
test -f pkg/resources/gvk/gvk.go
rg -n 'pkg/resources/gvk' internal cmd || exit 1
```

## CRD identity (steps 4, 8)

The module must contain its **own** CRD, not the ray/template CRD that existed
right after the copy step.

- `api/components/v1alpha1/*_types.go` must define this component's Kind and
  singleton validation
- `config/crd/bases/` must generate this module's CRD name/path/kind
- If the chart vendors CRDs, the chart templates must match this module too

Concrete checks:

```bash
# No stale ray/template CRD identity in API types or generated CRDs
rg -n '\bRay\b|default-ray|components_v1alpha1_ray|components_ray_' \
  api/components config/crd config/chart/templates && exit 1 || true

# Sanity-check that the generated CRD mentions the target Kind/name instead
rg -n "$KIND|default-$COMPONENT|$MODULE_NAME" \
  api/components config/crd config/chart/templates || exit 1
```

## Build (step 8)

```bash
pwd   # must already be modules/$MODULE_NAME before module build/deploy targets
go mod tidy
make manifests generate
make test
make lint
```

## Cleanup and test targets (step 7)

```bash
grep -E '^test-integration:.*cleanup-integration' Makefile || exit 1
grep -E '^test-integration-run:' Makefile || exit 1
grep -E '^test-e2e:.*cleanup-e2e' Makefile || exit 1
grep -E '^test-e2e:.*deploy-helm' Makefile || exit 1
grep -E '^test-e2e-run:' Makefile || exit 1
test -x hack/scripts/cleanup-integration.sh
test -x hack/scripts/cleanup-e2e.sh
rg 'ray|opendatahub-ray' hack/scripts/cleanup-*.sh && exit 1 || true
```

Before `container-build`, `helm`, or `deploy-helm`, verify you are still in
`modules/$MODULE_NAME/`. The repo root has the same target names and will build
the wrong chart if you drift back there.

## Test timeouts (step 7 / 10)

Makefile integration/e2e targets must use short `-timeout` and `-failfast`:

```bash
rg '-timeout 30m' Makefile && exit 1 || true
rg '-timeout 5m' Makefile || exit 1
rg 'failfast' Makefile || exit 1
```

Test code should use `90s–2m` for `Eventually` `WithTimeout()`. See
[testing.md](testing.md) (Test timeouts).

## Manifest kustomize build (step 5b / 8)

After `make get-manifests`, resolve path from extraction checklist:

```bash
# Example ray (fixed overlay)
kustomize build config/manifests/ray/openshift >/dev/null

# Example spark (platform map — match configmap platform-type)
kustomize build config/manifests/sparkoperator/overlays/odh >/dev/null

# If the module has multiple overlays, build every overlay for RBAC/permission review
kustomize build config/manifests/sparkoperator/overlays/rhoai >/dev/null
```

Full audit procedure: [manifest-rbac-audit.md](manifest-rbac-audit.md).

Manual checks (or adversarial review):

- Every Kind in build output has `Owns()` / `OwnsGVK()` on the controller
  (except CRD and documented Namespace exceptions)
- Every rule in deployed operand ClusterRoles appears in operator
  `+kubebuilder:rbac` markers (escalation rule — see troubleshooting.md)
- If the module has multiple overlays, the Kind/RBAC audit covers the union of
  all overlay builds, not only the default overlay
