#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"

NAMESPACE="${1:-opendatahub-workbenches-system}"
HELM_RELEASE="${2:-opendatahub-workbenches-operator}"
APPLICATION_NAMESPACE="${3:-${NAMESPACE}}"
CR_RESOURCE="workbenches.components.platform.opendatahub.io"

echo "Cleaning up e2e test resources..."

# Delete component CRs first and wait for them to disappear before Helm or CRD removal.
kubectl delete "${CR_RESOURCE}" --all --ignore-not-found 2>/dev/null || true
kubectl wait --for=delete "${CR_RESOURCE}" --all --timeout=60s 2>/dev/null || true

# Uninstall Helm release (removes operator Deployment, RBAC, CRD, etc.)
make -C "${PROJECT_ROOT}" undeploy-helm \
  HELM_NAMESPACE="${NAMESPACE}" \
  HELM_RELEASE="${HELM_RELEASE}" 2>/dev/null || true

# Delete namespace
kubectl delete namespace "${NAMESPACE}" --ignore-not-found 2>/dev/null || true
kubectl wait --for=delete "namespace/${NAMESPACE}" --timeout=120s 2>/dev/null || true

# Delete any leftover cluster-scoped resources
kubectl delete clusterroles -l platform.opendatahub.io/part-of=workbenches --ignore-not-found 2>/dev/null || true
kubectl delete clusterrolebindings -l platform.opendatahub.io/part-of=workbenches --ignore-not-found 2>/dev/null || true
kubectl delete mutatingwebhookconfiguration \
  opendatahub-workbenches-mutating-webhook-configuration \
  odh-notebook-controller-mutating-webhook-configuration \
  --ignore-not-found 2>/dev/null || true
kubectl delete validatingwebhookconfiguration \
  odh-notebook-controller-validating-webhook-configuration \
  --ignore-not-found 2>/dev/null || true

# Delete CRD if still present (Helm should have removed it)
kubectl delete crd "${CR_RESOURCE}" --ignore-not-found 2>/dev/null || true
KUBECTL="${KUBECTL:-kubectl}" ./hack/scripts/external-types-cleanup.sh "${APPLICATION_NAMESPACE}"

echo "E2E test cleanup complete."
