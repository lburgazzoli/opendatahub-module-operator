# CRD Types

## Source

Copy from monolith:
```
opendatahub-operator/api/components/v1alpha1/${component}_types.go
```

## Changes from monolith

### Add ModuleStatus to the status struct

```go
type ${Kind}Status struct {
    common.Status                 `json:",inline"`
    common.ComponentReleaseStatus `json:",inline"`

    // Module reports the module operator's runtime information.
    Module ModuleStatus `json:"module,omitempty"`

    // ... keep any component-specific status fields from monolith
}
```

The `ModuleStatus` type is defined in `status_types.go` (copied from the
reference module).

### Remove DSC types

Delete these types — they are monolith-specific:
- `DSC${Kind}` (e.g., `DSCRay`)
- `DSC${Kind}Status` (e.g., `DSCRayStatus`)
- `${Kind}CommonSpec` and `${Kind}CommonStatus` if only used by DSC types

### Keep

- `PlatformObject` interface assertion: `var _ common.PlatformObject = (*${Kind})(nil)`
- All interface methods: `GetStatus`, `GetConditions`, `SetConditions`,
  `GetReleaseStatus`, `SetReleaseStatus`
- Singleton XValidation rule
- `+kubebuilder` markers (root, subresource, resource scope, printcolumn)
- `SchemeBuilder.Register` in `init()`
- Any component-specific Spec fields needed by the controller

### Do NOT include

- `status.configValues` — that was a template example, not for real modules

## Supporting files

These are copied from the reference module (not component-specific):
- `groupversion_info.go` — scheme builder + group/version constants
- `semver.go` — SemVer type
- `status_types.go` — ModuleStatus, PlatformStatus, SourceStatus
