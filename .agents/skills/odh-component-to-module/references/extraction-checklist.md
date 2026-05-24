# Extraction Checklist

When reading monolith source, record each item below. These become the
inputs for porting.

## From `${component}_controller.go` (or `${component}_controller_actions.go`)

- [ ] **Owns list**: every `Owns()` call with type and predicates
- [ ] **Watches list**: every `Watches()` and `WatchesGVK()` with predicates and handlers
- [ ] **Action pipeline**: every `WithAction()` in order
- [ ] **Conditions**: the `WithConditions()` arguments
- [ ] **Preconditions**: any `WithPreCondition()` calls

## From `${component}_support.go` (or constants at top of files)

- [ ] **ComponentName**: the constant (e.g., `"ray"`)
- [ ] **LegacyComponentName**: if different from ComponentName
- [ ] **imageParamMap**: the full map contents
- [ ] **conditionTypes**: the slice contents
- [ ] **manifestPath function**: overlay path, context dir

## From `${component}.go` (the handler)

- [ ] **Init()**: what it does at startup (usually `odhdeploy.ApplyParams` with imageParamMap)
- [ ] **initialize()**: what it does per-reconcile (usually appends manifests + applies namespace params)
- [ ] **Custom actions**: any action functions (setKustomizedParams, checkPreConditions, etc.)

## From `${component}_types.go` (API)

- [ ] **Kind**: the CRD Kind string
- [ ] **InstanceName**: the singleton name
- [ ] **Spec fields**: any component-specific spec fields
- [ ] **Status fields**: any component-specific status fields beyond common.Status

## From `get_all_manifests.sh`

- [ ] **ODH entry**: repo-org:repo-name:ref@commit:source-folder
- [ ] **RHOAI entry**: same format
