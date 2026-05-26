#!/usr/bin/env bash
set -euo pipefail

usage() {
    cat >&2 <<'EOF'
Usage: push-crc-image.sh <source-image> <namespace> <image-name>

Push a locally built image to the CRC/OpenShift internal registry and print the
in-cluster pullspec on stdout.
EOF
}

source_img="${1:-}"
image_namespace="${2:-}"
image_name="${3:-}"

registry_route_name="${CRC_REGISTRY_ROUTE_NAME:-default-route}"
registry_route_namespace="${CRC_REGISTRY_ROUTE_NAMESPACE:-openshift-image-registry}"
internal_registry_host="${CRC_INTERNAL_REGISTRY_HOST:-image-registry.openshift-image-registry.svc:5000}"

if [[ -z "${source_img}" || -z "${image_namespace}" || -z "${image_name}" ]]; then
    usage
    exit 1
fi

external_host="$(oc get route "${registry_route_name}" -n "${registry_route_namespace}" -o jsonpath='{.spec.host}' 2>/dev/null || true)"
if [[ -z "${external_host}" ]]; then
    echo "OpenShift image registry route ${registry_route_name} not found in namespace ${registry_route_namespace}" >&2
    echo "Verify CRC exposes the default route and that 'oc registry login --insecure=true' works." >&2
    exit 1
fi

if [[ -z "$(kubectl get namespace "${image_namespace}" -o name --ignore-not-found)" ]]; then
    echo "Ensuring namespace ${image_namespace} exists" >&2
    kubectl create namespace "${image_namespace}" >/dev/null
fi

crc_tag="$(uuidgen | tr '[:upper:]' '[:lower:]')"
external_image="${external_host}/${image_namespace}/${image_name}:${crc_tag}"
internal_image="${internal_registry_host}/${image_namespace}/${image_name}:${crc_tag}"

echo "Logging into ${external_host}" >&2
oc registry login --insecure=true --registry "${external_host}" >/dev/null
echo "Tagging ${source_img} as ${external_image}" >&2
podman tag "${source_img}" "${external_image}"
echo "Pushing ${external_image}" >&2
podman push "${external_image}" --tls-verify=false >/dev/null
printf '%s\n' "${internal_image}"
