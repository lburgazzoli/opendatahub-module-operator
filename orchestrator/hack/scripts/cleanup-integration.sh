#!/usr/bin/env bash
set -euo pipefail

KUBECTL="${KUBECTL:-kubectl}"

resources=(
	"platform/default-platform"
	"platformoperator/alpha"
	"platformoperator/beta"
	"platformoperator/delta"
	"platformoperator/epsilon"
	"platformoperator/gamma"
	"platformoperator/gated"
	"namespace/test-alpha"
	"namespace/test-beta"
	"namespace/test-delta"
	"namespace/test-epsilon"
	"namespace/test-gamma"
	"namespace/test-gated"
)

for resource in "${resources[@]}"; do
	$KUBECTL delete "$resource" --ignore-not-found >/dev/null
done

$KUBECTL delete configmap/opendatahub-admin -n opendatahub --ignore-not-found >/dev/null

for resource in "${resources[@]}"; do
	if $KUBECTL get "$resource" >/dev/null 2>&1; then
		$KUBECTL wait --for=delete "$resource" --timeout=60s >/dev/null
	fi
done

if $KUBECTL get configmap/opendatahub-admin -n opendatahub >/dev/null 2>&1; then
	$KUBECTL wait --for=delete configmap/opendatahub-admin -n opendatahub --timeout=60s >/dev/null
fi
