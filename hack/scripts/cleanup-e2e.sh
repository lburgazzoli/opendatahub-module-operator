#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"

NAMESPACE="${1:-opendatahub-module-operator-system}"
HELM_RELEASE="${2:-opendatahub-module-operator}"

echo "Cleaning up e2e test resources..."

# Delete component CRs
kubectl delete mymodules.components.platform.opendatahub.io --all --ignore-not-found 2>/dev/null || true

# Uninstall Helm release (removes operator Deployment, RBAC, CRD, etc.)
go run helm.sh/helm/v4/cmd/helm@v4.2.0 uninstall "${HELM_RELEASE}" \
    --namespace "${NAMESPACE}" \
    --ignore-not-found \
    --wait 2>/dev/null || true

# Delete namespace
kubectl delete namespace "${NAMESPACE}" --ignore-not-found 2>/dev/null || true

# Delete any leftover cluster-scoped resources
kubectl delete clusterroles -l platform.opendatahub.io/part-of=mymodule --ignore-not-found 2>/dev/null || true
kubectl delete clusterrolebindings -l platform.opendatahub.io/part-of=mymodule --ignore-not-found 2>/dev/null || true

# Delete CRD if still present (Helm should have removed it)
kubectl delete crd mymodules.components.platform.opendatahub.io --ignore-not-found 2>/dev/null || true

echo "E2E test cleanup complete."
