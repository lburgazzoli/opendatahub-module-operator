---
name: odh-module-test
description: >
  Testing guide for ODH Module Operators. Covers unit, integration, and e2e
  test structure, OpenShift assumptions, cleanup patterns, timeouts, and
  common pitfalls. Use when writing tests, running tests, or debugging test
  failures.
user-invocable: true
allowed-tools:
  - Read
  - Glob
  - Grep
  - Bash
  - Write
  - Edit
---

# ODH Module Operator Testing Guide

## OpenShift Assumptions

Integration and e2e tests target **OpenShift** (ROSA, shared dev cluster). OpenShift APIs (SCC, Route, etc.) are available natively -- no
synthetic CRDs needed. Use an OpenShift kubeconfig (`oc login` or existing
context).

## Test Framework

Go `testing` package + Gomega + gomega-matchers. No Ginkgo BDD. Use dot
imports for Gomega.

Canonical pattern:

```go
import (
    . "github.com/onsi/gomega"
    k8sm "github.com/lburgazzoli/gomega-matchers/pkg/matchers/k8s"
    "github.com/lburgazzoli/gomega-matchers/pkg/matchers/jq"
)

func TestRay(t *testing.T) {
    t.Run("should become ready", func(t *testing.T) {
        g := NewWithT(t)
        g.Eventually(k.Get(obj)).WithContext(ctx).WithTimeout(timeout).WithPolling(interval).Should(
            jq.Match(`.status.phase == "Ready"`),
        )
    })
}
```

Key constants for test timeouts:

```go
const (
    timeout  = 90 * time.Second  // max wait per Eventually
    interval = 2 * time.Second
)
```

## Make Targets

Run these from the **module directory** (`modules/$name/`), not the repo root.

| Target | What it does |
|---|---|
| `make test` | Unit tests (compilation, vet) |
| `make test-integration` | `test-integration-setup` + `test-integration-run` |
| `make test-integration-setup` | `cleanup-integration` + CRD install |
| `make test-integration-run` | Run integration tests only (cluster must be prepared) |
| `make test-e2e` | `cleanup-e2e` + `deploy-helm` + `test-e2e-run` |
| `make test-e2e-run` | Run e2e tests only (operator must be deployed) |
| `make cleanup-integration` | Remove integration test leftovers from cluster |
| `make cleanup-e2e` | Uninstall operator and remove e2e leftovers |

Always use `-timeout 5m -failfast` in Makefile test commands.

## Reference Files

- [references/testing-structure.md](references/testing-structure.md) --
  Detailed test code structure: unit test checklist, integration TestMain
  16-step structure, e2e test structure, module-specific test patterns,
  and workload resource naming.

- [references/testing-ops.md](references/testing-ops.md) --
  Operational concerns: OpenShift assumptions, pre-test cleanup (Makefile
  targets, cleanup scripts, in-process cleanup), container build workflow,
  test timeouts, and common pitfalls.
