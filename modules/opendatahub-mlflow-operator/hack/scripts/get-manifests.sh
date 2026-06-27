#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"

COMPONENT_NAME="mlflowoperator"
SOURCE_PATH="config"
DST_MANIFESTS_DIR="${PROJECT_ROOT}/assets/manifests/${COMPONENT_NAME}"

if [[ "${ODH_PLATFORM_TYPE:-OpenDataHub}" == "OpenDataHub" ]]; then
    echo "Downloading manifests for ODH"
    REPO_URL="https://github.com/opendatahub-io/mlflow-operator"
    COMMIT_SHA="4cccfcc2dd8576cabbf255f66894d801a68eb844"
else
    echo "Downloading manifests for RHOAI"
    REPO_URL="https://github.com/red-hat-data-services/mlflow-operator"
    COMMIT_SHA="ce3625ccb267d349d9ba08b98dec089c499a08b2"
fi

if [[ "${USE_LOCAL:-}" == "true" ]] && [[ -d "${PROJECT_ROOT}/../mlflow-operator" ]]; then
    echo "Copying manifests from adjacent mlflow-operator checkout"
    rm -rf "${DST_MANIFESTS_DIR}"
    mkdir -p "${DST_MANIFESTS_DIR}"
    cp -a "${PROJECT_ROOT}/../mlflow-operator/${SOURCE_PATH}/." "${DST_MANIFESTS_DIR}/"
    echo "Manifests copied to ${DST_MANIFESTS_DIR}"
    exit 0
fi

TMP_DIR=$(mktemp -d -t "odh-mlflow-manifests.XXXXXXXXXX")
trap 'rm -rf -- "${TMP_DIR}"' EXIT

git -C "${TMP_DIR}" init -q
git -C "${TMP_DIR}" remote add origin "${REPO_URL}"
git -C "${TMP_DIR}" fetch --depth 1 -q origin "${COMMIT_SHA}"
git -C "${TMP_DIR}" reset -q --hard "${COMMIT_SHA}"

rm -rf "${DST_MANIFESTS_DIR}"
mkdir -p "${DST_MANIFESTS_DIR}"
cp -a "${TMP_DIR}/${SOURCE_PATH}/." "${DST_MANIFESTS_DIR}/"

echo "Manifests downloaded to ${DST_MANIFESTS_DIR}"
