# Testing Limitations

## SCC on Kind clusters

Components that deploy `SecurityContextConstraints` (e.g., ray) fail on Kind
because the OpenShift SCC controller is not present. The CRD schema enforces
required fields (`allowHostDirVolumePlugin`, `allowHostIPC`, etc.) that the SCC
controller normally handles.

**Workaround options:**
1. Run integration/e2e tests on an OpenShift cluster (CRC, rosa, etc.)
2. Use a dedicated kustomize overlay that excludes the SCC for Kind testing
3. Strip SCC resources from the kustomize output at test time

**Affected components:** ray, datasciencepipelines, any component with SCC in
its manifests.
