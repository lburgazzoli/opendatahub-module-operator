#!/usr/bin/env bash
set -euo pipefail

fail() {
  printf 'error: %s\n' "$1" >&2
  exit 1
}

require_command() {
  command -v "$1" >/dev/null 2>&1 || fail "required command not found: $1"
}

require_command oc
require_command uuidgen

container_tool="${CONTAINER_TOOL:-podman}"
require_command "$container_tool"

source_img="${IMG:-}"
module_name="${MODULE_NAME:-}"
crc_registry="${CRC_REGISTRY:-}"
crc_image_namespace="${CRC_IMAGE_NAMESPACE:-}"
crc_img="${CRC_IMG:-}"

[[ -n "$source_img" ]] || fail "IMG must point at the local image to push"
[[ -n "$module_name" ]] || fail "MODULE_NAME must be set"

if [[ -z "$crc_registry" ]]; then
  crc_registry="$(oc get route default-route -n openshift-image-registry --template='{{ .spec.host }}' 2>/dev/null || true)"
fi
[[ -n "$crc_registry" ]] || fail "unable to resolve the CRC image registry route; enable it with: oc patch configs.imageregistry.operator.openshift.io/cluster --type=merge -p '{\"spec\":{\"defaultRoute\":true}}'"

if [[ -z "$crc_image_namespace" ]]; then
  crc_image_namespace="$(oc project -q 2>/dev/null || true)"
fi
[[ -n "$crc_image_namespace" ]] || fail "unable to determine the OpenShift project; run 'oc project <name>' first or set CRC_IMAGE_NAMESPACE"

if [[ -z "$crc_img" ]]; then
  crc_tag="$(uuidgen | tr '[:upper:]' '[:lower:]')"
  crc_img="${crc_registry}/${crc_image_namespace}/${module_name}:${crc_tag}"
fi

oc_user="$(oc whoami 2>/dev/null || true)"
oc_token="$(oc whoami -t 2>/dev/null || true)"
[[ -n "$oc_user" ]] || fail "unable to determine the current OpenShift user; run 'oc login' first"
[[ -n "$oc_token" ]] || fail "unable to fetch an OpenShift token; run 'oc login' first"

printf 'Logging into CRC registry %s\n' "$crc_registry" >&2
"$container_tool" login -u "$oc_user" -p "$oc_token" "$crc_registry" >/dev/null

printf 'Tagging %s as %s\n' "$source_img" "$crc_img" >&2
"$container_tool" tag "$source_img" "$crc_img"

printf 'Pushing %s\n' "$crc_img" >&2
"$container_tool" push "$crc_img" 1>&2

printf '%s\n' "$crc_img"
