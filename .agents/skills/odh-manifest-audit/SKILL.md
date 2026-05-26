---
name: odh-manifest-audit
description: >
  Audit controller Owns/RBAC markers against kustomize build output. Run after
  make get-manifests, upstream manifest updates, or adding new workload
  resource types. Ensures the controller owns every deployed resource and
  holds all required RBAC permissions.
user-invocable: true
allowed-tools:
  - Read
  - Glob
  - Grep
  - Bash
---

# Manifest RBAC Audit

## When to use

- After `make get-manifests` (upstream manifest refresh)
- After upstream manifest updates or version bumps
- After adding new resources to `config/manifests/`
- When debugging "is not cached" errors (`ReaderFailOnMissingInformer`)
- When debugging "RBAC escalation" errors during deploy

## Core rules

1. **Own everything kustomize deploys (except CRDs/Namespaces).** Every Kind
   in `kustomize build` output needs a matching `Owns()` or `OwnsGVK()` on the
   controller. Missing Owns causes runtime "is not cached" failures.

2. **RBAC for deployed operand ClusterRoles/Roles.** Every permission granted
   by ClusterRoles or Roles in the build output must appear as
   `+kubebuilder:rbac` markers on the module operator. Kubernetes RBAC
   escalation prevention blocks creating roles the operator SA does not hold.

3. **Watches stay from the monolith.** Cross-component watches, CRD watches,
   and dynamic GVK watches are not inferable from kustomize. Port them from
   the monolith controller verbatim.

4. **Baseline module-operator RBAC.** Every module keeps the CRD marker
   (`customresourcedefinitions get;list;watch;create;update;patch;delete`).
   When `/metrics` is exposed, also keep `tokenreviews create`,
   `subjectaccessreviews create`, and `urls=/metrics get`.

5. **Monolith Owns not in build.** Drop only if confirmed removed upstream;
   otherwise flag for review.

## Procedure

1. Resolve the kustomize path from the module's `ContextDir` + `SourcePath`.
2. Run `kustomize build` on every overlay (ODH and RHOAI if both exist).
3. List deployed Kinds; cross-check against controller `Owns`/`OwnsGVK`.
4. List operand ClusterRole/Role rules; cross-check against `+kubebuilder:rbac`.
5. Verify baseline RBAC markers (CRDs, protected-metrics) are present.
6. Check monolith Watches are ported.
7. Update markers and Owns, then run `make manifests generate`.
8. Verify `config/rbac/role.yaml` includes all operand permissions.

## Quick reference

Full commands, Kind-to-Owns mapping table, multi-overlay audit commands, and
the complete checklist are in
[references/manifest-rbac-audit.md](references/manifest-rbac-audit.md).

## After updating markers

```bash
make manifests generate
```

Verify generated `config/rbac/role.yaml` includes operand permissions.
When multiple overlays exist, size Owns and RBAC markers for the **union**
of all overlays, not just the default.
