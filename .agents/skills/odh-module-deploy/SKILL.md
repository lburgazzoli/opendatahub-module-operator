---
name: odh-module-deploy
description: >
  Build, package, and deploy ODH Module Operators. Covers Containerfile
  conventions, Helm chart generation, deploy workflow, and image
  management. Use when building images, generating charts, or deploying to
  OpenShift.
user-invocable: true
allowed-tools:
  - Read
  - Glob
  - Grep
  - Bash
  - Write
  - Edit
---

# ODH Module Deploy

## Helm Chart Generation

`cmd/chartgen/` reads multi-doc YAML from stdin and generates a Helm chart.
Resources are grouped by GVK into files named `<group>_<version>_<kind>.yaml`.

For split modules, `cmd/chartgen/` must import the module-local
`pkg/resources/gvk/gvk.go` package. Do not import
`github.com/opendatahub-io/opendatahub-operator/v2/pkg/cluster/gvk`
directly from chartgen files.

Transformations:

- **Deployment**: image from `image.fullRef`, resources, replicas,
  serviceAccountName, imagePullSecrets from values, and
  `imagePullPolicy: Always`
- **ServiceAccount**: name from values, annotations from values
- **ConfigMap**: merges `.Values.config` and `.Values.imagePullSecret` into data
- **RoleBinding/ClusterRoleBinding**: subjects namespace and SA name from values
- **Namespaced resources**: `metadata.namespace` -> `{{ .Release.Namespace }}`
- **Everything else**: passed through as-is

`values.schema.json` is generated via `invopop/jsonschema` reflection on the
`Values` struct in `cmd/chartgen/values.go`. Adding a field to `Values`
automatically updates the schema.

### Maintaining the Chart Generator

Update `cmd/chartgen/` when:

- **New Helm values** -- add a field to `Values` struct in `values.go`.
  The schema updates automatically. Add a `jsonschema` struct tag for
  description/enum/default.
- **New resource types need special templating** -- add a case to
  `transformResource()` in `chart.go`. Tier-1 resources get value injection,
  tier-2 resources get namespace templating only.
- **Namespace templating logic changes** -- edit `replaceNamespace()`,
  `replaceSubjectsNamespace()`, and `replaceSubjectsServiceAccount()` in
  `chart.go`.
- **`_helpers.tpl` or `Chart.yaml` template changes** -- edit the constants
  in `helpers.go`. `Chart.yaml` is only generated if missing.

## Module Chart Conventions

- Use `image.fullRef`, not repository/tag value pairs
- Keep the chart helper responsible for rendering the full image reference
- Keep `imagePullPolicy: Always` in generated Deployments
- After changes: `make helm` regenerates the chart
- After a multi-module rollout, run `make helm` in every module and confirm
  the repo is clean

## References

- [Containerfile and manifest permissions](references/containerfile.md)
- [E2E and container workflow](references/e2e-workflow.md)
- [Image caching](references/image-caching.md)
