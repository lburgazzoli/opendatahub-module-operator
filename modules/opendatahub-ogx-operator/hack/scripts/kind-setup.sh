#!/usr/bin/env bash
# Create a Kind cluster with podman provider and install cert-manager.
#
# Usage:
#   hack/scripts/kind-setup.sh [cluster-name] [cert-manager-version]
#
# Environment:
#   KIND                 - kind command (default: go run sigs.k8s.io/kind@v0.31.0)
#   HELM                 - helm command (default: go run helm.sh/helm/v4/cmd/helm@v4.2.0)
#   CONTAINER_TOOL       - container runtime (default: podman)
#   KIND_CLUSTER         - cluster name (default: opendatahub-ogx-operator)
#   KIND_NODE_IMAGE      - node image for the Kind cluster (optional, e.g. kindest/node:v1.32.3)
#   CERT_MANAGER_VERSION - cert-manager version (default: v1.17.2)

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

KIND="${KIND:-go run sigs.k8s.io/kind@v0.31.0}"
HELM="${HELM:-go run helm.sh/helm/v4/cmd/helm@v4.2.0}"
CONTAINER_TOOL="${CONTAINER_TOOL:-podman}"
CLUSTER="${1:-${KIND_CLUSTER:-opendatahub-ogx-operator}}"
NODE_IMAGE="${KIND_NODE_IMAGE:-}"
CM_VERSION="${2:-${CERT_MANAGER_VERSION:-v1.17.2}}"

# Check if the cluster already exists
if ${KIND} get clusters 2>/dev/null | grep -qw "${CLUSTER}"; then
    echo "Kind cluster '${CLUSTER}' already exists. Skipping creation."
    exit 0
fi

KIND_ARGS=(--name "${CLUSTER}")
if [ -n "${NODE_IMAGE}" ]; then
    KIND_ARGS+=(--image "${NODE_IMAGE}")
fi

echo "Creating Kind cluster '${CLUSTER}' with ${CONTAINER_TOOL} provider..."
KIND_EXPERIMENTAL_PROVIDER="${CONTAINER_TOOL}" ${KIND} create cluster "${KIND_ARGS[@]}"

echo "Installing cert-manager ${CM_VERSION}..."
${HELM} repo add jetstack https://charts.jetstack.io --force-update
${HELM} install cert-manager jetstack/cert-manager \
    --namespace cert-manager \
    --create-namespace \
    --version "${CM_VERSION}" \
    --set crds.enabled=true \
    --wait \
    --timeout 5m



# Export kubeconfig to .kube/config for the project
KUBECONFIG_DIR=".kube"
mkdir -p "${KUBECONFIG_DIR}"
KIND_EXPERIMENTAL_PROVIDER="${CONTAINER_TOOL}" ${KIND} get kubeconfig --name "${CLUSTER}" 2>/dev/null > "${KUBECONFIG_DIR}/config"
echo "Kubeconfig written to ${KUBECONFIG_DIR}/config"

echo "Kind cluster '${CLUSTER}' ready with cert-manager ${CM_VERSION}."
