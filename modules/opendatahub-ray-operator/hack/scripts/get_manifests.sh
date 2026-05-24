#!/usr/bin/env bash
set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"

GITHUB_URL="https://github.com"
DST_MANIFESTS_DIR="${PROJECT_ROOT}/config/manifests"

# Format: "repo-org:repo-name:ref-name:source-folder"
# ref-name supports: "branch", "tag", or "branch@commit-sha"
declare -A ODH_COMPONENT_MANIFESTS=(
    ["ray"]="opendatahub-io:kuberay:dev@b30c9c1a8c95cf22c2b6fe051193b429f92c69ed:ray-operator/config"
)

declare -A RHOAI_COMPONENT_MANIFESTS=(
    ["ray"]="red-hat-data-services:kuberay:rhoai-3.5-ea.1@3e576f58face1b5a14ae77c8e3270b95b2dbc423:ray-operator/config"
)

if [ "${ODH_PLATFORM_TYPE:-OpenDataHub}" = "OpenDataHub" ]; then
    echo "Cloning manifests for ODH"
    declare -A COMPONENT_MANIFESTS=()
    for key in "${!ODH_COMPONENT_MANIFESTS[@]}"; do
        COMPONENT_MANIFESTS["$key"]="${ODH_COMPONENT_MANIFESTS[$key]}"
    done
else
    echo "Cloning manifests for RHOAI"
    declare -A COMPONENT_MANIFESTS=()
    for key in "${!RHOAI_COMPONENT_MANIFESTS[@]}"; do
        COMPONENT_MANIFESTS["$key"]="${RHOAI_COMPONENT_MANIFESTS[$key]}"
    done
fi

# Allow overwriting repo using flags component=repo
pattern="^[a-zA-Z0-9_.-]+:[a-zA-Z0-9_.-]+:([a-zA-Z0-9_./-]+|[a-zA-Z0-9_./-]+@[a-f0-9]{7,40}):[a-zA-Z0-9_./-]+$"
if [ "$#" -ge 1 ]; then
    for arg in "$@"; do
        if [[ $arg == --* ]]; then
            arg="${arg:2}"
            IFS="=" read -r key value <<< "$arg"
            if [[ -n "${COMPONENT_MANIFESTS[$key]}" ]]; then
                if [[ ! $value =~ $pattern ]]; then
                    echo "ERROR: The value '$value' does not match the expected format."
                    continue
                fi
                COMPONENT_MANIFESTS["$key"]=$value
            else
                echo "ERROR: '$key' does not exist in COMPONENT_MANIFESTS."
                exit 1
            fi
        fi
    done
fi

TMP_DIR=$(mktemp -d -t "odh-ray-manifests.XXXXXXXXXX")
trap '{ rm -rf -- "$TMP_DIR"; }' EXIT

function git_fetch_ref() {
    local repo=$1
    local ref=$2
    local dir=$3

    mkdir -p "$dir"
    pushd "$dir" &>/dev/null
    git init -q

    if [[ $ref =~ ^([a-zA-Z0-9_./-]+)@([a-f0-9]{7,40})$ ]]; then
        local commit_sha="${BASH_REMATCH[2]}"
        git remote add origin "$repo"
        if ! git fetch --depth 1 -q origin "$commit_sha"; then
            echo "ERROR: Failed to fetch from repository $repo"
            popd &>/dev/null
            return 1
        fi
        if ! git reset -q --hard "$commit_sha" 2>/dev/null; then
            echo "ERROR: Commit SHA $commit_sha not found in repository $repo"
            popd &>/dev/null
            return 1
        fi
    else
        if git ls-remote --exit-code "$repo" "refs/tags/$ref" &>/dev/null; then
            git fetch -q --depth 1 "$repo" "refs/tags/$ref" && git reset -q --hard FETCH_HEAD
        elif git ls-remote --exit-code "$repo" "refs/heads/$ref" &>/dev/null; then
            git fetch -q --depth 1 "$repo" "refs/heads/$ref" && git reset -q --hard FETCH_HEAD
        else
            echo "ERROR: '$ref' is not a valid branch, tag, or commit SHA in $repo"
            popd &>/dev/null
            return 1
        fi
    fi

    popd &>/dev/null
}

for key in "${!COMPONENT_MANIFESTS[@]}"; do
    repo_info="${COMPONENT_MANIFESTS[$key]}"
    echo -e "\033[32mCloning repo \033[33m${key}\033[32m:\033[0m ${repo_info}"
    IFS=':' read -r -a parts <<< "${repo_info}"

    repo_org="${parts[0]}"
    repo_name="${parts[1]}"
    repo_ref="${parts[2]}"
    source_path="${parts[3]}"

    repo_url="${GITHUB_URL}/${repo_org}/${repo_name}"
    repo_dir="${TMP_DIR}/${key}"

    if [[ "${USE_LOCAL}" == "true" ]] && [[ -e ../${repo_name} ]]; then
        echo "copying from adjacent checkout ..."
        mkdir -p "${DST_MANIFESTS_DIR}/${key}"
        cp -rf "../${repo_name}/${source_path}"/* "${DST_MANIFESTS_DIR}/${key}"
        continue
    fi

    if ! git_fetch_ref "${repo_url}" "${repo_ref}" "${repo_dir}"; then
        echo "ERROR: Failed to fetch ref '${repo_ref}' from '${repo_url}'"
        exit 1
    fi

    mkdir -p "${DST_MANIFESTS_DIR}/${key}"
    cp -rf "${repo_dir}/${source_path}"/* "${DST_MANIFESTS_DIR}/${key}"
done

echo "Manifests downloaded to ${DST_MANIFESTS_DIR}"
