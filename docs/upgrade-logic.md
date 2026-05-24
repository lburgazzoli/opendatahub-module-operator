# Upgrade Logic

Source: `/home/luca/work/dev/openshift-ai/opendatahub-operator/pkg/upgrade/`

## Global Upgrades (run on every operator startup)

`CleanupExistingResource()` in `upgrade.go` runs these migrations:

| Migration | Affects | What it does |
|-----------|---------|-------------|
| Deprecated RoleBinding cleanup | Global | Removes legacy default RoleBindings |
| ModelController selector fix | modelcontroller | Deletes stale Deployment with immutable selector (v2.16→v2.17) |
| Kueue VAP cleanup | kueue | Removes deprecated ValidatingAdmissionPolicyBinding |
| AcceleratorProfile→HardwareProfile | dashboard, kserve, workbenches | Migrates AP resources to HP, updates Notebook/ISVC annotations |
| GatewayConfig ingressMode | Global | Preserves LoadBalancer mode for existing deployments |

## Per-Component Upgrade Mapping

For each module, copy only the relevant migrations:

| Module | Needs from upgrade.go |
|--------|----------------------|
| trainingoperator | None |
| ray | None |
| sparkoperator | None |
| feastoperator | None |
| ogx | None |
| mlflowoperator | None |
| trustyai | None |
| trainer | None |
| datasciencepipelines | None |
| modelregistry | None |
| modelcontroller | ModelController selector fix (deployment deletion) |
| kserve | AcceleratorProfile→HardwareProfile (ISVC annotations) |
| kueue | Kueue VAP cleanup |
| dashboard | AcceleratorProfile→HardwareProfile (AP resource migration) |
| workbenches | AcceleratorProfile→HardwareProfile (Notebook annotations) |
| modelsasservice | None |

## Module Upgrade Pattern

Each module uses the pattern from the reference implementation:

```go
func (m *Module) upgrade(ctx context.Context, prev componentApi.ModuleStatus, rr *odhtypes.ReconciliationRequest) error {
    // Version-gated migrations
    // Direct API calls to amend existing resources before new manifests apply
    return nil
}
```
