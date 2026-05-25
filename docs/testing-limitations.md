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

## Unsupported Cluster Types

Supported integration and e2e testing assumes a real **OpenShift** API surface.
Plain Kubernetes environments that do not provide OpenShift resources are not a
supported validation path for this repo.

In particular, components that deploy `SecurityContextConstraints` (for example
ray) require OpenShift behavior that is not present on non-OpenShift clusters.
Use CRC, ROSA, or another connected OpenShift cluster instead of trying to
approximate these APIs locally.
