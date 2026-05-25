# External CRDs

## OpenShift (default test target)

When integration/e2e run on **OpenShift**, OpenShift API types (SCC, Route,
ConsoleLink, etc.) are **already registered** on the cluster. You do **not**
need to fetch or install external CRDs for tests.

If the controller `OwnsGVK(gvk.SecurityContextConstraints)` (or Route,
ConsoleLink):

- **On OpenShift:** no extra setup — reconcile and tests proceed normally.
- **Do not** add extra external-CRD helper targets to `test-integration`.
- **Do not** add helper scripts that synthesize OpenShift APIs for tests.

Still add `OwnsGVK` + RBAC markers in the controller — the operator manages
real SCC objects on OpenShift.

## Unsupported Test Strategy

Do not add a fallback path that tries to emulate OpenShift APIs on plain
Kubernetes. If a controller owns SCC, Route, ConsoleLink, or other
OpenShift-specific resources, the supported answer is to run integration/e2e
against OpenShift.

## If not needed (any cluster)

If the component doesn't own any OpenShift-specific resources:

- Don't create extra external-CRD helper scripts
- Don't add extra external-CRD helper targets to the Makefile
- Don't add synthetic API-install steps to test setup
