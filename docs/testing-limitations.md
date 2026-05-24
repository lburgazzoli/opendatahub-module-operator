# Testing

## Default: OpenShift

Integration and e2e tests for **split modules** under `modules/` target
**OpenShift** (CRC, ROSA, shared dev cluster). Prerequisites:

- `oc login` or kubeconfig pointing at OpenShift
- OpenShift APIs (SCC, etc.) available natively
- Registry reachable from the cluster for e2e image pulls

Use the skill testing guide:
`.agents/skills/odh-component-to-module/references/testing.md`

E2e/container workflow:
`.agents/skills/odh-component-to-module/references/e2e-workflow.md`

## Kind (optional, not default)

Kind is supported for the **root reference operator** local dev only.
OpenShift-specific resources (SCC) do not reconcile on plain Kind — see
[testing-limitations.md](testing-limitations.md).

```sh
make kind-create          # Optional local cluster
make test-integration     # Root reference only
make kind-delete
```

Split modules should be tested on OpenShift, not Kind.

## SCC on Kind clusters

Components that deploy `SecurityContextConstraints` (e.g., ray) fail on Kind
because the OpenShift SCC controller is not present. The CRD schema enforces
required fields that the SCC controller normally handles on OpenShift.

**Workaround options (Kind only):**
1. Run integration/e2e tests on OpenShift (recommended)
2. Use a kustomize overlay that excludes the SCC for Kind testing
3. Strip SCC resources from the kustomize output at test time

**Affected components:** ray, datasciencepipelines, any component with SCC in
its manifests.
