# E2E and container workflow

Run from `modules/$MODULE_NAME/` against **OpenShift**. If you are using a tool
that supports `working_directory`, point it at that module path before running
any integration/e2e/deploy target. The repo root defines the same target names
for `opendatahub-module-operator`, so running them from the wrong directory can
build, deploy, or test the wrong operator. In particular, a first-time e2e run
from the repo root can install the root `opendatahub-module-operator` chart
instead of the module chart. Do not chain build, push, deploy, and test in one
command -- run each step separately to inspect failures.

Before the first `container-build`, `helm`, `deploy-crc`, or `deploy-helm`, verify that the
current directory is the module directory you intend to test.

## Compute IMG once

```bash
export IMG="${MODULE_NAME}:dev"
echo "IMG=${IMG}"
```

Use the same `IMG` for `container-build`, `deploy-crc`, and `deploy-helm`.
Keep it in shell memory and pass it directly to `make`, for example
`make container-build IMG="${IMG}"` or `IMG="${IMG}" make container-build`.
Do not write it to a temp file for later `cat`.

On CRC, `deploy-crc` pushes the current `IMG` to the OpenShift internal
registry and then runs the Helm deploy with the in-cluster pullspec. That is
the preferred local OpenShift workflow.

## Integration tests (in-process manager)

Prepare once, then run tests only:

```bash
make prepare-integration
make test-integration-run
```

Or all-in-one: `make test-integration` (runs cleanup, installs CRDs, then tests).

## E2E tests (deployed operator, CRC-first)

One step at a time after exporting `IMG`:

```bash
pwd   # must end with /modules/$MODULE_NAME
export ODH_PLATFORM_TYPE=OpenDataHub   # when fetching ODH manifests
make container-prep       # host: manifests + generate + get-manifests
echo "IMG=${IMG}"
make container-build IMG="${IMG}"      # host prep (if needed) + image build; binary compiled in container
make helm                   # generates config/chart (runs manifests generate)
make cleanup-e2e
make deploy-crc IMG="${IMG}"           # push to CRC registry + helm deploy
make test-e2e-run           # go test only -- operator already deployed
make cleanup-e2e
```

If you are validating against a non-CRC OpenShift cluster, replace
`deploy-crc` with `deploy-helm IMG="${IMG}"` and make sure `IMG` is already a
cluster-reachable image reference.

When using `deploy-helm` (instead of `deploy-crc`), always pass the platform
config flags that `make test-e2e` sets automatically, otherwise the operator
ConfigMap will have empty defaults and tests like `testOperatorConfigMap` and
`testReleaseStatus` will fail:

```bash
make deploy-helm IMG="${IMG}" \
  HELM_EXTRA_ARGS="--set-string config.platform-name=OpenDataHub --set-string config.platform-version=<version>"
```

Check the module's `Makefile` `test-e2e` target for the exact flag names and
default values. **The right `config.platform-version` depends on which tests
you are running** — tests that exercise upgrade paths expect a specific
version sequence (e.g. deploy at version N-1, then upgrade to N). Use the
version the test expects at the deploy step, not an arbitrary value.

On the **first** cluster verify pass, keep tool timeouts short so obvious
problems fail quickly. If `deploy-helm` or `test-e2e-run` stops making progress,
check operator logs immediately instead of waiting indefinitely for the outer
command timeout.

`container-build` depends on `container-prep`, so you can skip the explicit
`container-prep` line unless you want to inspect generated output first.
Only **`make build-bin`** runs inside the Containerfile -- not manifests,
generate, or get-manifests.

Use **`test-e2e-run`**, not `make test-e2e`, after manual deploy -- `test-e2e`
re-runs `cleanup-e2e` and deploy.

All-in-one (CI or repeat full cycle): `make test-e2e`.

## Makefile targets

Every module should define:

| Target | Purpose |
|--------|---------|
| `cleanup-integration` | Cluster cleanup before integration; delete module CRs and wait before CRD removal |
| `prepare-integration` | Cleanup + CRD install before `test-integration-run` |
| `cleanup-e2e` | Uninstall operator + CRs before e2e; delete module CRs and wait before CRD removal |
| `deploy-crc` | Push `IMG` to the CRC internal registry and deploy Helm with that internal pullspec |
| `test-integration-run` | `go test ./test/integration/` only |
| `test-integration` | `prepare-integration` + `test-integration-run` (+ deps) |
| `test-e2e-run` | `go test ./test/e2e/` only |
| `test-e2e` | `cleanup-e2e deploy-helm test-e2e-run` or module-specific equivalent |

## Anti-patterns

| Do not | Do instead |
|--------|------------|
| `IMG=... make container-build deploy-crc test-e2e` | One target per line |
| `make container-build IMG="$(cat /tmp/img)"` | `make container-build IMG="${IMG}"` |
| Run `make container-build`, `make helm`, `make deploy-crc`, or `make deploy-helm` from the repo root | Run them from `modules/$MODULE_NAME/` with tool `working_directory` set there |
| `make test-e2e` after manual `deploy-helm` | `make test-e2e-run` |
| `deploy-crc` or `deploy-helm` without `make helm` | `make helm` first (or after manifest changes) |
| `make test-integration-run` on a dirty cluster | `make prepare-integration` first |
| Run module test/deploy targets from repo root | Run them from `modules/$MODULE_NAME/` |
| Skip `echo $IMG` | Print tag in logs |

See [testing.md](testing.md) for timeouts and test code patterns.
