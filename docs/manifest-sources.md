# Manifest Sources per Component

Extracted from `get_all_manifests.sh`. Each module needs its own
`get-manifests.sh` that fetches only its component's manifests.

Format: `repo-org:repo-name:ref@commit:source-folder`

## ODH Manifests

| Component | Source |
|-----------|--------|
| dashboard | opendatahub-io:odh-dashboard:main@48eb77f:manifests |
| datasciencepipelines | opendatahub-io:data-science-pipelines-operator:main@582a406:config |
| kserve | opendatahub-io:kserve:release-v0.17@5807ece:config |
| ray | opendatahub-io:kuberay:dev@b30c9c1:ray-operator/config |
| trustyai | opendatahub-io:trustyai-service-operator:incubation@de96668:config |
| modelregistry | opendatahub-io:model-registry-operator:main@a982cf8:config |
| trainingoperator | opendatahub-io:training-operator:stable@28a60bd:manifests |
| modelcontroller | opendatahub-io:odh-model-controller:incubating@3632f68:config |
| feastoperator | opendatahub-io:feast:stable@fc14c10:infra/feast-operator/config |
| ogx | opendatahub-io:ogx-k8s-operator:odh@54ce7ea:config |
| trainer | opendatahub-io:trainer:stable@51baadf:manifests |
| maas | opendatahub-io:models-as-a-service:stable@322a275:deployment |
| mlflowoperator | opendatahub-io:mlflow-operator:main@4cccfcc:config |
| sparkoperator | opendatahub-io:spark-operator:main@f91ff22:config |
| workbenches/kf-notebook-controller | opendatahub-io:kubeflow:main@f09b56e:components/notebook-controller/config |
| workbenches/odh-notebook-controller | opendatahub-io:kubeflow:main@f09b56e:components/odh-notebook-controller/config |
| workbenches/notebooks | opendatahub-io:notebooks:main@139807f:manifests |
| wva | opendatahub-io:workload-variant-autoscaler:main@0641ee0:config |
| kueue | (not in get_all_manifests — uses operator-managed manifests) |

## RHOAI Manifests

Same components with `red-hat-data-services` org and `rhoai-3.5-ea.1` refs.
See `get_all_manifests.sh` for exact refs.

## Per-Module Script Pattern

Each module's `get-manifests.sh` should:
1. Define only its own component entry (ODH + RHOAI)
2. Resolve `PROJECT_ROOT` from the script location
3. Select one pinned source based on `ODH_PLATFORM_TYPE`
4. Optionally copy from an adjacent checkout when `USE_LOCAL=true`
5. **`rm -rf config/manifests/$component/`** then copy fetched content (avoids stale YAML)

Because each module downloads only one component, the script can stay linear.
It does not need the monolith's associative arrays, shared download helpers,
or `--component=org:repo:ref:path` override handling unless a module has a
real need for them.

After fetch, run the manifest RBAC audit (skill step 5b /
`.agents/skills/odh-component-to-module/references/manifest-rbac-audit.md`):
`kustomize build` on `config/manifests/${ContextDir}/${SourcePath}`, then
align controller `Owns` and operator RBAC with the build output.
