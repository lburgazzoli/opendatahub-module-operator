#!/usr/bin/env bash
# Fetch external CRDs required for integration and e2e tests.
# These CRDs are not part of the module itself but are needed at test time
# so the controller's informers can register watchers for them.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"

DST_DIR="${PROJECT_ROOT}/config/crd/external"
mkdir -p "${DST_DIR}"

# HardwareProfile CRD — from the opendatahub-operator fork used by this module.
# The commit is derived from the replace directive in go.mod:
#   replace github.com/opendatahub-io/opendatahub-operator/v2 => github.com/lburgazzoli/opendatahub-operator/v2 v2.0.0-20260522211029-67d95fa3b5a1
REPO_URL="https://github.com/lburgazzoli/opendatahub-operator"
CRD_PATH="config/crd/bases/infrastructure.opendatahub.io_hardwareprofiles.yaml"
DST_FILE="${DST_DIR}/infrastructure.opendatahub.io_hardwareprofiles.yaml"

# Adjacent checkout: modules/opendatahub-workbenches-operator → ../../.. → openshift-ai/opendatahub-operator
LOCAL_MONOLITH="${PROJECT_ROOT}/../../../opendatahub-operator"

# Prefer the adjacent local checkout when it exists — avoids network fetch.
if [[ -d "${LOCAL_MONOLITH}" ]] && [[ -f "${LOCAL_MONOLITH}/${CRD_PATH}" ]]; then
    echo "Copying HardwareProfile CRD from adjacent opendatahub-operator checkout"
    cp "${LOCAL_MONOLITH}/${CRD_PATH}" "${DST_FILE}"
    echo "HardwareProfile CRD copied to ${DST_FILE}"
    exit 0
fi

# Fall back to remote fetch. Use the full commit SHA from the go.mod pseudo-version.
# Pseudo-version format: v0.0.0-<timestamp>-<12-char-sha>
# git fetch accepts full SHAs but not short ones; extract from go.mod at runtime.
FULL_SHA=$(grep 'lburgazzoli/opendatahub-operator' "${PROJECT_ROOT}/go.mod" | \
           grep -oE '[a-f0-9]{12}$' | head -1)

if [[ -z "${FULL_SHA}" ]]; then
    echo "ERROR: could not determine commit SHA from go.mod" >&2
    exit 1
fi

echo "Fetching HardwareProfile CRD from ${REPO_URL} (resolving ${FULL_SHA})"

TMP_DIR=$(mktemp -d -t "odh-external-crds.XXXXXXXXXX")
trap 'rm -rf -- "${TMP_DIR}"' EXIT

git -C "${TMP_DIR}" init -q
git -C "${TMP_DIR}" remote add origin "${REPO_URL}"
# Short SHAs don't work with fetch --depth; clone the tip then checkout.
git -C "${TMP_DIR}" fetch -q origin
git -C "${TMP_DIR}" checkout -q "${FULL_SHA}" -- "${CRD_PATH}"

cp "${TMP_DIR}/${CRD_PATH}" "${DST_FILE}"
echo "HardwareProfile CRD fetched to ${DST_FILE}"
