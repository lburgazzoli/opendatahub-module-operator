# AGENTS.md

Module-specific guidance for `modules/opendatahub-db-operator`.

## Scope

This module manages PostgreSQL access through:

- `SchemaClaim`
- `DatabaseClaim`
- `DatabaseProvider`
- `DatabaseService`

## Rules

- Run module commands from `modules/opendatahub-db-operator/`.
- After changing API types or kubebuilder markers, run `make manifests generate`.
- Do not hand-edit generated files:
  - `api/**/zz_generated.deepcopy.go`
  - `config/crd/bases/*.yaml`
- Keep embedded-provider behavior split clearly from external-provider behavior.
- Prefer shared helpers in `pkg/controller` or `pkg/postgres` over duplicating
  provider or connection logic in controllers.
- Claims write connection Secrets in the claim namespace. The Secret name
  defaults to the claim name but may be overridden via `spec.secretName`.
- Embedded resources default to the operator namespace unless
  `spec.embedded.namespace` overrides it.

## Verification

Before finishing substantial changes, prefer:

- `make manifests generate`
- `make lint`
- focused `go test` or `make test`
- `make test-integration` when the change affects controller behavior
