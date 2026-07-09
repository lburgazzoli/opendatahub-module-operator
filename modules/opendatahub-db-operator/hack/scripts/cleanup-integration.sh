#!/usr/bin/env bash
set -euo pipefail

NAMESPACE="${1:-odh-db-operator-integration}"

SCHEMA_CLAIMS_GVR="schemaclaims.infrastructure.opendatahub.io"
DATABASE_CLAIMS_GVR="databaseclaims.infrastructure.opendatahub.io"
DATABASE_PROVIDERS_GVR="databaseproviders.infrastructure.opendatahub.io"
DATABASE_SERVICES_GVR="databaseservices.services.platform.opendatahub.io"

echo "Cleaning up integration test resources for namespace ${NAMESPACE}..."

kubectl delete "${SCHEMA_CLAIMS_GVR}" --all -n "${NAMESPACE}" --ignore-not-found 2>/dev/null || true
kubectl delete "${DATABASE_CLAIMS_GVR}" --all -n "${NAMESPACE}" --ignore-not-found 2>/dev/null || true
kubectl delete "${DATABASE_SERVICES_GVR}" default-db-operator --ignore-not-found 2>/dev/null || true

deadline=$((SECONDS + 90))
while true; do
    claims="$(
        {
            kubectl get "${SCHEMA_CLAIMS_GVR}" -n "${NAMESPACE}" -o name 2>/dev/null || true
            kubectl get "${DATABASE_CLAIMS_GVR}" -n "${NAMESPACE}" -o name 2>/dev/null || true
        }
    )"
    if [[ -z "${claims}" ]]; then
        break
    fi
    if (( SECONDS >= deadline )); then
        echo "Timed out waiting for claims to be deleted from ${NAMESPACE}" >&2
        exit 1
    fi
    sleep 2
done

provider_names="$(
    kubectl get "${DATABASE_PROVIDERS_GVR}" -o json 2>/dev/null | jq -r --arg ns "${NAMESPACE}" '
        .items[]
        | select(
            (.metadata.annotations["db.infrastructure.opendatahub.io/operator-namespace"] == $ns)
            or (.metadata.name | test("^(provider-|schema-provider-|database-provider-|embedded-)"))
        )
        | .metadata.name
    ' || true
)"
if [[ -n "${provider_names}" ]]; then
    while IFS= read -r name; do
        [[ -n "${name}" ]] || continue
        kubectl delete "${DATABASE_PROVIDERS_GVR}" "${name}" --ignore-not-found 2>/dev/null || true
    done <<<"${provider_names}"
fi

kubectl delete secrets,configmaps,services,persistentvolumeclaims,statefulsets,networkpolicies \
    --all -n "${NAMESPACE}" --ignore-not-found 2>/dev/null || true

embedded_namespaces="$(
    kubectl get namespaces -o json 2>/dev/null | jq -r '
        .items[]
        | select(.metadata.name | test("^embedded-"))
        | .metadata.name
    ' || true
)"
if [[ -n "${embedded_namespaces}" ]]; then
    while IFS= read -r ns; do
        [[ -n "${ns}" ]] || continue
        kubectl delete namespace "${ns}" --ignore-not-found 2>/dev/null || true
    done <<<"${embedded_namespaces}"
fi

for crd in \
    "${SCHEMA_CLAIMS_GVR}" \
    "${DATABASE_CLAIMS_GVR}" \
    "${DATABASE_PROVIDERS_GVR}" \
    "${DATABASE_SERVICES_GVR}"
do
    kubectl delete crd "${crd}" --ignore-not-found 2>/dev/null || true
done

echo "Integration test cleanup complete."
