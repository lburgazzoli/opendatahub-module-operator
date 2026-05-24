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

The env var prefix is **`ODH_OPERATOR_`** for ALL modules — do NOT include
the component name. This is already set in the ray module and should NOT
be renamed when copying.

## PROJECT file

Update domain, group, kind, version to match the new CRD.
