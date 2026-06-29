# opendatahub-datasciencepipelines-operator

Standalone module operator for the `DataSciencePipelines` component, split from
the monolithic `opendatahub-operator`. It manages the Data Science Pipelines
operator deployment on OpenShift and gates reconciliation on the
`workflows.argoproj.io` CRD ownership checks used by the monolith.

## Status

**Migration in progress.** Before this module is review-ready, complete and
verify the remaining DSP-specific checks:

- [ ] Fetch manifests: `make get-manifests` and verify the overlay structure
  under `assets/manifests/datasciencepipelines/overlays/{odh,rhoai}/`
- [ ] Derive full RBAC from kustomize output and add missing markers:
  ```bash
  kustomize build assets/manifests/datasciencepipelines/overlays/odh 2>/dev/null | \
    yq e '.kind' - | sort -u
  kustomize build assets/manifests/datasciencepipelines/overlays/odh 2>/dev/null | \
    yq e 'select(.kind == "ClusterRole") | .rules[] | .apiGroups[] + "/" + .resources[] + " " + (.verbs | join(","))' - | sort -u
  ```
- [ ] Keep the negative-path integration and e2e tests for Argo CRD gating:
  missing `workflows.argoproj.io` when `argoWorkflowsControllers.managementState=Removed`,
  and foreign-owned CRDs when DSP expects to manage Argo
- [ ] Run integration tests against an OpenShift cluster:
  `make test-integration`
- [ ] Run e2e tests after Helm deploy:
  `make get-manifests container-build container-push deploy-helm && make test-e2e`
- [ ] Implement upgrade migrations in `internal/controller/datasciencepipelines/datasciencepipelines_upgrade.go`

## Quick Start

```bash
# Download manifests
make get-manifests

# Build and test locally
make test lint

# Deploy to OpenShift
IMG=ttl.sh/opendatahub-datasciencepipelines-operator-$(uuidgen | tr '[:upper:]' '[:lower:]'):1h
make container-build container-push deploy-helm IMG=$IMG

# Run tests
make test-integration  # requires cluster
make test-e2e          # requires deployed operator
```

## Architecture

See `docs/index.md` in the root of this repository for the full split plan
and design decisions.
