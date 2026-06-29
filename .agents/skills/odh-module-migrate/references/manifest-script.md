# Manifest Script

## Location

`hack/scripts/get-manifests.sh` — downloads component manifests from GitHub.

Module hack scripts use **hyphens**, not underscores (e.g. `get-manifests.sh`,
`cleanup-integration.sh`, `cleanup-e2e.sh`).

## Source

Extract the component's entry from the monolith's `get_all_manifests.sh`:
```
/home/luca/work/dev/openshift-ai/opendatahub-operator/get_all_manifests.sh
```

Each entry has format: `repo-org:repo-name:ref@commit:source-folder`

You need both the ODH and RHOAI entries for the component.

## Template

Copy from the ray module and change the component-specific constants:
```
modules/opendatahub-ray-operator/hack/scripts/get-manifests.sh
```

Key things to change:
- `COMPONENT_NAME`
- `REPO_NAME`
- `SOURCE_PATH`
- ODH and RHOAI `REPO_URL` / `COMMIT_SHA` values

## Required patterns

### Resolve project root from script location

```bash
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
DST_MANIFESTS_DIR="${PROJECT_ROOT}/config/manifests/${COMPONENT_NAME}"
```

This ensures the script works regardless of the caller's working directory.
**Always** set `DST_MANIFESTS_DIR` to the component subdir
(`config/manifests/$COMPONENT/`), not the parent `config/manifests/`.

### Wipe destination before copy (required)

**Always** remove the component directory before writing fetched content:

```bash
rm -rf "${DST_MANIFESTS_DIR}"
mkdir -p "${DST_MANIFESTS_DIR}"
cp -a "${source}/." "${DST_MANIFESTS_DIR}/"
```

Apply in **both** paths: git shallow-fetch and `USE_LOCAL=true` adjacent checkout.

Rationale: upstream renames or deletes files between commits. Copying on top
of an existing tree leaves orphaned YAML, which breaks kustomize builds and
produces wrong Owns/RBAC audits.

Do **not** delete `config/manifests/` itself or `.gitkeep` — only the
component subdir.

No separate Makefile `clean-manifests` target is needed if the script always
wipes before copy.

### Fetch-time vs runtime platform

Two different platform concepts — do not conflate them:

| When | Variable | Purpose |
|------|----------|---------|
| **`make get-manifests`** | `ODH_PLATFORM_TYPE` (bash env) | Selects ODH vs RHOAI **git repo/commit** to fetch |
| **Operator runtime** | `platformType` in ConfigMap / `cfg.PlatformType` | Selects **kustomize overlay** at reconcile (platform-map components) |

- **Fixed overlay** (ray): `SourcePath` is always `openshift`; ODH vs RHOAI
  differs only at fetch time — fetch twice or document which fetch you used.
- **Platform map** (spark, ogx, dsp): fetched tree contains `overlays/odh` and
  `overlays/rhoai`; runtime picks overlay from platform type. Kustomize audit
  uses configmap default — see [manifest-rbac-audit.md](manifest-rbac-audit.md).

`ODH_PLATFORM_TYPE` is **not** `ODH_MODULE_OPERATOR_PLATFORM_TYPE`.

### Keep the flow linear

Module scripts only fetch one component:

1. Select ODH or RHOAI source based on `ODH_PLATFORM_TYPE`
2. Optionally copy from adjacent checkout when `USE_LOCAL=true` (after `rm -rf`)
3. Otherwise shallow-fetch pinned commit into temp dir
4. `rm -rf` + copy into `config/manifests/$COMPONENT/`

Do not carry over the monolith's associative-array or `--component=...`
override machinery.

### Download destination and Makefile

Always `config/manifests/$COMPONENT/`. After script is ready, run:

```bash
make get-manifests
```

The agent should call `make get-manifests` itself as soon as the script is
ready. Do not pause the migration waiting for user confirmation at this step;
the follow-up audit depends on fetched manifests being present.

Then run the kustomize audit in [manifest-rbac-audit.md](manifest-rbac-audit.md).

```makefile
.PHONY: get-manifests
get-manifests: ## Download component manifests.
	./hack/scripts/get-manifests.sh
```

## .gitignore

Add to `.gitignore`:
```
config/manifests/$COMPONENT/
```

Keep a `.gitkeep` in `config/manifests/` (not in the component subdir).

## Containerfile

`make container-build` runs **`container-prep` on the host** first, then
`podman build`. Prep and image tag are separate from the in-container compile.

| Step | Where | Target |
|------|-------|--------|
| Regenerate CRDs/RBAC | Host | `make manifests` |
| Regenerate DeepCopy | Host | `make generate` |
| Fetch workload YAML | Host | `make get-manifests` |
| Compile manager binary | **Inside Containerfile** | `make build-bin` |

```bash
# Optional explicit prep (container-build runs this automatically)
export ODH_PLATFORM_TYPE=OpenDataHub   # for get-manifests
make container-prep

export IMG="ttl.sh/${MODULE_NAME}-$(uuidgen | tr '[:upper:]' '[:lower:]'):1h"
make container-build IMG="${IMG}"   # host prep + podman build; only go build runs in image
```

Keep `IMG` in shell memory and pass it directly to `make`. Do not write it to a
temp file and later read it back with `cat`.

Manifests are copied into the runtime image:

```dockerfile
COPY config/manifests/ /manifests/
```

Run the kustomize audit after `container-prep` and before first deploy.
