# Manifest Script

## Location

`hack/scripts/get_manifests.sh` — downloads component manifests from GitHub.

## Source

Extract the component's entry from the monolith's `get_all_manifests.sh`:
```
/home/luca/work/dev/openshift-ai/opendatahub-operator/get_all_manifests.sh
```

Each entry has format: `repo-org:repo-name:ref@commit:source-folder`

You need both the ODH and RHOAI entries for the component.

## Template

Copy from the ray module and change the component entries:
```
modules/opendatahub-ray-operator/hack/scripts/get_manifests.sh
```

Key things to change:
- `ODH_COMPONENT_MANIFESTS` — the component's ODH entry
- `RHOAI_COMPONENT_MANIFESTS` — the component's RHOAI entry
- Temp dir name in `mktemp` (cosmetic)

## Required patterns

### Resolve project root from script location

```bash
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
DST_MANIFESTS_DIR="${PROJECT_ROOT}/config/manifests"
```

This ensures the script works regardless of the caller's working directory.

### Download destination

Always `config/manifests/$COMPONENT/`. The Makefile target is:
```makefile
.PHONY: get-manifests
get-manifests: ## Download component manifests.
	./hack/scripts/get_manifests.sh
```

## .gitignore

Add to `.gitignore`:
```
config/manifests/$COMPONENT/
```

Keep a `.gitkeep` in `config/manifests/` (not in the component subdir).

## Containerfile

The Containerfile copies manifests into the image:
```dockerfile
COPY config/manifests/ /manifests/
```

This requires running `make get-manifests` before `make container-build`.
