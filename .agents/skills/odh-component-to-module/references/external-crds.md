# External CRDs

## OpenShift (default test target)

When integration/e2e run on **OpenShift**, OpenShift API types (SCC, Route,
ConsoleLink, etc.) are **already registered** on the cluster. You do **not**
need to fetch or install external CRDs for tests.

If the controller `OwnsGVK(gvk.SecurityContextConstraints)` (or Route,
ConsoleLink):

- **On OpenShift:** no extra setup — reconcile and tests proceed normally.
- **Do not** add `fetch-external-crds` to `test-integration` for OpenShift-only
  workflows.
- **Do not** block on `kind-setup.sh` external CRD install.

Still add `OwnsGVK` + RBAC markers in the controller — the operator manages
real SCC objects on OpenShift.

## Kind / vanilla Kubernetes (optional, not default)

Some components own OpenShift-specific resource types that don't exist on
vanilla Kubernetes. Only if you explicitly test on Kind:

- `SecurityContextConstraints` (security.openshift.io)
- `Route` (route.openshift.io)
- `ConsoleLink` (console.openshift.io)

### Script (Kind only)

`hack/scripts/fetch-external-crds.sh` — generates CRDs from Go module cache
using controller-gen. Copy from the ray module and customize `fetch_crds`
calls.

```bash
# SecurityContextConstraints (ray, datasciencepipelines)
fetch_crds "github.com/openshift/api" "security/v1" "securitycontextconstraints"

# Route (dashboard, mlflowoperator)
fetch_crds "github.com/openshift/api" "route/v1"

# ConsoleLink (dashboard, mlflowoperator)
fetch_crds "github.com/openshift/api" "console/v1" "consolelinks"
```

Output: `config/crd/external/` (tracked in git, not in kustomize deploy).

### Kind setup (Kind only)

`kind-setup.sh` can install external CRDs after cert-manager. Skip this path
when the team standard is OpenShift-only testing.

### Makefile (Kind only)

```makefile
.PHONY: fetch-external-crds
fetch-external-crds: ## Fetch external CRDs for Kind testing (not needed on OpenShift).
	./hack/scripts/fetch-external-crds.sh

test-integration: manifests generate fetch-external-crds cleanup-integration test-integration-run
```

## Decision table

| Controller owns | OpenShift tests | Kind tests |
|-----------------|-----------------|------------|
| SCC / Route / ConsoleLink | Nothing extra | `fetch-external-crds` + kind install |
| No OpenShift types | Nothing extra | Nothing extra |

## If not needed (any cluster)

If the component doesn't own any OpenShift-specific resources:

- Don't create `fetch-external-crds.sh`
- Don't add `fetch-external-crds` to the Makefile
- Don't add external CRD install to `kind-setup.sh`
