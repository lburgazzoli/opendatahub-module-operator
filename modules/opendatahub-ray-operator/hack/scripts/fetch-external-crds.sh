#!/usr/bin/env bash
set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"

CRD_DIR="${PROJECT_ROOT}/config/crd/external"
CONTROLLER_GEN="go run sigs.k8s.io/controller-tools/cmd/controller-gen@v0.20.1"

mkdir -p "${CRD_DIR}"

# Resolve module version from go.mod
go_mod_version() {
    local module=$1
    cd "${PROJECT_ROOT}"
    go list -m -f '{{.Version}}' "${module}" 2>/dev/null
}

# Fetch CRDs from a Go module using controller-gen
# Usage: fetch_crds <module> <path> [kinds...]
fetch_crds() {
    local module=$1
    local path=$2
    shift 2
    local kinds=("$@")

    local version
    version=$(go_mod_version "${module}")
    if [ -z "${version}" ]; then
        echo "WARNING: module ${module} not found in go.mod, skipping"
        return
    fi

    local mod_path
    mod_path="$(go env GOPATH)/pkg/mod/${module}@${version}/${path}/..."

    local tmp_dir
    tmp_dir=$(mktemp -d)
    trap "rm -rf ${tmp_dir}" RETURN

    echo "Fetching CRDs from ${module}@${version}/${path}"

    GOFLAGS="-mod=readonly" ${CONTROLLER_GEN} crd \
        "paths=${mod_path}" \
        "output:crd:artifacts:config=${tmp_dir}"

    if [ ${#kinds[@]} -gt 0 ]; then
        for kind in "${kinds[@]}"; do
            find "${tmp_dir}" -type f -name "*_${kind}.yaml" -exec mv {} "${CRD_DIR}/" \;
        done
    else
        mv "${tmp_dir}"/*.yaml "${CRD_DIR}/" 2>/dev/null || true
    fi
}

# SecurityContextConstraints — required by the ray operator (OpenShift-only resource)
fetch_crds "github.com/openshift/api" "security/v1" "securitycontextconstraints"

echo "External CRDs fetched to ${CRD_DIR}"
ls -1 "${CRD_DIR}"
