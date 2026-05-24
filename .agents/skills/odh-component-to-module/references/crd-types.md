# CRD Types

## Source

Copy from monolith:
```
opendatahub-operator/api/components/v1alpha1/${component}_types.go
```

The primary CRD type file must come from the **target component** in the
monolith, not from the copied ray/template module. After scaffolding, the
module must define its **own** `${Kind}` CRD schema, singleton name, print
columns, and generated CRD metadata.

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

### CRD identity rule

After renaming, every CRD-facing identifier must match the new module:

- Go type names (`${Kind}`, `${Kind}Spec`, `${Kind}Status`) — not `Ray*`
- Singleton validation (`default-$COMPONENT`) — not `default-ray`
- `+kubebuilder:resource` names/singular/path/short names for this component
- Generated CRD filenames and metadata under `config/crd/bases/`
- Helm chart CRD templates for this module, if present

If any of those still describe the template source component, the module does
**not** contain its own CRD yet.

### Do NOT include

- `status.configValues` — that was a template example, not for real modules

## Supporting files

These are copied from the reference module (not component-specific):
- `groupversion_info.go` — scheme builder + group/version constants
- `semver.go` — SemVer type
- `status_types.go` — ModuleStatus, PlatformStatus, SourceStatus
