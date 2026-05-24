# opendatahub-spark-operator

Standalone module operator for the SparkOperator component, split from the
monolithic opendatahub-operator. Manages the lifecycle of the Spark Operator
deployment on OpenShift.

## Status

**Scaffolding complete.** Teams owning this module must:

- [ ] Fetch manifests: `make get-manifests` and verify the overlay structure
  under `config/manifests/sparkoperator/overlays/{odh,rhoai}/`
- [ ] Identify actual workload Deployment/Service names from the manifests
  and update `test/integration/integration_test.go` and
  `test/e2e/e2e_test.go` fixture names (`workloadDeploy`, `workloadService`)
- [ ] Derive full RBAC from kustomize output and add missing markers:
  ```bash
  kustomize build config/manifests/sparkoperator/overlays/odh 2>/dev/null | \
    yq e '.kind' - | sort -u
  kustomize build config/manifests/sparkoperator/overlays/odh 2>/dev/null | \
    yq e 'select(.kind == "ClusterRole") | .rules[] | .apiGroups[] + "/" + .resources[] + " " + (.verbs | join(","))' - | sort -u
  ```
- [ ] Run integration tests against an OpenShift cluster:
  `make test-integration`
- [ ] Run e2e tests after Helm deploy:
  `make get-manifests container-build container-push deploy-helm && make test-e2e`
- [ ] Implement upgrade migrations in `internal/controller/sparkoperator/sparkoperator_upgrade.go`

## Quick Start

```bash
# Download manifests
make get-manifests

# Build and test locally
make test lint

# Deploy to OpenShift
IMG=ttl.sh/opendatahub-spark-operator-$(uuidgen | tr '[:upper:]' '[:lower:]'):1h
make container-build container-push deploy-helm IMG=$IMG

# Run tests
make test-integration  # requires cluster
make test-e2e          # requires deployed operator
```

## Architecture

See `docs/index.md` in the root of this repository for the full split plan
and design decisions.
