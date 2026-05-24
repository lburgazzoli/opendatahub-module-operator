# Manifest Sources per Component

Extracted from `get_all_manifests.sh`. Each module needs its own
`get_manifests.sh` that fetches only its component's manifests.

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

Each module's `get_manifests.sh` should:
1. Define only its own component entry (ODH + RHOAI)
2. Reuse the same `git_fetch_ref` / `download_repo_content` functions
3. Download to `opt/manifests/$component/`
4. Support `--component=org:repo:ref:path` override
