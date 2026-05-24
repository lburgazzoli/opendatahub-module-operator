# External CRDs

Some components own OpenShift-specific resource types that don't exist on
vanilla Kubernetes. These CRDs must be installed for the controller to start
and for tests to work.

## When needed

If the controller has any of these in its Owns/OwnsGVK list:
- `SecurityContextConstraints` (security.openshift.io)
- `Route` (route.openshift.io)
- `ConsoleLink` (console.openshift.io)

## Script

`hack/scripts/fetch_external_crds.sh` — generates CRDs from Go module cache
using controller-gen.

### Template

Copy from the ray module:
```
modules/opendatahub-ray-operator/hack/scripts/fetch_external_crds.sh
```

Change the `fetch_crds` call at the bottom to match the component's needs:

```bash
# SecurityContextConstraints (ray, datasciencepipelines)
fetch_crds "github.com/openshift/api" "security/v1" "securitycontextconstraints"

# Route (dashboard, mlflowoperator)
fetch_crds "github.com/openshift/api" "route/v1"

# ConsoleLink (dashboard, mlflowoperator)
fetch_crds "github.com/openshift/api" "console/v1" "consolelinks"
```

## Output

CRDs are written to `config/crd/external/`. This directory is:
- **Tracked in git** (NOT gitignored)
- **Not added to kustomization.yaml** (they're for testing only)
- **Installed during kind-setup.sh**

## Kind Setup Integration

The `kind-setup.sh` script installs external CRDs after cert-manager:

```bash
EXTERNAL_CRD_DIR="${SCRIPT_DIR}/../../config/crd/external"
if [ -d "${EXTERNAL_CRD_DIR}" ] && ls "${EXTERNAL_CRD_DIR}"/*.yaml &>/dev/null; then
    echo "Installing external CRDs from ${EXTERNAL_CRD_DIR}..."
    kubectl apply -f "${EXTERNAL_CRD_DIR}/"
fi
```

## Makefile

Add a target and make it a dependency of `test-integration`:

```makefile
.PHONY: fetch-external-crds
fetch-external-crds: ## Fetch external CRDs needed for testing (e.g. SCC).
	./hack/scripts/fetch_external_crds.sh

test-integration: manifests generate fetch-external-crds
	go test ./test/integration/ -tags=integration -v -timeout 30m
```

## If not needed

If the component doesn't own any OpenShift-specific resources:
- Don't create `fetch_external_crds.sh`
- Don't add `fetch-external-crds` to the Makefile
- Don't add the external CRD install block to `kind-setup.sh`
