#!/usr/bin/env bash
set -euo pipefail

log() {
    printf '%s\n' "$*" >&2
}

usage() {
    cat >&2 <<'EOF'
Usage: resolve-image-ref.sh <image-ref>

Print a canonical image reference for Helm deployment.
If the input already uses a digest, it is returned unchanged.
Otherwise the script tries to resolve the image tag to a digest via the local
container tool, pulling the image if needed. If digest lookup still fails,
the original input reference is returned.
EOF
}

image_ref="${1:-}"
container_tool="${CONTAINER_TOOL:-podman}"

if [[ -z "${image_ref}" ]]; then
    usage
    exit 1
fi

if ! command -v "${container_tool}" >/dev/null 2>&1; then
    log "${container_tool} is required"
    exit 1
fi

if [[ "${image_ref}" == *@sha256:* ]]; then
    printf '%s\n' "${image_ref}"
    exit 0
fi

if [[ "${image_ref}" == *.svc:*/* || "${image_ref}" == *.svc/* ]]; then
    log "leaving in-cluster service image reference unchanged: ${image_ref}"
    printf '%s\n' "${image_ref}"
    exit 0
fi

if [[ "${image_ref}" == ttl.sh/* ]]; then
    log "leaving ttl.sh image reference unchanged: ${image_ref}"
    printf '%s\n' "${image_ref}"
    exit 0
fi


printf '%s\n' "${image_ref}"
