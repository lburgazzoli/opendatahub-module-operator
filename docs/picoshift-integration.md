# Plan: picoshift Integration for OpenShift Testing

## Goal

Add picoshift as an alternative test target for running integration tests
against an OpenShift-like environment. This keeps the existing lightweight
Kind setup for fast iteration while enabling OpenShift-specific testing
when needed.

## Background

### What is picoshift?

[picoshift](https://github.com/jctanner/picoshift) is a lightweight OpenShift
simulator that runs on Kind. It provides:

- **OpenShift CRDs**: Routes, Projects, SCCs, ClusterVersion, OLM types,
  Gateway API, ImageStreams
- **ocp-shim**: Go sidecar that intercepts API server port 6443 to serve
  OpenShift discovery endpoints (so `oc` and operators see "OpenShift")
- **ocp-sim**: Rust controller (kube-rs) providing Route admission, Project
  mirroring, Service CA injection, SCC-like pod mutation, LoadBalancer IP
  assignment, and built-in Gateway API
- **~200 MB RAM** for basic operation (more with Istio stack)

### Why add it?

The current module operator uses standard Kubernetes APIs and works on plain
Kind. However, picoshift adds value for:

1. **Platform detection testing** — verify the operator detects "OpenShift"
   and applies correct overlays (`config/manifests/openshift/`)
2. **SCC-related behavior** — especially for workloads like Ray that need
   SecurityContextConstraints
3. **Route testing** — test Routes instead of Ingresses
4. **Realistic RHOAI environment** — test against the same simulated
   environment the ODH operator uses

## Design

### Cluster Naming

picoshift creates a Kind cluster named `ocp-sim`. This is distinct from the
existing `opendatahub-module-operator` cluster used by `make kind-create`.

### Idempotent Setup Script

A wrapper script `hack/scripts/picoshift-setup.sh` handles:

1. Clone picoshift (if `.context/repos/picoshift` doesn't exist)
2. Run `make init-deps` (if `deps/kind` doesn't exist)
3. Build and create cluster (if cluster `ocp-sim` doesn't exist)
4. Export kubeconfig path

The script is idempotent — running it multiple times is safe and fast after
the initial build.

### Kubeconfig

picoshift writes kubeconfig to `~/.kube/config`. The `test-integration-ocp`
target sets `KUBECONFIG` to this path.

### Auth Mode

picoshift supports three auth modes: `legacy`, `oidc`, `byoidc`. This
integration uses **legacy** (default) for simplicity — it uses `sha256~`
opaque tokens and a built-in login form.

### Gateway

picoshift has a **built-in gateway controller** that provides lightweight
Gateway API support without requiring Istio. This is the default
(`ENABLE_BUILTIN_GATEWAY=true`) and sufficient for testing.

The full Istio + Kuadrant stack (`make gateway-stack`) is NOT included in
the default setup to minimize resource usage.

## Files to Create/Modify

### New Files

| File | Purpose |
|------|---------|
| `hack/scripts/picoshift-setup.sh` | Idempotent setup: clone, init-deps, build, create cluster |
| `hack/scripts/picoshift-teardown.sh` | Tear down picoshift cluster |

### Modified Files

| File | Change |
|------|--------|
| `Makefile` | Add `picoshift-create`, `picoshift-delete`, `test-integration-ocp` targets |
| `AGENTS.md` | Document new targets and prerequisites |

### Already Configured

| File | Status |
|------|--------|
| `.gitignore` | Already ignores `.context/` |

## Implementation Details

### hack/scripts/picoshift-setup.sh

```bash
#!/usr/bin/env bash
# Create a picoshift cluster for OpenShift-like testing.
# Idempotent: skips steps that are already complete.
#
# Usage:
#   hack/scripts/picoshift-setup.sh
#
# Environment:
#   PICOSHIFT_DIR - picoshift clone location (default: .context/repos/picoshift)

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"

PICOSHIFT_REPO="https://github.com/jctanner/picoshift.git"
PICOSHIFT_DIR="${PICOSHIFT_DIR:-${PROJECT_ROOT}/.context/repos/picoshift}"
CLUSTER_NAME="ocp-sim"

# Step 1: Clone picoshift if not present
if [[ ! -d "${PICOSHIFT_DIR}/.git" ]]; then
    echo "Cloning picoshift to ${PICOSHIFT_DIR}..."
    mkdir -p "$(dirname "${PICOSHIFT_DIR}")"
    git clone "${PICOSHIFT_REPO}" "${PICOSHIFT_DIR}"
else
    echo "picoshift already cloned at ${PICOSHIFT_DIR}"
fi

cd "${PICOSHIFT_DIR}"

# Step 2: Initialize dependencies if not present
if [[ ! -d "deps/kind" ]]; then
    echo "Initializing picoshift dependencies..."
    make init-deps
else
    echo "picoshift dependencies already initialized"
fi

# Step 3: Check if cluster exists using picoshift's kind binary
KIND_BIN="deps/kind/bin/kind"
if [[ ! -x "${KIND_BIN}" ]]; then
    echo "Building kind CLI..."
    make kind-cli
fi

if sudo "${KIND_BIN}" get clusters 2>/dev/null | grep -q "^${CLUSTER_NAME}$"; then
    echo "picoshift cluster '${CLUSTER_NAME}' already running"
else
    echo "Building and creating picoshift cluster..."
    sudo make all
fi

# Step 4: Report kubeconfig location
KUBECONFIG_PATH="${HOME}/.kube/config"
echo ""
echo "=== picoshift ready ==="
echo "Cluster: ${CLUSTER_NAME}"
echo "Kubeconfig: ${KUBECONFIG_PATH}"
echo ""
echo "Test with:"
echo "  KUBECONFIG=${KUBECONFIG_PATH} kubectl get nodes"
echo "  oc login --username=admin --password=admin"
```

### hack/scripts/picoshift-teardown.sh

```bash
#!/usr/bin/env bash
# Tear down the picoshift cluster.
#
# Usage:
#   hack/scripts/picoshift-teardown.sh

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"

PICOSHIFT_DIR="${PICOSHIFT_DIR:-${PROJECT_ROOT}/.context/repos/picoshift}"

if [[ ! -d "${PICOSHIFT_DIR}" ]]; then
    echo "picoshift not found at ${PICOSHIFT_DIR}, nothing to tear down"
    exit 0
fi

cd "${PICOSHIFT_DIR}"

echo "Tearing down picoshift cluster..."
sudo make teardown
```

### Makefile Additions

Add after the `##@ Kind` section:

```makefile
##@ picoshift (OpenShift Simulator)

PICOSHIFT_DIR       := .context/repos/picoshift
PICOSHIFT_KUBECONFIG := $(HOME)/.kube/config

.PHONY: picoshift-create
picoshift-create: ## Create picoshift cluster (clones/builds if needed).
	hack/scripts/picoshift-setup.sh

.PHONY: picoshift-delete
picoshift-delete: ## Delete picoshift cluster.
	hack/scripts/picoshift-teardown.sh

.PHONY: test-integration-ocp
test-integration-ocp: ## Run integration tests against picoshift cluster.
	KUBECONFIG=$(PICOSHIFT_KUBECONFIG) $(MAKE) test-integration
```

### AGENTS.md Additions

Add to the **Testing** section (after the existing table):

```markdown
### picoshift (OpenShift Simulator)

For testing against an OpenShift-like environment, use picoshift:

| Target | Scope | Requires |
|---|---|---|
| `make picoshift-create` | Create picoshift cluster | Rust toolchain, rootful podman |
| `make picoshift-delete` | Delete picoshift cluster | — |
| `make test-integration-ocp` | Integration tests against picoshift | `make picoshift-create` |

**Prerequisites (one-time)**:
- Rust toolchain: `curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs | sh`
- Rootful podman (already configured for Kind)

**First build**: ~5-10 minutes (builds Kind fork + Rust simulator image)

**Cluster name**: `ocp-sim` (distinct from `opendatahub-module-operator`)

The picoshift cluster provides OpenShift CRDs, platform detection (operators
see "OpenShift"), Routes, Projects, SCCs, and a built-in Gateway API
controller. It does NOT include Istio/Kuadrant by default.
```

## Prerequisites

### One-Time Setup

Before first use of `make picoshift-create`:

1. **Rust toolchain**:
   ```bash
   curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs | sh
   source ~/.cargo/env
   ```

2. **Rootful podman**: Already configured for Kind usage

3. **Disk space**: ~2 GB for container images

### First Build Time

The first `make picoshift-create` takes **5-10 minutes** to:
- Clone picoshift and its dependencies (kind fork, opendatahub-operator, entra-mock)
- Build the Kind CLI from the fork
- Build the Kind base image (includes ocp-shim)
- Build the Kind node image
- Build the ocp-sim Rust container image
- Create the cluster and deploy components

Subsequent runs are fast — the script skips completed steps.

## Usage

### Create Cluster

```bash
make picoshift-create
```

### Run Tests

```bash
# Against picoshift (OpenShift-like)
make test-integration-ocp

# Against plain Kind (existing)
make test-integration
```

### Delete Cluster

```bash
make picoshift-delete
```

### Verify Cluster

```bash
# Check OpenShift discovery endpoint
kubectl get --raw /.well-known/oauth-authorization-server | jq .

# Check ClusterVersion (simulated 4.20)
kubectl get clusterversion version -o yaml

# Log in with oc
oc login --username=admin --password=admin
```

## Limitations

- **No Istio/Kuadrant by default**: Use picoshift's `make gateway-stack`
  inside `.context/repos/picoshift/` if needed
- **Rootful podman required**: picoshift uses `sudo make all`
- **Shared kubeconfig**: picoshift writes to `~/.kube/config`, which may
  conflict with other clusters. Back up if needed.
- **First build time**: 5-10 minutes, but subsequent runs are fast

## Future Enhancements

- Add `test-e2e-ocp` target for e2e tests against picoshift
- Add optional Istio stack via `PICOSHIFT_GATEWAY_STACK=true`
- Pin picoshift to specific commit/tag for reproducibility
- Add CI job for periodic OpenShift compatibility testing
