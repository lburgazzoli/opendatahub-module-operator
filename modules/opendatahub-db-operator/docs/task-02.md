# Task 02 — CRD API Types

See `docs/plan.md` §5 for field tables.

## Goal

Define the three infrastructure CRD Go types with status/condition wiring, deepcopy, kubebuilder
markers, and generated CRD YAML — no reconcile logic yet.

## Depends on

Task-01 (module scaffold).

## Key files/packages

- `api/infrastructure/v1alpha1/schemaclaim_types.go`
- `api/infrastructure/v1alpha1/databaseclaim_types.go`
- `api/infrastructure/v1alpha1/databaseprovider_types.go`
- `api/infrastructure/v1alpha1/groupversion_info.go`, `zz_generated.deepcopy.go` (generated)
- `api/components/v1alpha1/databaseservice_types.go` — the module-enablement CR (`docs/plan.md` §4)

## Steps

1. `SchemaClaim`: `Spec{Provider ProviderRef, Schema string, Access AccessMode, DeletionPolicy
   DeletionPolicy}`, `Status{Phase string, Schema string, Conditions []metav1.Condition,
   Connection SchemaConnectionStatus, MatchedProviders []string}`. `ProviderRef{Name string,
   Selector *metav1.LabelSelector}`. `SchemaConnectionStatus{SecretRef
   corev1.LocalObjectReference, Host string, Port int32, Database string, Schema string}`.
2. `DatabaseClaim`: same shape minus `DeletionPolicy` and `Schema`, plus required
   `Spec.Database string`; `Status.Database` echoes it. **Do not reuse `SchemaConnectionStatus`
   for this CRD's `Status.Connection`** — spec.md's `DatabaseClaim` status example has no
   `schema` key under `connection` at all, and reusing the schema-claim struct verbatim would
   silently serialize an always-empty `schema: ""` field, contradicting the spec's shape. Define
   a separate `DatabaseConnectionStatus{SecretRef corev1.LocalObjectReference, Host string, Port
   int32, Database string}` (identical to `SchemaConnectionStatus` minus the `Schema` field)
   instead of adding `omitempty` to a shared type — two small, honest types beat one type with a
   field that's meaningless for half its uses.
3. `DatabaseProvider`: `Spec{Type ProviderType, External *ExternalProviderSpec, Embedded
   *EmbeddedProviderSpec}` with a `+kubebuilder:validation:XValidation` CEL rule enforcing exactly
   one of `External`/`Embedded` is set matching `spec.type`. `ExternalProviderSpec{
   ConnectionSecretRef corev1.SecretReference}`. `EmbeddedProviderSpec{DeletionPolicy
   DeletionPolicy, Storage StorageSpec, Resources corev1.ResourceRequirements, Extensions
   []string}` — no image field, ever (`docs/plan.md` §7.1). `Status{Conditions
   []metav1.Condition}`.
4. Condition/status helpers: reuse this repo's existing `common.Status`
   `GetConditions`/`SetConditions` embedding pattern (same as every other module's CRD types) —
   do not hand-roll a new condition type.
5. `DatabaseService` (module-enablement CR, `api/components/v1alpha1/`): standard `PlatformObject`
   shape (`common.Status` + `common.ComponentReleaseStatus`), no custom spec fields needed beyond
   what the framework requires for `reconciler.ReconcilerFor`.
6. Kubebuilder markers: `+kubebuilder:resource:scope=Namespaced` for the two claims,
   `+kubebuilder:resource:scope=Cluster` for `DatabaseProvider`, `+kubebuilder:printcolumn` for
   `phase`/`Reachable` where useful.
7. **Push every shape-level constraint into schema/CEL markers, per `docs/plan.md` §5 — do not
   leave these to be checked imperatively in a reconciler:**
   - `ProviderRef`: `+kubebuilder:validation:XValidation` on the parent spec enforcing exactly one
     of `provider.name` / `provider.selector` is set (mirrors the `DatabaseProvider`
     `external`/`embedded` rule in step 3 — same pattern, applied on both claim kinds).
   - `AccessMode`: `+kubebuilder:validation:Enum=ReadWrite;ReadOnly`,
     `+kubebuilder:default=ReadWrite`.
   - `DeletionPolicy` (on `SchemaClaim.spec` and `EmbeddedProviderSpec`):
     `+kubebuilder:validation:Enum=Retain;Delete`, `+kubebuilder:default=Retain`.
   - `SchemaClaim.spec.schema`: `+kubebuilder:validation:MaxLength=63` (PostgreSQL identifier
     limit) and a `+kubebuilder:validation:Pattern` restricting it to a safe identifier shape
     (lowercase alphanumeric + underscore, must start with a letter) *when set* — this doesn't
     replace the runtime default-and-sanitize behavior for the unset case (task-06), it just
     rejects an obviously-invalid explicit value at admission time instead of at reconcile time.
   - `DatabaseClaim.spec.database`: `+kubebuilder:validation:Required`,
     `+kubebuilder:validation:MinLength=1` (already implied by "required, always" in
     `docs/plan.md` §5, but must be an actual schema marker, not just a Go comment).
   - `DatabaseProvider.spec.type`: `+kubebuilder:validation:Enum=External;Embedded`,
     `+kubebuilder:validation:Required`.
   - `EmbeddedProviderSpec.extensions`: `+kubebuilder:validation:Pattern` per item restricting to
     a safe extension-name shape (defense in depth — the image-mapping table in task-08 still
     does its own exact-match validation, but garbage input shouldn't reach that logic at all).
   - Consider `+kubebuilder:validation:XValidation` immutability rules (`self == oldSelf`) for
     fields that don't make sense to change post-creation — `DatabaseClaim.spec.database` and
     `SchemaClaim.spec.schema` are the strongest candidates (changing either mid-life implies a
     different claim, not an update to this one); decide during implementation and document the
     reasoning either way, don't skip the question silently.
8. Run `make manifests generate` — verify `config/crd/bases/*.yaml` and
   `zz_generated.deepcopy.go` are produced without manual edits.
9. Add an integration test, run against the connected cluster: install the generated CRDs
   (`kubectl apply -f config/crd/bases/`, or the equivalent envtest bootstrap against the real
   cluster) and attempt to create objects violating **every** schema/CEL rule from step 7 above —
   `DatabaseProvider` with both/neither `external`/`embedded` set or a `spec.type` mismatch;
   claims with both/neither `provider.name`/`provider.selector`; an invalid `access` or
   `deletionPolicy` enum value; a `DatabaseClaim` missing `spec.database`; a `SchemaClaim.spec.
   schema` violating the identifier pattern or length limit; an `EmbeddedProviderSpec.extensions`
   entry violating the name-shape pattern; and, if immutability rules were added, an update
   attempting to change an immutable field. Every one of these must be rejected by
   `kube-apiserver` itself — CEL/schema validation is enforced server-side, so none of this can
   be verified with a fake client; it requires the CRDs actually installed on a real cluster.
   Also assert the positive case: a fully valid object of each kind is accepted.

## Acceptance criteria

- `make manifests generate` succeeds and produces valid CRD YAML for all four kinds
  (`SchemaClaim`, `DatabaseClaim`, `DatabaseProvider`, `DatabaseService`).
- **Integration test** (step 9) exists and passes against the connected cluster, covering every
  schema/CEL rule added in step 7 (both the rejection and the valid-object acceptance cases) —
  this task is not complete without it.
- `go vet ./...` and `go build ./...` succeed.
- Every status type embeds the shared `common.Status` conditions helper — no new condition
  plumbing invented.
- No shape-level constraint enumerated in step 7 is left to be checked only in Go reconciler code
  — if a later task (06/07/08) still contains an imperative check for something step 7 could have
  expressed as a schema/CEL rule, that's a regression to fix, not an acceptable alternative.
