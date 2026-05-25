#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"

COMPONENT_NAME="workbenches"
DST_MANIFESTS_DIR="${PROJECT_ROOT}/config/manifests/${COMPONENT_NAME}"

# Always wipe before copy to keep manifests clean.
rm -rf "${DST_MANIFESTS_DIR}"
mkdir -p "${DST_MANIFESTS_DIR}"

if [[ "${ODH_PLATFORM_TYPE:-OpenDataHub}" == "OpenDataHub" ]]; then
    echo "Downloading workbenches manifests for ODH"
    KUBEFLOW_REPO_URL="https://github.com/opendatahub-io/kubeflow"
    KUBEFLOW_COMMIT_SHA="f09b56e860ff88bcc05668b3f517791cdccd5b4d"
    NOTEBOOKS_REPO_URL="https://github.com/opendatahub-io/notebooks"
    NOTEBOOKS_COMMIT_SHA="139807fdae45d5186c3d978e05a90d975093dcf6"
else
    echo "Downloading workbenches manifests for RHOAI"
    KUBEFLOW_REPO_URL="https://github.com/red-hat-data-services/kubeflow"
    KUBEFLOW_COMMIT_SHA="576ed1b4beceb2bae931b64210842912edc8aa26"
    NOTEBOOKS_REPO_URL="https://github.com/red-hat-data-services/notebooks"
    NOTEBOOKS_COMMIT_SHA="2fd3c903d22110d8199c1d4683209c4e092f0b57"
fi

# --------------------------------------------------------------------------
# Fetch kubeflow repo (odh-notebook-controller + kf-notebook-controller)
# --------------------------------------------------------------------------

KF_LOCAL="${PROJECT_ROOT}/../kubeflow"

if [[ "${USE_LOCAL:-}" == "true" ]] && [[ -d "${KF_LOCAL}" ]]; then
    echo "Copying kubeflow manifests from adjacent checkout"
    cp -a "${KF_LOCAL}/components/odh-notebook-controller/config/." "${DST_MANIFESTS_DIR}/odh-notebook-controller/"
    cp -a "${KF_LOCAL}/components/notebook-controller/config/." "${DST_MANIFESTS_DIR}/kf-notebook-controller/"
else
    TMP_KF=$(mktemp -d -t "odh-workbenches-kubeflow.XXXXXXXXXX")
    trap 'rm -rf -- "${TMP_KF}"' EXIT

    git -C "${TMP_KF}" init -q
    git -C "${TMP_KF}" remote add origin "${KUBEFLOW_REPO_URL}"
    git -C "${TMP_KF}" fetch --depth 1 -q origin "${KUBEFLOW_COMMIT_SHA}"
    git -C "${TMP_KF}" reset -q --hard "${KUBEFLOW_COMMIT_SHA}"

    mkdir -p "${DST_MANIFESTS_DIR}/odh-notebook-controller"
    mkdir -p "${DST_MANIFESTS_DIR}/kf-notebook-controller"
    cp -a "${TMP_KF}/components/odh-notebook-controller/config/." "${DST_MANIFESTS_DIR}/odh-notebook-controller/"
    cp -a "${TMP_KF}/components/notebook-controller/config/." "${DST_MANIFESTS_DIR}/kf-notebook-controller/"
fi

# --------------------------------------------------------------------------
# Fetch notebooks repo (workbench ImageStreams)
# --------------------------------------------------------------------------

NB_LOCAL="${PROJECT_ROOT}/../notebooks"

if [[ "${USE_LOCAL:-}" == "true" ]] && [[ -d "${NB_LOCAL}" ]]; then
    echo "Copying notebooks manifests from adjacent checkout"
    cp -a "${NB_LOCAL}/manifests/." "${DST_MANIFESTS_DIR}/notebooks/"
else
    TMP_NB=$(mktemp -d -t "odh-workbenches-notebooks.XXXXXXXXXX")
    trap 'rm -rf -- "${TMP_NB}"' EXIT

    git -C "${TMP_NB}" init -q
    git -C "${TMP_NB}" remote add origin "${NOTEBOOKS_REPO_URL}"
    git -C "${TMP_NB}" fetch --depth 1 -q origin "${NOTEBOOKS_COMMIT_SHA}"
    git -C "${TMP_NB}" reset -q --hard "${NOTEBOOKS_COMMIT_SHA}"

    mkdir -p "${DST_MANIFESTS_DIR}/notebooks"
    cp -a "${TMP_NB}/manifests/." "${DST_MANIFESTS_DIR}/notebooks/"
fi

echo "Workbenches manifests downloaded to ${DST_MANIFESTS_DIR}"
