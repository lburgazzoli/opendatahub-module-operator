# Test Operations

## OpenShift Assumptions

Integration and e2e tests are run against **OpenShift** (ROSA, shared dev
cluster). Assume:

- **Kubeconfig** points at OpenShift (`oc login` or existing context).
- **OpenShift APIs** (SCC, Route, etc.) exist on the cluster -- no need to
  install synthetic CRDs before tests (see
  [external-crds.md](../../odh-module-migrate/references/external-crds.md)).
- **Platform overlay** in tests matches the cluster: use `rhoai` or `odh` from
  `config/manager/configmap.yaml` based on the OpenShift environment under test.
- **SCC workloads** reconcile normally because OpenShift provides the required APIs.

Use `oc` or `kubectl` interchangeably in cleanup scripts; prefer whatever is
in the user's PATH.

Unsupported: plain Kubernetes without OpenShift APIs (SCC, Route, etc.).

## Pre-test Cleanup (Required)

Integration and e2e tests assume a **clean cluster**. Leftover module CRs,
workload objects, cluster RBAC, or a stale CRD from a failed run cause flaky
or misleading failures. Every module must ship cleanup and run it **before**
`go test` for integration/e2e.

When you are working on `modules/$MODULE_NAME/`, run `make cleanup-*`,
`make test-integration*`, `make deploy-helm`, and
`make test-e2e*` from that
module directory (or set your tool `working_directory` there). The repo root
defines targets with the same names for `opendatahub-module-operator`, so
running them from the wrong directory can deploy or test the wrong operator.

### Makefile Targets

Add cleanup targets plus an integration prep target (copy from
`modules/opendatahub-ray-operator/Makefile` and rename for `$COMPONENT`):

```makefile
.PHONY: cleanup-integration
cleanup-integration: ## Clean up integration test resources from the cluster.
	./hack/scripts/cleanup-integration.sh

.PHONY: cleanup-e2e
cleanup-e2e: ## Clean up e2e test resources and uninstall operator from the cluster.
	./hack/scripts/cleanup-e2e.sh

.PHONY: test-integration-setup
test-integration-setup: manifests generate ## Clean cluster state and install CRDs for integration tests.
	$(MAKE) cleanup-integration
	$(MAKE) install
```

Wire integration prep and e2e cleanup as **dependencies** of the composite test
targets:

```makefile
.PHONY: test-integration-run
test-integration-run: ## Run integration tests only (cluster must be prepared).
	go test ./test/integration/ -tags=integration -v -timeout 5m -failfast

.PHONY: test-integration
test-integration: test-integration-setup test-integration-run ## ...

.PHONY: test-e2e-run
test-e2e-run: ## Run e2e tests only (operator must already be deployed).
	go test ./test/e2e/ -tags=e2e -v -timeout 5m -failfast

.PHONY: test-e2e
test-e2e: cleanup-e2e deploy-helm test-e2e-run ## ...
```

After `deploy-helm`, use **`make test-e2e-run`** -- not
`make test-e2e` (which re-runs cleanup and deploy). See
[e2e-workflow.md](../../odh-module-deploy/references/e2e-workflow.md).

Integration CRDs must be installed by `make`, not by Go test code. `TestMain`
should fail fast if the expected module CRD is missing.

For **dependency-gated modules**, add negative-path integration and e2e tests
that exercise the preconditions directly. Do not stop at a single happy-path
Ready test. If reconciliation depends on another API surface or ownership
contract, cover both:

- the dependency missing path (for example missing operand CRD / required CR)
- the dependency present but unusable path (for example foreign-owned CRD,
  unmanaged singleton, or wrong ownership label)

Prefer assertions on the specific condition/reason set by the gate in addition
to the top-level Ready=False state. When a negative path requires mutating a
cluster-scoped dependency, only do so when the test owns that dependency; if an
external cluster already provides a conflicting shared CRD/CR, skip rather than
rewriting shared state.

### Cleanup Scripts

Copy `hack/scripts/cleanup-integration.sh` and `cleanup-e2e.sh` from the ray
module and update for `$KIND` / `$COMPONENT`:

| Script | Namespace default | Deletes |
|--------|-------------------|---------|
| `cleanup-integration.sh` | `$MODULE_NAME-integration` | Module CRs (cluster-scoped), waits for CR deletion, workload + RBAC in test namespace, `part-of=$COMPONENT` ClusterRoles/Bindings, integration test RBAC, module CRD |
| `cleanup-e2e.sh` | `$MODULE_NAME-system` | Module CRs, waits for CR deletion, Helm release uninstall, operator namespace, leftover cluster RBAC, module CRD |

Use `--ignore-not-found` on all `kubectl delete` calls so cleanup is idempotent.
Delete the module CRs **before** any CRD removal path and wait for them to
disappear first, otherwise Helm uninstall or direct CRD deletion can strand
terminating CRs behind a missing CRD.

After [renaming](../../odh-module-migrate/references/renaming.md), grep
the scripts for stale ray names -- they are easy to miss:

```bash
rg 'ray|opendatahub-ray' hack/scripts/cleanup-*.sh && exit 1 || true
```

### In-process Cleanup (Integration Only)

`TestMain` in integration tests should **also** delete leftovers via the API
client (defense in depth if someone runs `go test` without `make`). Do **not**
install CRDs in Go; instead, assert the module CRD is already present and fail
fast if it is missing:

```go
// After EnsureNamespace, before starting the manager:
moduleCRD := &apiextensionsv1.CustomResourceDefinition{ObjectMeta: metav1.ObjectMeta{Name: moduleCRDName}}
if err := directClient.Get(ctx, client.ObjectKeyFromObject(moduleCRD), moduleCRD); err != nil {
    fmt.Fprintf(os.Stderr, "expected CRD %s to be installed before running integration tests: %v\n", moduleCRDName, err)
    os.Exit(1)
}
_ = directClient.DeleteAllOf(ctx, &componentsv1alpha1.${Kind}{})
_ = directClient.DeleteAllOf(ctx, &appsv1.Deployment{}, client.InNamespace(testNamespace))
_ = directClient.DeleteAllOf(ctx, &corev1.Service{}, client.InNamespace(testNamespace))
```

E2e tests use a deployed operator -- cluster cleanup is the Makefile/script
responsibility, not `TestMain`.

## Container Build and E2E Workflow

See [e2e-workflow.md](../../odh-module-deploy/references/e2e-workflow.md)
for the full step-by-step sequence (IMG export, `make helm`,
`test-e2e-run` vs `make test-e2e`).

## Test Timeouts (Fast Feedback)

Tests must fail fast -- never wait 30 minutes to learn something hung.

### `go test` Flags (Makefile)

Use **short** package timeouts for local and CI feedback:

```makefile
# Integration / e2e -- 5m is enough when Eventually timeouts are tight
go test ./test/integration/ -tags=integration -v -timeout 5m -failfast
go test ./test/e2e/ -tags=e2e -v -timeout 5m -failfast
```

During active debugging, run an even shorter ad-hoc timeout:

```bash
go test ./test/integration/ -tags=integration -v -timeout 3m -failfast -run TestBecomesReady
```

For the **initial** migration validation run, prefer the short timeout first.
If it fails or stalls, inspect logs and narrow the failing step before trying a
longer rerun.

Always pass **`-failfast`**: the first failing subtest stops the run instead of
cascading hangs.

### `Eventually` Timeouts (Test Code)

Keep per-assertion waits short -- the package timeout is a backstop, not the
primary feedback loop:

```go
const (
    timeout  = 90 * time.Second  // max wait per Eventually (Ready, Deployment, etc.)
    interval = 2 * time.Second
)
```

Use **90s-2m** for `WithTimeout()` in integration/e2e tests. If a reconcile
routinely needs longer, fix the controller or test setup -- do not inflate
timeouts to mask bugs.

E2e operator gate (deployment ready) should use the same `timeout` constant so
a missing operator fails in ~90s, not after the full package timeout.

### When a Test Hangs

1. Note which step last printed (build / push / deploy / which `t.Run`)
2. Re-run only that step with the same `IMG`
3. Lower `-timeout` and `-run` to isolate one test
4. Check operator logs right away: `kubectl logs -n $MODULE_NAME-system deploy/...`
5. Only after logs are understood should you consider a longer rerun

## Common Pitfalls

1. **Dirty cluster**: running integration/e2e without cleanup first -- always
   wire `cleanup-integration` / `cleanup-e2e` into Makefile test targets
2. **Long hangs**: `-timeout 30m` or inflated `Eventually` waits hide bugs -- use
   `-timeout 5m -failfast` in Makefile and `90s-2m` per assertion; run build/
   push/deploy/test as separate steps (see Container build and e2e workflow)
3. **Chained make one-liners** or **`make test-e2e` after manual deploy** --
   use [e2e-workflow.md](../../odh-module-deploy/references/e2e-workflow.md);
   run `test-e2e-run` after deploy
4. **Wrong working directory**: running module integration/e2e/deploy targets
   from the repo root acts on `opendatahub-module-operator` -- run them from
   `modules/$MODULE_NAME/` instead
5. **Missing integration CRD prep**: `test-integration-run` or raw `go test`
   without prior `make test-integration-setup` / `make test-integration` fails
   the CRD presence check
6. **Unused struct fields**: remove `workloadService` etc. from test structs
   if no test asserts on them
7. **Operator gate in e2e**: check operator deployment BEFORE subtests, not
   as a subtest -- a failed subtest doesn't stop subsequent subtests
8. **Wrong env prefix**: deployment sets `ODH_OPERATOR_*` but `pkg/config`
   uses `ODH_MODULE_OPERATOR` -- viper silently ignores mismatched vars
   during `Unmarshal()`. All env vars must use `ODH_MODULE_OPERATOR_`
   (see controller-rules.md).
9. **Racey dependency mutation tests**: if an e2e test mutates a shared
   cluster-scoped dependency (for example a CRD ownership label), pause the
   controller while making the mutation and resume it only after the dependency
   state is ready for reconciliation.
