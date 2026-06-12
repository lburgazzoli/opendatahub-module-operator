# Extending the Module Operator

## Adding a Custom Action

```go
func myAction(ctx context.Context, rr *odhtypes.ReconciliationRequest) error {
    module := rr.Instance.(*componentApi.MyModule)
    // modify module.Status, rr.Resources, etc.
    return nil
}
```

Insert via `WithAction(myAction)` at the desired pipeline position. Actions
receive the shared `ReconciliationRequest` state bag and can read/write
`rr.Manifests`, `rr.Resources`, and `rr.Instance`.

## Adding a New Module Kind

For a **split module** under `modules/`, use
[odh-module-migrate](../../odh-module-migrate/SKILL.md) -- do not
hand-scaffold from mymodule.

To extend the **example module** only (`modules/opendatahub-mymodule-operator/`):

1. `kubebuilder create api --group components --version v1alpha1 --kind NewModule --namespaced=false`
2. Replace types with PlatformObject contract (embed `common.Status` +
   `common.ComponentReleaseStatus`)
3. Add CEL singleton validation marker
4. Create `internal/controller/newmodule/` (controller, actions, support)
5. Pass `*moduleconfig.Config` to `NewReconciler(ctx, mgr, cfg)`
6. Register in `cmd/operator/operator.go`
7. `make manifests generate build test`

## Build Metadata

The Makefile injects version info via `-ldflags` into `pkg/module`:

```
-X pkg/module.Version=$(VERSION)
-X pkg/module.Commit=$(GIT_COMMIT)
-X pkg/module.Branch=$(GIT_BRANCH)
-X pkg/module.Repo=$(GIT_REPO)
```

These surface in the `reportStatus` action as `status.module.version` and
`status.module.buildSource`.
