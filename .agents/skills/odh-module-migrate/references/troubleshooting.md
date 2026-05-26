# In-Cluster Troubleshooting

When the operator fails in-cluster, diagnose before rebuilding:

1. **Check pod status**: `kubectl get pods -n $NS` — look for CrashLoopBackOff, OOMKilled
2. **Check exit code**: `kubectl describe pod -n $NS $POD` — exit code 137 = OOMKilled
3. **Check logs**: `kubectl logs -n $NS deployment/$DEPLOY -c manager --tail=20`
4. **Patch instead of rebuild**: fix issues in-cluster first to verify the solution:
   - Memory: `kubectl patch deployment -n $NS $DEPLOY --type=json -p='[{"op":"replace","path":"/spec/template/spec/containers/0/resources/limits/memory","value":"512Mi"}]'`
   - RBAC: `kubectl patch clusterrole $ROLE --type=json -p='[{"op":"add","path":"/rules/-","value":{...}}]'`
   - Image: `kubectl set image -n $NS deployment/$DEPLOY manager=$NEW_IMAGE`
5. **Then fix in code**: once the patch works, update the source files and rebuild

## Common in-cluster errors

| Error | Cause | Fix |
|-------|-------|-----|
| `exit code 137` | OOMKilled — increase memory limit | Increase `resources.limits.memory` in manager.yaml |
| `is not cached` | Cache has `ReaderFailOnMissingInformer: true` and the resource type has no informer | Add `Owns()` or `OwnsGVK()` for that resource type |
| `is forbidden: attempting to grant RBAC permissions not currently held` | The operator SA tries to create a ClusterRole granting permissions it doesn't have | Add those permissions to the operator's RBAC markers — the SA must hold ALL permissions that any deployed ClusterRole grants |
| `Permission denied` on manifest files | Container runs as arbitrary UID (OpenShift) | Set `chmod -R a+rX` in builder stage, use init container with emptyDir for writable copy |
| `field not declared in schema` | CRD on cluster doesn't match module's CRD (e.g., missing `.status.module`) | `InstallCRDs` must use Get+Update pattern to replace existing CRDs |
| `metrics on wrong port` / config ignored | Env var prefix doesn't match `EnvPrefix` in pkg/config | Align all env vars to `ODH_MODULE_OPERATOR_` (see [controller-rules.md](controller-rules.md)) |
| `tests hang / no output` | Wrong timeout, chained make hiding failure step, or waiting too long before inspecting the pod | Run build/push/deploy/test separately; start with `-timeout 5m -failfast`, then check operator logs immediately; see [testing.md](testing.md) |
| Logs mention `/mymodule-mutate-deploy` or the root operator when testing a module | `container-build` / `deploy-helm` ran from the repo root, so the root `opendatahub-module-operator` chart was installed instead of the module chart | Return to `modules/$MODULE_NAME/`, rebuild/push the image there, regenerate Helm there, and redeploy from that module directory |

## RBAC for operator-that-deploys-an-operator

When the kustomize output includes ClusterRoles (like kuberay-operator's role),
the module operator SA must hold ALL permissions those ClusterRoles grant.
Otherwise Kubernetes RBAC escalation prevention blocks the apply.

See [manifest-rbac-audit.md](manifest-rbac-audit.md) for the full procedure:
resolve `config/manifests/${ContextDir}/${SourcePath}`, list operand
ClusterRole rules, add matching `+kubebuilder:rbac` markers. Do NOT use `*/*`
wildcards — list each resource explicitly.
