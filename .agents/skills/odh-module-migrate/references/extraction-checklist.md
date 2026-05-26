# Extraction Checklist

When reading monolith source, record each item below. These become the
inputs for porting.

## From `${component}_controller.go` (or `${component}_controller_actions.go`)

- [ ] **Owns list**: every `Owns()` call with type and predicates
- [ ] **Watches list**: every `Watches()` and `WatchesGVK()` with predicates and handlers
- [ ] **Action pipeline**: every `WithAction()` in order
- [ ] **Conditions**: the `WithConditions()` arguments
- [ ] **Preconditions**: any `WithPreCondition()` calls
- [ ] **Dependency checks**: identify the concrete dependency resources:
  - [ ] required operator/controller CRDs
  - [ ] required singleton/operator CR instances
  - [ ] required operand CRD types
- [ ] **Operator-level checks to replace**: note any `OperatorExists`,
      `MonitorOperator`, Subscription, CSV, or similar checks that should be
      re-expressed as type/instance checks in the module

## From `${component}_support.go` (or constants at top of files)

- [ ] **ComponentName**: the constant (e.g., `"ray"`)
- [ ] **LegacyComponentName**: if different from ComponentName
- [ ] **imageParamMap**: the full map contents
- [ ] **conditionTypes**: the slice contents
- [ ] **manifestPath function**: overlay path, context dir
  - [ ] **ContextDir** (e.g. `ray`, `sparkoperator`)
  - [ ] **SourcePath** (e.g. `openshift` or `overlays/odh`)
  - [ ] **Overlay type**: fixed vs platform-map
  - [ ] **ODH/RHOAI fetch pins** from `get_all_manifests.sh`
  - [ ] **Kustomize audit path** verified after `make get-manifests`:
        `config/manifests/${ContextDir}/${SourcePath}`

## From `${component}.go` (the handler)

- [ ] **Init()**: what it does at startup (usually `odhdeploy.ApplyParams` with imageParamMap)
- [ ] **initialize()**: what it does per-reconcile (usually appends manifests + applies namespace params)
- [ ] **Custom actions**: any action functions (setKustomizedParams, checkPreConditions, etc.)

## Dependency-check rewrite rule

- [ ] For every dependency, prefer **CRD + CR instance + operand CRD** checks
      over generic "operator installed" checks whenever the concrete resources
      exist and can be queried directly

## From `${component}_types.go` (API)

- [ ] **Kind**: the CRD Kind string
- [ ] **InstanceName**: the singleton name
- [ ] **Spec fields**: any component-specific spec fields
- [ ] **Status fields**: any component-specific status fields beyond common.Status

## From `get_all_manifests.sh`

- [ ] **ODH entry**: repo-org:repo-name:ref@commit:source-folder
- [ ] **RHOAI entry**: same format

## From module scaffold (must not rename)

When copying from the ray module, these values come from the example module
and must **not** change during component rename:

- [ ] **EnvPrefix** = `"ODH_MODULE_OPERATOR"` in `pkg/config/config.go`
- [ ] **ConfigPathEnvVar** = `"ODH_MODULE_OPERATOR_CONFIGURATION_PATH"`
- [ ] **Deployment env vars** use `ODH_MODULE_OPERATOR_*` prefix
- [ ] **Makefile `run`** uses `ODH_MODULE_OPERATOR_MANIFESTS_PATH`
