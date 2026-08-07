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
- `api/services/v1alpha1/databaseservice_types.go` — the module-enablement CR (`docs/plan.md` §4)

## Steps

1. `ConnectionStatus{SecretRef corev1.LocalObjectReference, Host string, Port int32}` in
   `common_types.go` — the fields shared by both claim kinds' connection status, all three
   `+kubebuilder:validation:Required` (once a claim's connection is populated at all, none of
   these can legitimately be empty — they're written atomically, never partially).
2. `SchemaClaim`: `Spec{Provider ProviderRef, Schema string, Database string, Access AccessMode,
   DeletionPolicy DeletionPolicy}`, `Status{common.Status, common.ComponentReleaseStatus, Schema string,
   Connection SchemaConnectionStatus, Provider string}`. `ProviderRef{Name string, Selector
   *metav1.LabelSelector}`. `SchemaConnectionStatus{ConnectionStatus, Database string
   (required), Schema string (required)}` — embeds `ConnectionStatus` from step 1. **No custom
   `Phase` field** — `common.Status` already has one (used generically by the orchestrator across
   every `PlatformObject`); a second, differently-typed `Phase` field on this struct would shadow
   it for JSON purposes without either author realizing it. **`Provider` is a single string, not
   spec.md's literal `matchedProviders: [...]` list** — a claim binds to exactly one provider, so
   surfacing the also-ran candidates that lost has no consumer (`docs/plan.md` §6 documents this
   as a deliberate, disclosed divergence from spec.md's example).
3. `DatabaseClaim`: same shape minus `DeletionPolicy` and `Schema`, with optional
   `Spec.Database string`; `Status.Database` stores the resolved effective database.
   **Do not reuse `SchemaConnectionStatus`
   for this CRD's `Status.Connection`** — spec.md's `DatabaseClaim` status example has no
   `schema` key under `connection` at all, and reusing the schema-claim struct verbatim would
   silently serialize an always-empty `schema: ""` field, contradicting the spec's shape. Define
   a separate `DatabaseConnectionStatus{ConnectionStatus, Database string (required)}` —
   embedding the same shared `ConnectionStatus` from step 1, just without the `Schema` field —
   instead of adding `omitempty` to a shared type: two small, honest types beat one type with a
   field that's meaningless for half its uses.
4. `DatabaseProvider`: `Spec{Type ProviderType, DefaultDatabase string, External *ExternalProviderSpec, Embedded
   *EmbeddedProviderSpec}` with a `+kubebuilder:validation:XValidation` CEL rule enforcing exactly
   one of `External`/`Embedded` is set matching `spec.type`. `ExternalProviderSpec{
   ConnectionSecretRef corev1.SecretReference, Capabilities []ExternalCapability}`.
   `EmbeddedProviderSpec{DeletionPolicy
   DeletionPolicy, Storage StorageSpec, Resources corev1.ResourceRequirements, Extensions
   []string}` — no image field, ever (`docs/plan.md` §7.1). `Status{common.Status,
   common.ComponentReleaseStatus}` — no custom fields; spec.md's own status example for this CRD
   has no phase, so the embedded generic fields are sufficient.
5. Condition/status helpers: reuse this repo's existing `common.Status`
   `GetConditions`/`SetConditions` embedding pattern (same as every other module's CRD types) —
   do not hand-roll a new condition type. **Every one of the four CRD status types must also
   embed `common.ComponentReleaseStatus` and implement `GetReleaseStatus`/`SetReleaseStatus`**,
   not just `DatabaseService` — `reconciler.ReconcilerFor[T]`'s generic constraint is the full
   `common.PlatformObject` interface (`client.Object` + status + conditions + releases), so
   `SchemaClaim`/`DatabaseClaim`/`DatabaseProvider` need it structurally to typecheck against
   that builder in task-03, even though "releases" has no obvious semantic meaning for a claim.
6. `DatabaseService` (module-enablement CR, `api/services/v1alpha1/`, group
   `services.platform.opendatahub.io` — **a distinct group from
   `components.platform.opendatahub.io`**, since this CR represents a platform infrastructure
   service, not a user-facing ML/serving component the way Ray/ModelRegistry's Module CRs do):
   standard `PlatformObject` shape (`common.Status` + `common.ComponentReleaseStatus`), no custom
   spec fields needed beyond what the framework requires for `reconciler.ReconcilerFor`. Singleton
   CEL rule on `metadata.name` (`self.metadata.name == 'default-db-operator'`), same pattern as
   `Ray`'s `default-ray` singleton enforcement.
7. `groupversion_info.go` for both new packages: build `SchemeBuilder` directly on apimachinery's
   `runtime.SchemeBuilder` (`SchemeBuilder.Register(func(s *runtime.Scheme) error { ... })` per
   type file, plus one `init()` in `groupversion_info.go` calling `metav1.AddToGroupVersion`) —
   **not** controller-runtime's `scheme.Builder` helper, which is deprecated for api packages
   specifically because it drags in controller-runtime as a dependency of a package that should
   depend only on the standard library, apimachinery, and other api packages. Also add a `Version
   = "v1alpha1"` constant alongside `GroupName` rather than a literal string inline in
   `SchemeGroupVersion`.
8. Kubebuilder markers: `+kubebuilder:resource:scope=Namespaced` for the two claims,
   `+kubebuilder:resource:scope=Cluster` for `DatabaseProvider`/`DatabaseService`,
   `+kubebuilder:printcolumn` referencing real condition types (`Provisioned` for claims,
   `Reachable` for `DatabaseProvider`, `Ready` for `DatabaseService`) — not a `Phase`
   printcolumn, since there is no claim-specific phase enum distinct from `common.Status.Phase`
   (step 2).
9. **Push every shape-level constraint into schema/CEL markers, per `docs/plan.md` §5 — do not
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
  - `DatabaseClaim.spec.database`: optional, but when set should carry
    `+kubebuilder:validation:MinLength=1`.
   - `DatabaseProvider.spec.type`: `+kubebuilder:validation:Enum=External;Embedded`,
     `+kubebuilder:validation:Required`.
  - `DatabaseProvider.spec.defaultDatabase`: optional with
    `+kubebuilder:validation:MinLength=1`.
  - `ExternalProviderSpec.capabilities`: per-item enum validation for
    `CreateDatabase` and `CreateSchema`.
   - `EmbeddedProviderSpec.extensions`: `+kubebuilder:validation:Pattern` per item restricting to
     a safe extension-name shape (defense in depth — the image-mapping table in task-08 still
     does its own exact-match validation, but garbage input shouldn't reach that logic at all).
   - Consider `+kubebuilder:validation:XValidation` immutability rules (`self == oldSelf`) for
     fields that don't make sense to change post-creation — `DatabaseClaim.spec.database` and
     `SchemaClaim.spec.schema` are the strongest candidates (changing either mid-life implies a
     different claim, not an update to this one); decide during implementation and document the
     reasoning either way, don't skip the question silently.
10. Run `make manifests generate` — verify `config/crd/bases/*.yaml` and
    `zz_generated.deepcopy.go` are produced without manual edits.
11. Add an integration test, run against the connected cluster: install the generated CRDs
    (`kubectl apply -f config/crd/bases/`, or the equivalent envtest bootstrap against the real
    cluster) and attempt to create objects violating **every** schema/CEL rule from step 9 above —
    `DatabaseProvider` with both/neither `external`/`embedded` set or a `spec.type` mismatch;
    claims with both/neither `provider.name`/`provider.selector`; an invalid `access` or
    `deletionPolicy` enum value; a `SchemaClaim.spec.schema` violating the identifier pattern or
    length limit; an `EmbeddedProviderSpec.extensions`
    entry violating the name-shape pattern; and, if immutability rules were added, an update
    attempting to change an immutable field. Every one of these must be rejected by
    `kube-apiserver` itself — CEL/schema validation is enforced server-side, so none of this can
    be verified with a fake client; it requires the CRDs actually installed on a real cluster.
    Also assert the positive case: a fully valid object of each kind is accepted.

## Acceptance criteria

- `make manifests generate` succeeds and produces valid CRD YAML for all four kinds
  (`SchemaClaim`, `DatabaseClaim`, `DatabaseProvider`, `DatabaseService`).
- **Integration test** (step 11) exists and passes against the connected cluster, covering every
  schema/CEL rule added in step 9 (both the rejection and the valid-object acceptance cases) —
  this task is not complete without it.
- `go vet ./...` and `go build ./...` succeed.
- Every status type embeds the shared `common.Status` conditions helper (and
  `common.ComponentReleaseStatus`, step 5) — no new condition plumbing invented.
- No shape-level constraint enumerated in step 9 is left to be checked only in Go reconciler code
  — if a later task (06/07/08) still contains an imperative check for something step 9 could have
  expressed as a schema/CEL rule, that's a regression to fix, not an acceptable alternative.

## Status: Done

Verified via `make manifests generate` (all four CRD YAMLs produced, deepcopy generated cleanly)
and `make test-integration-run` against the connected `kind` cluster: `TestCRDValidation`
exercises every CEL/schema rule (`DatabaseProvider` external/embedded exclusivity and type
match, both claims' `ProviderRef` exclusivity, `AccessMode`/`DeletionPolicy` enums, `schema`
pattern/length, database/schema immutability, provider default database and capabilities schema
validation, `extensions` item pattern)
plus the positive accept case for each kind, all rejected/accepted by `kube-apiserver` itself.
