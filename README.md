# opendatahub-module-operator

Template project for building ODH Module Operators — standalone Kubernetes controllers that implement the [ODH Module Operator Contract](https://github.com/opendatahub-io/opendatahub-operator).

## Prerequisites

- Go 1.25+
- podman
- kubectl
- Access to a Kubernetes cluster

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

## Local Development with Kind

```sh
make kind-create          # Create a Kind cluster (podman provider)
make container-build container-push
make deploy
make kind-delete          # Tear down the cluster
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
| `make test` | Run tests (envtest + Ginkgo) |
| `make manifests generate` | Regenerate CRDs, RBAC, deepcopy |
| `make container-build` | Build container image |
| `make container-push` | Push container image |
| `make deploy` | Deploy to cluster via kustomize |
| `make undeploy` | Remove from cluster |
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
