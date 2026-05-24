# E2E and container workflow

Run from `modules/$MODULE_NAME/` against **OpenShift**. If you are using a tool
that supports `working_directory`, point it at that module path before running
any integration/e2e/deploy target. The repo root defines the same target names
for `opendatahub-module-operator`, so running them from the wrong directory can
build, deploy, or test the wrong operator. Do not chain build, push, deploy,
and test in one command — run each step separately to inspect failures.

## Compute IMG once

```bash
export IMG="ttl.sh/${MODULE_NAME}-$(uuidgen | tr '[:upper:]' '[:lower:]'):1h"
echo "IMG=${IMG}"
```

Use the same `IMG` for `container-build`, `container-push`, and `deploy-helm`.

## Integration tests (in-process manager)

Prepare once, then run tests only:

```bash
make manifests generate
make cleanup-integration
make test-integration-run
```

Or all-in-one: `make test-integration` (runs cleanup + tests).

## E2E tests (deployed operator)

One step at a time after exporting `IMG`:

```bash
export ODH_PLATFORM_TYPE=OpenDataHub   # when fetching ODH manifests
make container-prep       # host: manifests + generate + get-manifests
echo "IMG=${IMG}"
make container-build      # host prep (if needed) + image build; binary compiled in container
make container-push
make helm                   # generates config/chart (runs manifests generate)
make cleanup-e2e
make deploy-helm
make test-e2e-run           # go test only — operator already deployed
```

`container-build` depends on `container-prep`, so you can skip the explicit
`container-prep` line unless you want to inspect generated output first.
Only **`make build-bin`** runs inside the Containerfile — not manifests,
generate, or get-manifests.

Use **`test-e2e-run`**, not `make test-e2e`, after manual deploy — `test-e2e`
re-runs `cleanup-e2e` and `deploy-helm`.

All-in-one (CI or repeat full cycle): `make test-e2e`.

## Makefile targets

Every module should define:

| Target | Purpose |
|--------|---------|
| `cleanup-integration` | Cluster cleanup before integration; delete module CRs and wait before CRD removal |
| `cleanup-e2e` | Uninstall operator + CRs before e2e; delete module CRs and wait before CRD removal |
| `test-integration-run` | `go test ./test/integration/` only |
| `test-integration` | `cleanup-integration` + `test-integration-run` (+ deps) |
| `test-e2e-run` | `go test ./test/e2e/` only |
| `test-e2e` | `cleanup-e2e deploy-helm test-e2e-run` |

## Anti-patterns

| Do not | Do instead |
|--------|------------|
| `IMG=... make container-build container-push deploy-helm test-e2e` | One target per line |
| `make test-e2e` after manual `deploy-helm` | `make test-e2e-run` |
| `deploy-helm` without `make helm` | `make helm` first (or after manifest changes) |
| Run module test/deploy targets from repo root | Run them from `modules/$MODULE_NAME/` |
| Skip `echo $IMG` | Print tag in logs |

See [testing.md](testing.md) for timeouts and test code patterns.
