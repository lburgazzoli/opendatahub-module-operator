#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"

COMPONENT_NAME="ray"
REPO_NAME="kuberay"
SOURCE_PATH="ray-operator/config"
DST_MANIFESTS_DIR="${PROJECT_ROOT}/assets/manifests/${COMPONENT_NAME}"

# Always wipe the component dir before copy — see manifest-script.md in
# .agents/skills/odh-component-to-module/references/

if [[ "${ODH_PLATFORM_TYPE:-OpenDataHub}" == "OpenDataHub" ]]; then
    echo "Downloading manifests for ODH"
    REPO_URL="https://github.com/opendatahub-io/${REPO_NAME}"
    COMMIT_SHA="b30c9c1a8c95cf22c2b6fe051193b429f92c69ed"
else
    echo "Downloading manifests for RHOAI"
    REPO_URL="https://github.com/red-hat-data-services/${REPO_NAME}"
    COMMIT_SHA="3e576f58face1b5a14ae77c8e3270b95b2dbc423"
fi

if [[ "${USE_LOCAL:-}" == "true" ]] && [[ -d "${PROJECT_ROOT}/../${REPO_NAME}" ]]; then
    echo "Copying manifests from adjacent ${REPO_NAME} checkout"
    rm -rf "${DST_MANIFESTS_DIR}"
    mkdir -p "${DST_MANIFESTS_DIR}"
    cp -a "${PROJECT_ROOT}/../${REPO_NAME}/${SOURCE_PATH}/." "${DST_MANIFESTS_DIR}/"
    echo "Manifests copied to ${DST_MANIFESTS_DIR}"
    exit 0
fi

TMP_DIR=$(mktemp -d -t "odh-ray-manifests.XXXXXXXXXX")
trap 'rm -rf -- "${TMP_DIR}"' EXIT

git -C "${TMP_DIR}" init -q
git -C "${TMP_DIR}" remote add origin "${REPO_URL}"
git -C "${TMP_DIR}" fetch --depth 1 -q origin "${COMMIT_SHA}"
git -C "${TMP_DIR}" reset -q --hard "${COMMIT_SHA}"

rm -rf "${DST_MANIFESTS_DIR}"
mkdir -p "${DST_MANIFESTS_DIR}"
cp -a "${TMP_DIR}/${SOURCE_PATH}/." "${DST_MANIFESTS_DIR}/"

echo "Manifests downloaded to ${DST_MANIFESTS_DIR}"
