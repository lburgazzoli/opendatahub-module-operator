#!/usr/bin/env bash
set -euo pipefail

source_img="${1:?source image is required}"
image_namespace="${2:?image namespace is required}"
image_name="${3:?image name is required}"
ocp_tag="$(uuidgen | tr '[:upper:]' '[:lower:]')"

ocp_registry_route_name="${OCP_REGISTRY_ROUTE_NAME:-default-route}"
ocp_registry_route_namespace="${OCP_REGISTRY_ROUTE_NAMESPACE:-openshift-image-registry}"
ocp_internal_registry_host="${OCP_INTERNAL_REGISTRY_HOST:-image-registry.openshift-image-registry.svc:5000}"
container_tool="${CONTAINER_TOOL:-podman}"

external_host="$(oc get route "${ocp_registry_route_name}" -n "${ocp_registry_route_namespace}" -o jsonpath='{.spec.host}' 2>/dev/null || true)"
if [[ -z "${external_host}" ]]; then
    echo "OpenShift image registry route ${ocp_registry_route_name} not found in namespace ${ocp_registry_route_namespace}" >&2
    echo "Verify the cluster exposes the default route and that 'oc registry login --insecure=true' works." >&2
    exit 1
fi

if [[ -z "$(kubectl get namespace "${image_namespace}" -o name --ignore-not-found)" ]]; then
    echo "Ensuring namespace ${image_namespace} exists" >&2
    kubectl create namespace "${image_namespace}" >/dev/null
fi

external_image="${external_host}/${image_namespace}/${image_name}:${ocp_tag}"
internal_image="${ocp_internal_registry_host}/${image_namespace}/${image_name}:${ocp_tag}"

echo "Logging into ${external_host}" >&2
oc registry login --insecure=true --registry "${external_host}" >/dev/null

echo "Tagging ${source_img} as ${external_image}" >&2
"${container_tool}" tag "${source_img}" "${external_image}"

echo "Pushing ${external_image}" >&2
"${container_tool}" push "${external_image}" --tls-verify=false >/dev/null

printf '%s\n' "${internal_image}"
