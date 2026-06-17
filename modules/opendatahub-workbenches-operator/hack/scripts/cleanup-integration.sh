#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"

NAMESPACE="${1:-opendatahub-workbenches-integration}"
CR_RESOURCE="workbenches.components.platform.opendatahub.io"

echo "Cleaning up integration test resources..."

# Delete component CRs first and wait for them to disappear before touching the CRD.
kubectl delete "${CR_RESOURCE}" --all --ignore-not-found 2>/dev/null || true
kubectl wait --for=delete "${CR_RESOURCE}" --all --timeout=60s 2>/dev/null || true

# Delete workload resources in test namespace
kubectl delete deployments --all -n "${NAMESPACE}" --ignore-not-found 2>/dev/null || true
kubectl delete services --all -n "${NAMESPACE}" --ignore-not-found 2>/dev/null || true
kubectl delete configmaps --all -n "${NAMESPACE}" --ignore-not-found 2>/dev/null || true
kubectl delete secrets --all -n "${NAMESPACE}" --ignore-not-found 2>/dev/null || true
kubectl delete serviceaccounts --all -n "${NAMESPACE}" --ignore-not-found 2>/dev/null || true
kubectl delete roles --all -n "${NAMESPACE}" --ignore-not-found 2>/dev/null || true
kubectl delete rolebindings --all -n "${NAMESPACE}" --ignore-not-found 2>/dev/null || true

# Delete cluster-scoped resources created by the controller
kubectl delete clusterroles -l platform.opendatahub.io/part-of=workbenches --ignore-not-found 2>/dev/null || true
kubectl delete clusterrolebindings -l platform.opendatahub.io/part-of=workbenches --ignore-not-found 2>/dev/null || true
kubectl delete mutatingwebhookconfiguration \
  opendatahub-workbenches-mutating-webhook-configuration \
  odh-notebook-controller-mutating-webhook-configuration \
  --ignore-not-found 2>/dev/null || true
kubectl delete validatingwebhookconfiguration \
  odh-notebook-controller-validating-webhook-configuration \
  --ignore-not-found 2>/dev/null || true

# Delete CRD (so next run installs fresh)
kubectl delete crd "${CR_RESOURCE}" --ignore-not-found 2>/dev/null || true
KUBECTL="${KUBECTL:-kubectl}" ./hack/scripts/external-types-cleanup.sh "${NAMESPACE}"

echo "Integration test cleanup complete."
