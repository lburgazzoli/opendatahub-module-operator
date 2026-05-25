#!/usr/bin/env bash
set -euo pipefail
shopt -s nullglob

workspace_dir="${WORKSPACE_DIR:-/workspace}"
output_dir="${OUTPUT_DIR:-/out}"
target_os="${TARGETOS:-linux}"
target_arch="${TARGETARCH:-$(go env GOARCH)}"
version="${VERSION:-0.0.0-dev}"
git_commit="${GIT_COMMIT:-unknown}"
git_branch="${GIT_BRANCH:-unknown}"
git_repo="${GIT_REPO:-unknown}"
excluded_modules="${EXCLUDED_MODULES:-opendatahub-mymodule-operator}"

cd "${workspace_dir}"

mkdir -p "${output_dir}/bin" "${output_dir}/manifests"

modules=()
for module_path in modules/*-operator; do
    [[ -d "${module_path}" ]] || continue

    module="${module_path##*/}"
    case " ${excluded_modules} " in
        *" ${module} "*) continue ;;
    esac

    modules+=("${module}")
done

if [[ ${#modules[@]} -eq 0 ]]; then
    echo "no eligible module operators found under modules/*-operator" >&2
    exit 1
fi

printf '%s\n' "${modules[@]}" | sort > "${output_dir}/modules.txt"

while IFS= read -r module; do
    module_path="modules/${module}"

    echo "==> ${module_path}"

    make -C "${module_path}" manifests generate
    if [[ -x "${module_path}/hack/scripts/get-manifests.sh" ]]; then
        "${module_path}/hack/scripts/get-manifests.sh"
    fi

    make -C "${module_path}" build-bin \
        BIN_DIR="${output_dir}/bin/${module}" \
        BIN_NAME=manager \
        GOOS="${target_os}" \
        GOARCH="${target_arch}" \
        CGO_ENABLED=0 \
        VERSION="${version}" \
        GIT_COMMIT="${git_commit}" \
        GIT_BRANCH="${git_branch}" \
        GIT_REPO="${git_repo}"

    mkdir -p "${output_dir}/manifests/${module}"
    cp -a "${module_path}/config/manifests/." "${output_dir}/manifests/${module}/"
    chmod -R a+rX "${output_dir}/bin/${module}" "${output_dir}/manifests/${module}"
done < "${output_dir}/modules.txt"
