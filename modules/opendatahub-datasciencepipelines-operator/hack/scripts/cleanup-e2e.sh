#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"

NAMESPACE="${1:-opendatahub-datasciencepipelines-system}"
HELM_RELEASE="${2:-opendatahub-datasciencepipelines-operator}"
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

echo "Cleaning up e2e test resources..."

# Delete component CRs first and wait for them to disappear before Helm or CRD removal.
kubectl delete "${CR_RESOURCE}" --all --ignore-not-found 2>/dev/null || true
kubectl wait --for=delete "${CR_RESOURCE}" --all --timeout=60s 2>/dev/null || true

# Uninstall Helm release (removes operator Deployment, RBAC, CRD, etc.)
go run helm.sh/helm/v4/cmd/helm@v4.2.0 uninstall "${HELM_RELEASE}" \
    --namespace "${NAMESPACE}" \
    --ignore-not-found \
    --wait 2>/dev/null || true

# Delete namespace
kubectl delete namespace "${NAMESPACE}" --ignore-not-found 2>/dev/null || true

# Delete any leftover cluster-scoped resources
kubectl delete clusterroles -l platform.opendatahub.io/part-of=data-science-pipelines-operator --ignore-not-found 2>/dev/null || true
kubectl delete clusterrolebindings -l platform.opendatahub.io/part-of=data-science-pipelines-operator --ignore-not-found 2>/dev/null || true
kubectl delete securitycontextconstraints -l platform.opendatahub.io/part-of=data-science-pipelines-operator --ignore-not-found 2>/dev/null || true

# Delete test-managed Argo Workflows CRD left by integration/e2e runs.
cleanup_test_managed_workflows_crd

# Delete CRD if still present (Helm should have removed it)
kubectl delete crd "${CR_RESOURCE}" --ignore-not-found 2>/dev/null || true

echo "E2E test cleanup complete."
