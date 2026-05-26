#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"

NAMESPACE="${1:-integration-test}"
CR_RESOURCE="datasciencepipelines.components.platform.opendatahub.io"
WORKFLOWS_CRD="workflows.argoproj.io"
TEST_MANAGED_LABEL="testing.opendatahub.io/managed-by"

cleanup_test_managed_workflows_crd() {
  local managed_by

  managed_by="$(
    { kubectl get crd "${WORKFLOWS_CRD}" -o json 2>/dev/null || true; } \
      | jq -r --arg key "${TEST_MANAGED_LABEL}" '.metadata.labels[$key] // ""'
  )"

  case "${managed_by}" in
    dsp-integration|dsp-e2e)
      kubectl delete crd "${WORKFLOWS_CRD}" --ignore-not-found 2>/dev/null || true
      kubectl wait --for=delete "crd/${WORKFLOWS_CRD}" --timeout=60s 2>/dev/null || true
      ;;
  esac
}

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
kubectl delete servicemonitors --all -n "${NAMESPACE}" --ignore-not-found 2>/dev/null || true

# Delete cluster-scoped resources created by the controller
kubectl delete clusterroles -l platform.opendatahub.io/part-of=data-science-pipelines-operator --ignore-not-found 2>/dev/null || true
kubectl delete clusterrolebindings -l platform.opendatahub.io/part-of=data-science-pipelines-operator --ignore-not-found 2>/dev/null || true
kubectl delete securitycontextconstraints -l platform.opendatahub.io/part-of=data-science-pipelines-operator --ignore-not-found 2>/dev/null || true

# Delete test RBAC
kubectl delete clusterrole integration-test-role --ignore-not-found 2>/dev/null || true
kubectl delete clusterrolebinding integration-test-binding --ignore-not-found 2>/dev/null || true

# Delete test-managed Argo Workflows CRD left by integration/e2e runs.
cleanup_test_managed_workflows_crd

# Delete CRD (so next run installs fresh)
kubectl delete crd "${CR_RESOURCE}" --ignore-not-found 2>/dev/null || true

echo "Integration test cleanup complete."
