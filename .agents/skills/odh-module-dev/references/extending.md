# Extending the Module Operator

## Adding a Custom Action

```go
func myAction(ctx context.Context, rr *fwtypes.ReconciliationRequest) error {
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
[odh-module-migrate](../../odh-module-migrate/SKILL.md) and copy from
`modules/opendatahub-ray-operator/` as the canonical template. The example
`opendatahub-mymodule-operator` template no longer exists.

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
