# opendatahub-module-operator

Monorepo for ODH module operators. Every runnable operator now lives under
`modules/`, including the example operator at
`modules/opendatahub-mymodule-operator/`.

The repository root is for monorepo orchestration, documentation, and agent
guidance. It is not an operator project.

## Repository Layout

- `modules/opendatahub-mymodule-operator/` — runnable example module operator
- `modules/opendatahub-ray-operator/` and other `modules/*/` directories — real split module operators
- `docs/` — split plan, lessons learned, and reference docs
- `.agents/skills/` and `.claude/skills/` — development guidance for module extraction and maintenance

## Prerequisites

- Go 1.25+
- podman
- kubectl
- Access to an **OpenShift** cluster (CRC, ROSA, or dev) for module integration/e2e tests

## Working On A Module

Run build, test, deploy, and chart commands from the module directory you are
working on.

Example:

```sh
cd modules/opendatahub-mymodule-operator
make test
make helm
make test-e2e
```

For e2e verification, prefer a fresh ephemeral `ttl.sh` image tag per run
instead of reusing a stable image tag. That is the most reliable way to avoid
stale image cache issues on the cluster while iterating locally.

For step-by-step image build, deploy, and e2e flows, see
`.agents/skills/odh-component-to-module/references/e2e-workflow.md`.

## Root Make Targets

The root `Makefile` contains aggregate monorepo targets only.

| Target | Purpose |
|---|---|
| `make list-modules` | Print the module directories covered by aggregate targets |
| `make test-modules` | Run module unit-test workflows across the tracked module set |
| `make lint-modules` | Run golangci-lint across the tracked module set |
| `make verify-all` | Run the standard aggregate verification suite |
| `make help` | Show all root targets |

## CI Model

- Root workflows run aggregate checks or target explicit module directories.
- Module-specific build, deploy, chart, and image jobs run from the relevant
  module directory.
- The example operator is treated like any other module for CI purposes.

## Reference Docs

- `docs/index.md` — high-level split index and reference map
- `docs/plan.md` — module split architecture and execution notes
- `docs/testing-limitations.md` — cluster/testing expectations
- `.agents/skills/odh-component-to-module/SKILL.md` — monolith component to module workflow

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
