#!/usr/bin/env bash
set -euo pipefail

APPLICATION_NAMESPACE="${1:-}"
KUBECTL="${KUBECTL:-kubectl}"

EXTERNAL_TYPES=(
	"notebooks:notebooks.kubeflow.org"
	"odhdashboardconfigs:odhdashboardconfigs.opendatahub.io"
	"acceleratorprofiles:acceleratorprofiles.dashboard.opendatahub.io"
	"hardwareprofiles:hardwareprofiles.infrastructure.opendatahub.io"
)

delete_all_instances() {
	local resource="$1"
	local namespace="$2"

	if ! "${KUBECTL}" api-resources --verbs=list -o name 2>/dev/null | rg -x --fixed-strings "${resource}" >/dev/null; then
		return 0
	fi

	if "${KUBECTL}" api-resources --verbs=list --namespaced -o name 2>/dev/null | rg -x --fixed-strings "${resource}" >/dev/null; then
		if [ -n "${namespace}" ]; then
			"${KUBECTL}" delete "${resource}" --all -n "${namespace}" --ignore-not-found 2>/dev/null || true
		fi
		return 0
	fi

	"${KUBECTL}" delete "${resource}" --all --ignore-not-found 2>/dev/null || true
}

echo "Cleaning up external dependency types..."

for entry in "${EXTERNAL_TYPES[@]}"; do
	resource="${entry%%:*}"
	crd="${entry#*:}"

	delete_all_instances "${resource}" "${APPLICATION_NAMESPACE}"
	"${KUBECTL}" delete crd "${crd}" --ignore-not-found 2>/dev/null || true
done

echo "External dependency types cleaned up."
