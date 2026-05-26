#!/usr/bin/env bash
set -euo pipefail

source_img="${1:?source image is required}"
image_namespace="${2:?image namespace is required}"
image_name="${3:?image name is required}"
crc_tag="$(uuidgen | tr '[:upper:]' '[:lower:]')"

crc_registry_route_name="${CRC_REGISTRY_ROUTE_NAME:-default-route}"
crc_registry_route_namespace="${CRC_REGISTRY_ROUTE_NAMESPACE:-openshift-image-registry}"
crc_internal_registry_host="${CRC_INTERNAL_REGISTRY_HOST:-image-registry.openshift-image-registry.svc:5000}"
container_tool="${CONTAINER_TOOL:-podman}"

external_host="$(oc get route "${crc_registry_route_name}" -n "${crc_registry_route_namespace}" -o jsonpath='{.spec.host}' 2>/dev/null || true)"
if [[ -z "${external_host}" ]]; then
    echo "OpenShift image registry route ${crc_registry_route_name} not found in namespace ${crc_registry_route_namespace}" >&2
    echo "Verify CRC exposes the default route and that 'oc registry login --insecure=true' works." >&2
    exit 1
fi

if [[ -z "$(kubectl get namespace "${image_namespace}" -o name --ignore-not-found)" ]]; then
    echo "Ensuring namespace ${image_namespace} exists" >&2
    kubectl create namespace "${image_namespace}" >/dev/null
fi

external_image="${external_host}/${image_namespace}/${image_name}:${crc_tag}"
internal_image="${crc_internal_registry_host}/${image_namespace}/${image_name}:${crc_tag}"

echo "Logging into ${external_host}" >&2
oc registry login --insecure=true --registry "${external_host}" >/dev/null

echo "Tagging ${source_img} as ${external_image}" >&2
"${container_tool}" tag "${source_img}" "${external_image}"

echo "Pushing ${external_image}" >&2
"${container_tool}" push "${external_image}" --tls-verify=false >/dev/null

printf '%s\n' "${internal_image}"
