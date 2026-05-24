# opendatahub-module-operator

Template project for building ODH Module Operators — standalone Kubernetes controllers that implement the [ODH Module Operator Contract](https://github.com/opendatahub-io/opendatahub-operator).

## Prerequisites

- Go 1.25+
- podman
- kubectl
- Access to an **OpenShift** cluster (CRC, ROSA, or dev) for module integration/e2e tests
- Optional: Kind for root reference operator local dev only

## Container Image Configuration

The `IMG` variable controls the container image used for building, pushing, and deploying.
It defaults to `ttl.sh/opendatahub-module-operator:1h` (an anonymous, ephemeral registry suitable for development).

Override `IMG` for your environment using one of these methods:

### Using direnv (recommended)

Create or edit `.envrc` in the project root:

```sh
export IMG=quay.io/myorg/opendatahub-module-operator:latest
```

Then run `direnv allow`. All `make` targets will use your image automatically.

### Using a local Makefile override

Create a `local.mk` file (gitignored) in the project root:

```makefile
IMG = quay.io/myorg/opendatahub-module-operator:latest
```

The Makefile includes `local.mk` if present, overriding defaults without modifying tracked files.

### Inline override

```sh
make container-build container-push IMG=quay.io/myorg/opendatahub-module-operator:v0.1.0
```

### Development with ttl.sh

[ttl.sh](https://ttl.sh) is a free, anonymous container registry where images expire after a TTL.
No authentication required — useful for quick development and testing:

```sh
make container-build container-push IMG=ttl.sh/my-module:1h
make deploy IMG=ttl.sh/my-module:1h
```

## Getting Started

### Build and test

```sh
make build
make test
```

### Deploy to a cluster

```sh
make container-build container-push
make deploy
```

### Apply a sample CR

```sh
kubectl apply -k config/samples/
```

### Run locally (against current kubeconfig)

```sh
make run
```

## Local Development

### OpenShift (recommended for module tests)

Module operators under `modules/` expect OpenShift (SCC and other APIs present).
Use `oc login` or a kubeconfig for your cluster, then from the module directory:

```sh
make test-integration          # cleanup + in-process manager
make test-e2e                  # cleanup + deploy + e2e (full cycle)
```

Root reference operator (this repo):

```sh
make cleanup-integration
make test-integration-run
# e2e: see e2e-workflow skill — export IMG, make helm, deploy-helm, test-e2e-run
make test-e2e                    # full cycle
```

For step-by-step debugging (build, push, deploy, test separately), see
`.agents/skills/odh-component-to-module/references/e2e-workflow.md`.

### Kind (optional — root reference operator only)

```sh
make kind-create          # Create a Kind cluster (podman provider)
make container-build container-push
make deploy
make kind-delete          # Tear down the cluster
```

### Avoiding image cache issues

When iterating locally, Kubernetes may serve a cached image even after a
rebuild if the tag hasn't changed. Two approaches:

**Unique tags with `uuidgen`** (recommended):

```sh
IMG=ttl.sh/opendatahub-module-operator-$(uuidgen):1h \
  make container-build container-push deploy-helm
```

**Force pull via Helm value** (the chart defaults to `Always`, but if
overridden):

```sh
make deploy-helm HELM_EXTRA_ARGS="--set image.pullPolicy=Always"
```

## Helm Chart

Generate the Helm chart from kustomize output:

```sh
make helm
```

The chart is generated at `config/chart/chart/`. Deploy via Helm:

```sh
make helm-deploy
make helm-status
make helm-uninstall
```

## Key Make Targets

| Target | Purpose |
|---|---|
| `make build` | Build manager binary |
| `make test` | Run unit tests |
| `make test-integration` | Integration tests (OpenShift; in-process manager) |
| `make test-e2e` | E2E tests (OpenShift; deployed operator) |
| `make manifests generate` | Regenerate CRDs, RBAC, deepcopy |
| `make container-build` | Build container image |
| `make container-push` | Push container image |
| `make deploy-kustomize` | Deploy via kustomize |
| `make deploy-helm` | Deploy via Helm |
| `make helm` | Generate Helm chart |
| `make helm-deploy` | Deploy via Helm |
| `make kind-create` | Create Kind cluster (podman) |
| `make kind-delete` | Delete Kind cluster |
| `make lint` | Run golangci-lint |
| `make help` | Show all targets |

## License

Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
