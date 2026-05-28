#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
KUBECTL="${KUBECTL:-kubectl}"
MODE="${1:-base}"
CONTROLLER_GEN_VERSION="${CONTROLLER_GEN_VERSION:-v0.20.1}"

BASE_EXTERNAL_TYPE_REFS=(
	"https://raw.githubusercontent.com/red-hat-data-services/odh-dashboard/rhoai-2.25/manifests/common/crd/odhdashboardconfigs.opendatahub.io.crd.yaml"
	"https://raw.githubusercontent.com/red-hat-data-services/odh-dashboard/rhoai-2.25/manifests/common/crd/acceleratorprofiles.opendatahub.io.crd.yaml"
)

UPGRADE_EXTERNAL_TYPE_REFS=(
	"${PROJECT_ROOT}/config/manifests/workbenches/odh-notebook-controller/crd/external/kubeflow.org_notebooks.yaml"
)

echo "Installing external dependency types..."

for ref in "${BASE_EXTERNAL_TYPE_REFS[@]}"; do
	"${KUBECTL}" apply -f "${ref}"
done

operator_dir="$(go list -m -f '{{.Dir}}' github.com/opendatahub-io/opendatahub-operator/v2)"
tmp_dir="$(mktemp -d)"
trap 'rm -rf "${tmp_dir}"' EXIT

(
	cd "${operator_dir}"
	go run "sigs.k8s.io/controller-tools/cmd/controller-gen@${CONTROLLER_GEN_VERSION}" \
		crd \
		paths="./api/infrastructure/..." \
		output:crd:artifacts:config="${tmp_dir}"
)

"${KUBECTL}" apply -f "${tmp_dir}/infrastructure.opendatahub.io_hardwareprofiles.yaml"

if [ "${MODE}" = "upgrade" ]; then
	for ref in "${UPGRADE_EXTERNAL_TYPE_REFS[@]}"; do
		"${KUBECTL}" apply -f "${ref}"
	done
fi

echo "External dependency types installed."
