---
name: odh-component-to-module
description: >
  Scaffold a new standalone module operator from an opendatahub-operator
  component. Use when creating modules/$name/ from the monolith source.
model: sonnet
user-invocable: true
allowed-tools:
  - Read
  - Glob
  - Grep
  - Bash
  - Write
  - Edit
  - Agent
---

# Scaffold Module Operator from Monolith

## Inputs

- `$COMPONENT`: monolith directory name (e.g., `ray`, `sparkoperator`)
- `$MODULE_NAME`: derived name (see `naming.md`)
- `$KIND`: CRD Kind (e.g., `Ray`, `SparkOperator`)

## Checklist

Work through each step sequentially. Reference docs are in this directory.

### 1. Read monolith source

Read ALL files in:
```
/home/luca/work/dev/openshift-ai/opendatahub-operator/internal/controller/components/$COMPONENT/
/home/luca/work/dev/openshift-ai/opendatahub-operator/api/components/v1alpha1/${COMPONENT}_types.go
```

Record findings per `extraction-checklist.md`.

### 2. Copy ray module and rename

```bash
cp -r modules/opendatahub-ray-operator/ modules/$MODULE_NAME/
```

Apply all renames per `renaming.md`.

### 3. Port controller

Write files per `controller-rules.md`. Key files:
- `${name}_controller.go` — wiring (Owns, Watches, pipeline, RBAC)
- `${name}.go` — Module struct, NewModule, initialize, reportStatus
- `${name}_actions.go` — custom actions (if any)
- `${name}_upgrade.go` — upgrade placeholder
- `${name}_webhook.go` — webhooks (if any)

### 4. Port CRD types

Per `crd-types.md`.

### 5. Create manifest script

Per `manifest-script.md`.

### 6. Set up external CRDs

Per `external-crds.md` (only if component owns OpenShift-specific resources).

### 7. Write tests

Per `testing.md`.

### 8. Build and verify

```bash
cd modules/$MODULE_NAME
go mod tidy
make manifests generate
make test
make lint
```

### 8b. Derive RBAC from kustomize output

Run kustomize build to discover all resource kinds and ClusterRole rules
the manifests deploy. The module operator's RBAC must cover all of these.

```bash
# List all resource kinds:
kustomize build config/manifests/$COMPONENT/$OVERLAY 2>/dev/null | yq e '.kind' - | sort -u

# Extract all ClusterRole rules (permissions the module SA must hold):
kustomize build config/manifests/$COMPONENT/$OVERLAY 2>/dev/null | \
  yq e 'select(.kind == "ClusterRole") | .rules[] | .apiGroups[] + "/" + (.resources[]) + " " + (.verbs | join(","))' - | \
  sort -u
```

Add RBAC markers for:
1. Every resource kind in the kustomize output (Owns + deploy needs full CRUD)
2. Every permission in every deployed ClusterRole (RBAC escalation prevention)

### 9. Adversarial review

Spawn a **clean-context subagent** using the prompt below. Do NOT summarize
the findings — read and address every difference it reports.

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
10. Monolith behavior NOT ported
11. Module behavior NOT in monolith

EXPECTED differences (do NOT flag these):
- Module adds upgradeIfNeeded action (after initialize)
- Module adds reportStatus action (after deployments)
- Module sets r.Release from config
- Module uses m.cfg.ApplicationsNamespace instead of cluster.ApplicationNamespace()
- Module uses gc.InNamespace(cfg.ApplicationsNamespace)
- Module has no DSC-specific types (NewCRObject, IsEnabled, UpdateDSCStatus)

Flag everything else with exact file:line references.
Be strict. Any difference could cause different runtime behavior."""
)
```

### 10. Fix findings and re-verify

Address all non-expected differences from the review, then re-run:
```bash
make test lint
make test-integration  # against real cluster
```

## Troubleshooting (in-cluster)

When the operator fails in-cluster, diagnose before rebuilding:

1. **Check pod status**: `kubectl get pods -n $NS` — look for CrashLoopBackOff, OOMKilled
2. **Check exit code**: `kubectl describe pod -n $NS $POD` — exit code 137 = OOMKilled
3. **Check logs**: `kubectl logs -n $NS deployment/$DEPLOY -c manager --tail=20`
4. **Patch instead of rebuild**: fix issues in-cluster first to verify the solution:
   - Memory: `kubectl patch deployment -n $NS $DEPLOY --type=json -p='[{"op":"replace","path":"/spec/template/spec/containers/0/resources/limits/memory","value":"512Mi"}]'`
   - RBAC: `kubectl patch clusterrole $ROLE --type=json -p='[{"op":"add","path":"/rules/-","value":{...}}]'`
   - Image: `kubectl set image -n $NS deployment/$DEPLOY manager=$NEW_IMAGE`
5. **Then fix in code**: once the patch works, update the source files and rebuild

### Common in-cluster errors

| Error | Cause | Fix |
|-------|-------|-----|
| `exit code 137` | OOMKilled — increase memory limit | Increase `resources.limits.memory` in manager.yaml |
| `is not cached` | Cache has `ReaderFailOnMissingInformer: true` and the resource type has no informer | Add `Owns()` or `OwnsGVK()` for that resource type |
| `is forbidden: attempting to grant RBAC permissions not currently held` | The operator SA tries to create a ClusterRole granting permissions it doesn't have | Add those permissions to the operator's RBAC markers — the SA must hold ALL permissions that any deployed ClusterRole grants |
| `Permission denied` on manifest files | Container runs as arbitrary UID (OpenShift) | Set `chmod -R a+rX` in builder stage, use init container with emptyDir for writable copy |
| `field not declared in schema` | CRD on cluster doesn't match module's CRD (e.g., missing `.status.module`) | `InstallCRDs` must use Get+Update pattern to replace existing CRDs |

### RBAC for operator-that-deploys-an-operator

When the kustomize output includes ClusterRoles (like kuberay-operator's role),
the module operator SA must hold ALL permissions those ClusterRoles grant.
Otherwise Kubernetes RBAC escalation prevention blocks the apply.

Run `kustomize build config/manifests/$COMPONENT/$OVERLAY` and inspect all
ClusterRole resources. Add every API group/resource/verb to the module
operator's RBAC markers. Do NOT use `*/*` wildcards — list each resource
explicitly.
