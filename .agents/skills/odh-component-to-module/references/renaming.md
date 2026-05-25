# Renaming Guide

After copying the ray module, apply these substitutions. Use `find` + `sed`
or editor search-replace. Check every file — Go source, Makefile, configs,
scripts, PROJECT, Containerfile.

## Go module path (go.mod, all imports)

```
github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-ray-operator
→
github.com/lburgazzoli/opendatahub-module-operator/modules/$MODULE_NAME
```

## Package names

```
package ray → package $COMPONENT
```

Rename all files: `ray.go` → `${component}.go`, `ray_controller.go` →
`${component}_controller.go`, etc.

Also rename the directory: `internal/controller/ray/` → `internal/controller/$COMPONENT/`

## Type names

```
Ray → $KIND
RaySpec → ${KIND}Spec
RayStatus → ${KIND}Status
RayList → ${KIND}List
RayComponentName → ${KIND}ComponentName
RayInstanceName → ${KIND}InstanceName
RayKind → ${KIND}Kind
```

## Constants

```
"ray" → "$COMPONENT"                    (component name)
"default-ray" → "default-$COMPONENT"    (instance name)
"Ray" → "$KIND"                         (kind string)
```

## CRD validation rule

```
self.metadata.name == 'default-ray' → self.metadata.name == 'default-$COMPONENT'
Ray name must be default-ray → $KIND name must be default-$COMPONENT
```

## Makefile / scripts / configs

```
opendatahub-ray-operator → $MODULE_NAME
```

Apply `$MODULE_NAME` to: image name, Helm release, namespace, Kind cluster
name, leader election ID, ConfigMap name, `app.kubernetes.io/name` label.

The env var prefix is **`ODH_MODULE_OPERATOR_`** for ALL modules — same as
the example module. Do NOT include the component name. This should NOT be
renamed when copying a module to a new component.

**Do-not-rename / forbidden prefixes:**

| Prefix | Action |
|--------|--------|
| `ODH_MODULE_OPERATOR_` | DO NOT CHANGE (not part of module rename) |
| `ODH_OPERATOR_` | FORBIDDEN (legacy drift) |
| `ODH_RAY_OPERATOR_`, `ODH_SPARK_OPERATOR_`, etc. | FORBIDDEN (component-specific) |

After all renames, verify env prefix and template leftovers per
[verification-gates.md](verification-gates.md).

Any match means rename is incomplete. Examples of bugs this catches:
- Spark module still importing `modules/ray/api/...`
- `PROJECT` repo still pointing at `modules/ray`
- RBAC or sample files still named `components_ray_*`
- Test fixtures still using `RayInstanceName` or `default-ray`
- Cleanup scripts still deleting `rays.components...` or `part-of=ray`

See [adversarial-review.md](adversarial-review.md) step **9b** for the full
adversarial rename consistency review.

## PROJECT file

Update domain, group, kind, version to match the new CRD.
