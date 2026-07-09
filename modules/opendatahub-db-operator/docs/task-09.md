# Task 09 — RBAC, Packaging, Helm Chart

See `docs/plan.md` §10.

## Goal

Finalize kubebuilder RBAC markers against the actual `Owns`/DDL surface, generate manifests, and
produce a working Helm chart via the reused `chartgen` command.

## Depends on

Tasks 02–08 (RBAC markers must reflect the real, final `Owns` list and DDL connection targets).

## Key files/packages

- RBAC markers across `internal/controller/**/*.go` (co-located with the reconciler that needs
  them, per this repo's convention).
- `config/chart/` (generated, not hand-edited).
- `config/rbac/schemaclaim-creator-role.yaml`, `config/rbac/db-consumer-role.yaml` — illustrative,
  non-enforced consumer-facing `ClusterRole`s (see step 7).

## Steps

1. Add `+kubebuilder:rbac` markers for: the four CRDs (`schemaclaims`, `databaseclaims`,
   `databaseproviders`, `databaseservices`) with full verbs; `secrets` (`get;list;watch;create;
   update;patch;delete` — claim Secrets, admin Secret, and reading `External`
   `connectionSecretRef` targets which may live in other namespaces — confirm whether a
   cluster-scoped Secret read permission is required for cross-namespace `External` secrets, or
   whether a narrower per-namespace RoleBinding pattern is more appropriate; document the
   decision); `statefulsets`, `persistentvolumeclaims`, `services`, `configmaps`,
   `networkpolicies` (`get;list;watch;create;update;patch;delete` — the `Embedded` provider's
   `NetworkPolicy`, `docs/plan.md` §6/task-08); baseline CRD marker
   (`apiextensions.k8s.io/customresourcedefinitions`); protected-metrics markers
   (`tokenreviews`, `subjectaccessreviews`, `urls=/metrics`) per every module operator's
   baseline.
2. Cross-check every `Owns()` call against the RBAC markers — every Kind this controller creates
   or watches must have a matching RBAC rule (per `.agents/skills/odh-manifest-audit`
   conventions, adapted here since there's no fetched upstream manifest to audit against — audit
   against this module's own `Owns()` calls instead).
3. `make manifests generate helm` — verify `config/crd/bases/*.yaml`, `config/rbac/role.yaml`,
   and `config/chart/` all generate cleanly.
4. `kustomize build config/default` must succeed with no errors.
5. Ship the consumer-facing RBAC split spec.md's design rationale calls for (§"RBAC Design
   Rationale") — not just the controller's own `ClusterRole`, which is a different thing (that's
   the permissions *this operator* needs; these are the permissions *tenants/module operators*
   need to *use* it): `config/rbac/schemaclaim-creator-role.yaml` (a `ClusterRole` granting
   `create;get;list;watch` on `schemaclaims` only — for team/tenant namespace service accounts,
   per spec.md) and `config/rbac/db-consumer-role.yaml` (the same plus `databaseclaims` — for
   platform module operators). These are illustrative reference manifests an admin binds via a
   `RoleBinding` in a consuming namespace; they are not applied automatically by this module and
   carry no owner reference to anything — document this explicitly in a comment in each file so
   they aren't mistaken for something this operator manages the lifecycle of.

For this module, `module.yaml` and `component_metadata.yaml` were consciously
left out of scope by product decision during implementation, so task closeout
focuses on RBAC correctness, manifest/chart generation, and installability.

## Acceptance criteria

- `make manifests generate helm` succeeds with no manual post-edits to generated files.
- `kustomize build config/default | kubectl apply --dry-run=server -f -` succeeds against a test
  cluster (validates RBAC/CRD schema correctness without actually installing).
- Every Kind referenced in `Owns()` across all reconcilers has a corresponding RBAC marker;
  every RBAC marker corresponds to something the controller actually touches (no unused,
  overly-broad grants).
- Helm chart installs cleanly on a fresh `kind` cluster (`make deploy-helm` succeeds, module pod
  reaches `Running`).
- `schemaclaim-creator-role.yaml` and `db-consumer-role.yaml` (step 7) exist, are valid
  `ClusterRole` manifests (`kubectl apply --dry-run=server -f` succeeds against the connected
  cluster), and `schemaclaim-creator-role.yaml` grants no verb on `databaseclaims` — this is the
  one place a validation test should actively check that a permission is *absent*, since the
  entire point of the split (spec.md's rationale) is that tenant namespaces cannot touch
  `DatabaseClaim`.
