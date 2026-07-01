# Embed `helmtemplate-generator` In Module `chartgen`

## Goal
Keep the current `make helm` and per-module `cmd/chartgen` UX, but reduce handwritten replacement logic by using `helmtemplate-generator` as an internal transform engine.

## Pilot Scope
Start with one representative module such as [`modules/opendatahub-spark-operator`](modules/opendatahub-spark-operator). Do not roll out repo-wide until the hybrid approach proves that generated chart output remains equivalent for the easy cases.

## Current Integration Point
The current flow already has a stable orchestration point in [`modules/opendatahub-spark-operator/cmd/chartgen/chartgen.go`](modules/opendatahub-spark-operator/cmd/chartgen/chartgen.go):

```40:53:modules/opendatahub-spark-operator/cmd/chartgen/chartgen.go
	groups := groupByGVK(resources)
	values := ExtractDefaults(resources)
	...
	if err := WriteValuesYAML(values, filepath.Join(outputDir, valuesYAMLFilename)); err != nil {
		return fmt.Errorf("writing %s: %w", valuesYAMLFilename, err)
	}

	if err := WriteValuesSchema(filepath.Join(outputDir, valuesSchemaFile)); err != nil {
		return fmt.Errorf("writing %s: %w", valuesSchemaFile, err)
	}
```

That means the new library should be inserted only in the manifest transformation phase, not in the higher-level chart generation flow.

## Target Architecture
Keep the pipeline structure and replace only selected transformation logic:

1. `chartgen` reads multi-doc YAML from `kustomize build`.
2. `chartgen` still groups by GVK and writes chart files in the existing layout.
3. `chartgen` builds an in-memory `helmtemplate-generator` config for straightforward rewrites.
4. `chartgen` runs the embedded transformer on target resources or resource groups.
5. `chartgen` applies repo-specific post-processing for cases that remain awkward or unsupported.
6. `chartgen` continues writing `Chart.yaml`, `_helpers.tpl`, `values.yaml`, and `values.schema.json`.

## First Transforms To Migrate
Move the simplest and most repetitive functions out of [`modules/opendatahub-spark-operator/cmd/chartgen/chart.go`](modules/opendatahub-spark-operator/cmd/chartgen/chart.go) first:
- `replaceNamespace`
- `replaceImageField`
- `replaceReplicas`
- `replaceResources`
- `addImagePullSecrets`
- optionally the simpler parts of service account annotation templating

These are the best fit because they are already deterministic field rewrites or injected Helm blocks.

## Keep Custom Initially
Do not migrate these in the first pass:
- `injectConfigMapValues`
- `replaceSubjectsNamespace`
- `replaceSubjectsServiceAccount`
- `replaceWebhookNamespace`
- `replaceCertificateDNSNames`
- everything in [`modules/opendatahub-spark-operator/cmd/chartgen/values.go`](modules/opendatahub-spark-operator/cmd/chartgen/values.go)

These encode repo-specific semantics and are the most likely to become less readable if forced into generic rule config.

## Implementation Shape
Introduce a small adapter layer in each module or, preferably, a shared internal package that:
- converts a resource or rendered YAML string into `helmtemplate-generator` input
- constructs the transform rules programmatically rather than depending on external config files
- returns transformed YAML back to existing `renderGroup()` / `transformResource()` logic

Suggested internal boundary:
- existing `transformResource()` decides whether a resource is handled by the embedded transformer or by custom code
- embedded-transform path handles standard rewrite rules
- custom path remains for cert-manager, webhook, RBAC subject, and ConfigMap-specific logic

## Dependency And Adoption Checks
Before implementation, explicitly validate:
- whether the repo is comfortable depending on a module with `go 1.25.8`
- whether the project's missing/placeholder license metadata is acceptable; if not, vendor/forking or upstream follow-up may be required before adoption
- whether embedding the library creates any conflicts with current YAML handling and output expectations

## Verification Plan
For the pilot module:
- run `make helm`
- compare generated `config/chart` output before and after the change
- verify that only the intended template sections change, if any
- confirm `values.yaml` and `values.schema.json` are unchanged
- spot-check generated templates for:
  - deployment image, replicas, resources
  - release namespace injection
  - `imagePullSecrets`
  - unchanged cert-manager / webhook / RBAC / ConfigMap special handling

## Rollout Strategy
After the pilot succeeds:
- extract the embedding adapter into a shared internal package to avoid repeating integration code across modules
- move the same low-risk rewrite set across the remaining modules
- leave special-case logic custom until the embedded library proves it improves clarity rather than obscuring behavior
- maintain a short upstream wishlist for features that would let more repo logic migrate later

## Success Criteria
The hybrid embedding is successful if it:
- preserves the current module-local `chartgen` contract and output layout
- removes a meaningful portion of handwritten field-replacement logic
- keeps values extraction and schema generation unchanged
- does not make cert-manager, webhook, RBAC, or ConfigMap behavior harder to reason about
