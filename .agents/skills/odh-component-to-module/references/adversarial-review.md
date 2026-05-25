# Adversarial Reviews

Run **both** reviews after implementation. Do NOT summarize findings — read
and address every difference each subagent reports.

## Step 9 — Monolith behavior review

Spawn a **clean-context subagent**:

```
Agent(
  description="Adversarial review $COMPONENT module",
  prompt="""You are an adversarial reviewer. Compare the $COMPONENT module
operator against the monolith source to find behavioral differences.

Read ALL of these files:

MONOLITH (reference — behavior must match):
- /home/luca/work/dev/openshift-ai/opendatahub-operator/internal/controller/components/$COMPONENT/*.go
- /home/luca/work/dev/openshift-ai/opendatahub-operator/api/components/v1alpha1/${COMPONENT}_types.go

MODULE (what we're reviewing):
- /home/luca/work/dev/openshift-ai/opendatahub-module-operator/modules/$MODULE_NAME/internal/controller/$COMPONENT/*.go
- /home/luca/work/dev/openshift-ai/opendatahub-module-operator/modules/$MODULE_NAME/api/components/v1alpha1/${COMPONENT}_types.go

Compare these dimensions and report ALL differences:
1. Owns() — exact same resources? Same predicates?
2. Watches() — same watches? Same predicates? Same event handlers?
3. Action pipeline — same actions in same order? Any missing? Any extra?
4. Kustomize labels — same label keys and values?
5. Image params — same map? Applied at same lifecycle point?
6. Namespace resolution — how is the application namespace obtained?
7. Condition types — same conditions?
8. GC configuration — same options?
9. Manifest path — same overlay, context dir?
10. **Kustomize vs Owns vs RBAC** — after `make get-manifests`, every Kind
    in `kustomize build config/manifests/${ContextDir}/${SourcePath}` has
    matching Owns/OwnsGVK (except CRD/Namespace)? Every operand ClusterRole
    rule has a matching operator RBAC marker? Monolith Owns not in build?
11. Monolith behavior NOT ported
12. Module behavior NOT in monolith
13. Env prefix — pkg/config EnvPrefix/ConfigPathEnvVar, manager.yaml,
    chart deployment env, Makefile run target all use ODH_MODULE_OPERATOR_?
    Flag ODH_OPERATOR_, component-specific prefixes, or prefix mismatch
    between pkg/config and manifests (e.g. metrics patch vs config).

EXPECTED differences (do NOT flag these):
- Module adds upgradeIfNeeded action (immediately after initialize, with
  nothing in between)
- Module adds reportStatus action (after deployments)
- Module sets r.Release from config
- Module uses m.cfg.ApplicationsNamespace instead of cluster.ApplicationNamespace()
- Module uses gc.InNamespace(cfg.ApplicationsNamespace)
- Module has no DSC-specific types (NewCRObject, IsEnabled, UpdateDSCStatus)

Flag everything else with exact file:line references.
Be strict. Any difference could cause different runtime behavior."""
)
```

## Step 9b — Rename consistency review

Spawn a **second clean-context subagent** focused on stale template names,
imports, and identifiers:

```
Agent(
  description="Rename consistency review $MODULE_NAME",
  prompt="""You are an adversarial reviewer checking rename consistency for a
module operator scaffolded from the ray template.

TARGET (what this module MUST be):
- Component directory: $COMPONENT
- Module name: $MODULE_NAME
- CRD Kind: $KIND
- Go module path: github.com/lburgazzoli/opendatahub-module-operator/modules/$MODULE_NAME
- Controller package: internal/controller/$COMPONENT/
- Instance name: default-$COMPONENT (or ${KIND}InstanceName constant)

SOURCE TEMPLATE (must NOT appear anywhere except historical comments):
- opendatahub-ray-operator
- modules/ray/ import paths
- internal/controller/ray/
- package ray
- Ray, RaySpec, RayStatus, RayList, RayInstanceName, RayComponentName
- default-ray, components_v1alpha1_ray, components_ray_

Read ALL files under:
/home/luca/work/dev/openshift-ai/opendatahub-module-operator/modules/$MODULE_NAME/

Also read references/renaming.md for the full substitution list.

Check EVERY dimension and report ALL stale references with file:line:
1. go.mod module path — must be modules/$MODULE_NAME, not modules/ray
2. Go import paths — all must use modules/$MODULE_NAME/..., never modules/ray/
3. Package declarations — controller dir must be package $COMPONENT, not ray
4. Type names — CRD types, constants (${KIND}*, ${KIND}InstanceName), no Ray*
5. File and directory names — no ray_*.go, no internal/controller/ray/
6. PROJECT file — projectName, repo, kind, path match $MODULE_NAME and $KIND
7. Makefile / Containerfile — IMG, HELM_RELEASE, VERSION_PKG, leader election
   ID use $MODULE_NAME not opendatahub-ray-operator
8. K8s manifests — Namespace, Deployment labels, ConfigMap names, RBAC
   resourceNames, CRD filenames, sample YAML names use $MODULE_NAME / $KIND
9. Helm chart — Chart.yaml, values, template helpers, release name
10. Tests — fixtures reference ${KIND}InstanceName, correct workload Deployment
    names from this component's manifests (not kuberay-operator unless ray)
11. Hack scripts — get-manifests.sh fetches $COMPONENT manifests; cleanup scripts
    delete $KIND CRs / part-of=$COMPONENT; Makefile has test-integration-run,
    test-e2e-run, and composite test-integration / test-e2e targets
12. Stray string literals — "ray", "default-ray", opendatahub-ray-operator in
    comments is OK only if clearly documenting the template source; anywhere
    else is a bug

Run ripgrep across the module to find template leftovers:
  rg -n 'opendatahub-ray-operator|modules/ray/|internal/controller/ray|package ray' .
  rg -n '\\bRay\\b|default-ray|components_v1alpha1_ray|components_ray_' . \
    --glob '!config/manifests/**'
  rg -n 'LegacyComponentName = "ray"|Component\\("ray"\\)|part-of.*ray' . \
    --glob '!config/manifests/**'
  rg 'ray|opendatahub-ray' hack/scripts/cleanup-*.sh && echo "STALE CLEANUP SCRIPTS"

ALLOWED (do NOT flag):
- References to other components in cross-dep logic (e.g. CodeFlare watch on ray)
- Upstream manifest content under config/manifests/ (fetched, not renamed)
- docs/comments saying "copied from ray module" in skill-level docs only

Flag everything else. Be strict — a single stale import path breaks the build
or causes the wrong CRD to reconcile."""
)
```
