# Implementation Plan — Module Operator Split

## Architecture

Each module is a standalone Go project under `modules/$name/` with:
- Its own `go.mod`, `Makefile`, `Containerfile`
- A `get-manifests.sh` fetching only its component manifests
- A `cmd/chartgen/` copied from the reference (only reusable piece)
- Local copy of its CRD types (from opendatahub-operator api/components/v1alpha1)
- Dependencies only on opendatahub-operator/v2 and odh-platform-utilities
- No inter-module Go dependencies

Cross-module awareness is achieved by watching CRDs or CRs via the Kubernetes
API, never by importing another module's Go types.

## Per-Module Structure

```
modules/$name/
  go.mod
  Makefile
  Containerfile
  get-manifests.sh              # fetches this component's manifests only
  api/components/v1alpha1/      # local CRD types (copied + adjusted)
  cmd/
    main.go
    operator/operator.go
    chartgen/                   # copied from reference
  internal/controller/$name/
    ${name}_controller.go       # ReconcilerFor pipeline (Owns, Watches, pipeline wiring)
    ${name}.go                  # Module struct, NewModule, initialize, reportStatus
    ${name}_actions.go          # Custom action functions (setKustomizedParams, etc.)
    ${name}_upgrade.go          # Upgrade logic (version-gated migrations)
    ${name}_webhook.go          # Webhook handlers (if component has webhooks)
    ${name}_test.go             # Unit tests
  pkg/config/config.go
  pkg/version/version.go
  pkg/cache/
  config/
    manifests/$name/            # symlink to opt/manifests/$name
    manager/
    crd/bases/
    rbac/
  test/
    integration/
    e2e/
    support/
```

Each concern lives in its own file so the logic is discoverable without reading
the entire controller. The `_controller.go` file is the wiring — it references
methods from the other files but contains no business logic itself.

## Design Principle: Minimize Dynamic Computation

Manifest paths, platform overlays, and image parameters are often fixed for
the lifetime of the process. Compute them once in `NewModule()` and store on
the Module struct — do NOT recompute them on every reconcile.

Pattern from the reference implementation:
```go
func NewModule(cfg *moduleconfig.Config) (*Module, error) {
    // Computed once, stored on struct
    mi := odhtypes.ManifestInfo{
        Path:       cfg.ManifestsPath,
        ContextDir: componentName,
        SourcePath: overlayODH,
    }
    if platform == SelfManagedRhoai {
        mi.SourcePath = overlayRhoai
    }
    return &Module{manifestInfo: mi, ...}, nil
}
```

The `initialize()` action just appends the pre-computed value:
```go
func (m *Module) initialize(_ context.Context, rr *odhtypes.ReconciliationRequest) error {
    rr.Manifests = append(rr.Manifests, m.manifestInfo)
    return nil
}
```

Apply this to all components: image parameters, kustomize variables, OIDC
URLs, gateway domains — anything that comes from config or build-time info
should be computed once in `NewModule()`, not re-derived on every reconcile.

## Design Principle: CRD Checks over Operator Checks

For preconditions that verify external dependencies, prefer checking **CRD
existence** over checking operator-specific resources (Subscriptions,
OperatorConditions, operator CRs). CRD checks are portable across
platforms (OpenShift, vanilla K8s, xKS) while operator checks are
OpenShift-specific.

Pattern:
```go
// Good: portable CRD existence check
precondition.MonitorCRDs("inferenceservices.serving.kserve.io")

// Avoid: OpenShift-specific operator check
precondition.MonitorOperator(subscriptionGVK, ...)
```

## Task Breakdown — Simple Components (Phase 1)

For each simple component, the work is:

### Task Template (repeat per component)

1. **Scaffold** — Copy reference module structure to `modules/$name/`
2. **CRD types** — Copy the component's types from opendatahub-operator, adjust package/imports
3. **get-manifests.sh** — Extract this component's entry from get_all_manifests.sh
4. **Controller** — Port the controller pipeline:
   - Map Owns/Watches from the monolith controller
   - Port action functions (initialize, kustomize params, etc.)
   - Port preconditions if any
   - Add RBAC markers matching the Owns list
5. **Upgrade** — Port relevant upgrade logic (most simple components: none)
6. **Webhooks** — Port if the component has webhooks (most simple: none)
7. **Config** — Set up config keys for the component
8. **Tests** — Unit tests for Module methods, integration test skeleton, e2e test skeleton
9. **Build** — Verify `make manifests generate test lint` passes

### Phase 1 Tasks — Simple Components (in order)

#### 1. ray
- Nearly as simple, one extra: CodeFlare sanity check (CRD must NOT exist)
- Pipeline: sanitycheck → initialize → releases → kustomize → deploy → deployments → gc
- Manifest: opendatahub-io/kuberay
- CRD: Ray / default-ray

#### 2. sparkoperator
- Simple, owns webhook configs (MWH/VWH) but no custom webhook logic
- Pipeline: initialize → releases → kustomize → deploy → deployments → gc
- Manifest: opendatahub-io/spark-operator
- CRD: SparkOperator / default-sparkoperator

#### 3. feastoperator
- Simple + OIDC issuer URL injection from gateway config
- Cross-dep: reads GatewayOIDCSpec (discovery via config, not Go import)
- Pipeline: initialize → setKustomizedParams → releases → kustomize → deploy → deployments → gc
- Manifest: opendatahub-io/feast

#### 4. ogx
- Simple + deprecation check (LlamaStackOperator must NOT be Managed)
- Cross-dep: needs to discover LlamaStackOperator state (CRD watch)
- Pipeline: initialize → checkPreConditions → releases → kustomize → deploy → deployments → gc
- Manifest: opendatahub-io/ogx-k8s-operator

#### 5. mlflowoperator
- Simple + gateway domain for URL construction
- Cross-dep: reads gateway domain (config/discovery)
- Owns ConsoleLink, ServiceMonitor, MLflow CRD
- Pipeline: initialize → setKustomizedParams → releases → kustomize → deploy → deployments → gc
- Manifest: opendatahub-io/mlflow-operator

#### 6. trustyai
- Precondition: InferenceServices CRD must exist (KServe dependency)
- Creates trustyai-dsc-config ConfigMap with eval permissions
- Pipeline: checkPreConditions → initialize → createConfigMap → releases → kustomize → deploy → deployments → gc
- Manifest: opendatahub-io/trustyai-service-operator

#### 7. trainer
- Precondition: JobSet operator must exist and be healthy
- Owns ClusterTrainingRuntime (dynamic GVK)
- Pipeline: checkPreConditions → initialize → checkJobSetCRD → releases → kustomize → deploy → deployments → gc
- Manifest: opendatahub-io/trainer

### Phase 2 Tasks — Medium Components

#### 8. datasciencepipelines
- Argo Workflows CRD precondition
- Owns SCC (OpenShift)
- Pipeline: checkPreConditions → initialize → argoWorkflowsControllersOptions → releases → kustomize → deploy → deployments → gc

#### 9. modelregistry
- Gateway domain dependency, template rendering
- Creates registries namespace
- Pipeline: initialize → customizeManifests → releases → configureDependencies → template → kustomize → deploy → deployments → updateStatus → gc

#### 10. modelcontroller
- Depends on KServe management state (discover KServe CR)
- KEDA subscription watch for WVA
- Upgrade: selector migration (delete stale Deployment)
- Pipeline: initialize → checkSubscriptionDependencies → kustomize → deploy → deployments → gc

### Phase 3 Tasks — Complex Components

#### 11-15. kserve, kueue, dashboard, workbenches, modelsasservice
- These require detailed per-component analysis and likely further tuning
- Deferred to Phase 3

## Execution Notes

- Start each module by copying the reference `mymodule` structure
- Replace `mymodule` naming with the component name throughout
- The chartgen command is the only code reused from this repo — copy it as-is
- Each module gets its own CI, image build, Helm chart
- Test on **OpenShift** with `make test-integration` / `make test-e2e` per module
- Kind is optional for root reference only — see `docs/testing-limitations.md`
