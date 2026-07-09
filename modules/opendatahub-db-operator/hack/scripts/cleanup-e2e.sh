#!/usr/bin/env bash
set -euo pipefail

OPERATOR_NAMESPACE="${1:-odh-db-operator-system}"
HELM_RELEASE="${2:-odh-db-operator}"
WORKLOAD_NAMESPACE="${E2E_TEST_NAMESPACE:-odh-db-operator-e2e}"

SCHEMA_CLAIMS_GVR="schemaclaims.infrastructure.opendatahub.io"
DATABASE_CLAIMS_GVR="databaseclaims.infrastructure.opendatahub.io"
DATABASE_PROVIDERS_GVR="databaseproviders.infrastructure.opendatahub.io"
DATABASE_SERVICES_GVR="databaseservices.services.platform.opendatahub.io"

echo "Cleaning up e2e test resources..."

kubectl delete "${SCHEMA_CLAIMS_GVR}" --all -n "${WORKLOAD_NAMESPACE}" --ignore-not-found 2>/dev/null || true
kubectl delete "${DATABASE_CLAIMS_GVR}" --all -n "${WORKLOAD_NAMESPACE}" --ignore-not-found 2>/dev/null || true
kubectl delete "${DATABASE_SERVICES_GVR}" default-db-operator --ignore-not-found 2>/dev/null || true

deadline=$((SECONDS + 90))
while true; do
    claims="$(
        {
            kubectl get "${SCHEMA_CLAIMS_GVR}" -n "${WORKLOAD_NAMESPACE}" -o name 2>/dev/null || true
            kubectl get "${DATABASE_CLAIMS_GVR}" -n "${WORKLOAD_NAMESPACE}" -o name 2>/dev/null || true
        }
    )"
    if [[ -z "${claims}" ]]; then
        break
    fi
    if (( SECONDS >= deadline )); then
        echo "Timed out waiting for e2e claims to be deleted from ${WORKLOAD_NAMESPACE}" >&2
        exit 1
    fi
    sleep 2
done

provider_names="$(
    kubectl get "${DATABASE_PROVIDERS_GVR}" -o json 2>/dev/null | jq -r '
        .items[]
        | select(.metadata.name | test("^e2e-"))
        | .metadata.name
    ' || true
)"
if [[ -n "${provider_names}" ]]; then
    while IFS= read -r name; do
        [[ -n "${name}" ]] || continue
        kubectl delete "${DATABASE_PROVIDERS_GVR}" "${name}" --ignore-not-found 2>/dev/null || true
    done <<<"${provider_names}"
fi

go run helm.sh/helm/v4/cmd/helm@v4.2.0 uninstall "${HELM_RELEASE}" \
    --namespace "${OPERATOR_NAMESPACE}" \
    --ignore-not-found \
    --wait 2>/dev/null || true

kubectl delete namespace "${WORKLOAD_NAMESPACE}" --ignore-not-found 2>/dev/null || true
kubectl delete namespace "${OPERATOR_NAMESPACE}" --ignore-not-found 2>/dev/null || true

for crd in \
    "${SCHEMA_CLAIMS_GVR}" \
    "${DATABASE_CLAIMS_GVR}" \
    "${DATABASE_PROVIDERS_GVR}" \
    "${DATABASE_SERVICES_GVR}"
do
    kubectl delete crd "${crd}" --ignore-not-found 2>/dev/null || true
done

echo "E2E test cleanup complete."
