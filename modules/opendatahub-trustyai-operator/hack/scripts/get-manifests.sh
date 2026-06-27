#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"

COMPONENT_NAME="trustyai"
SOURCE_PATH="config"
DST_MANIFESTS_DIR="${PROJECT_ROOT}/assets/manifests/${COMPONENT_NAME}"

if [[ "${ODH_PLATFORM_TYPE:-OpenDataHub}" == "OpenDataHub" ]]; then
    echo "Downloading manifests for ODH"
    REPO_URL="https://github.com/opendatahub-io/trustyai-service-operator"
    COMMIT_SHA="de96668b0690db47574bab3ff737e5748be235ee"
else
    echo "Downloading manifests for RHOAI"
    REPO_URL="https://github.com/red-hat-data-services/trustyai-service-operator"
    COMMIT_SHA="99914bc3c081532d0741a471826143b8adb67c6a"
fi

if [[ "${USE_LOCAL:-}" == "true" ]] && [[ -d "${PROJECT_ROOT}/../trustyai-service-operator" ]]; then
    echo "Copying manifests from adjacent trustyai-service-operator checkout"
    rm -rf "${DST_MANIFESTS_DIR}"
    mkdir -p "${DST_MANIFESTS_DIR}"
    cp -a "${PROJECT_ROOT}/../trustyai-service-operator/${SOURCE_PATH}/." "${DST_MANIFESTS_DIR}/"
    echo "Manifests copied to ${DST_MANIFESTS_DIR}"
    exit 0
fi

TMP_DIR=$(mktemp -d -t "odh-trustyai-manifests.XXXXXXXXXX")
trap 'rm -rf -- "${TMP_DIR}"' EXIT

git -C "${TMP_DIR}" init -q
git -C "${TMP_DIR}" remote add origin "${REPO_URL}"
git -C "${TMP_DIR}" fetch --depth 1 -q origin "${COMMIT_SHA}"
git -C "${TMP_DIR}" reset -q --hard "${COMMIT_SHA}"

rm -rf "${DST_MANIFESTS_DIR}"
mkdir -p "${DST_MANIFESTS_DIR}"
cp -a "${TMP_DIR}/${SOURCE_PATH}/." "${DST_MANIFESTS_DIR}/"

echo "Manifests downloaded to ${DST_MANIFESTS_DIR}"
