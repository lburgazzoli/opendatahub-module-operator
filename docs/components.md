# Component Catalog

Each row describes one component from the monolithic operator that becomes an
independent module operator.

Excluded from migration scope: `modelsasservice` and `kueue`.

## Simple Components (do first)

| Component | Migrated | CRD Kind | Instance Name | Owns | Watches | Cross-Deps | Manifest Source |
|-----------|----------|----------|---------------|------|---------|------------|-----------------|
| trainingoperator | No | TrainingOperator | default-trainingoperator | CM, SA, Svc, ClusterRole/Binding, Deployment, PodMonitor | CRD (by label) | None | opendatahub-io/training-operator |
| ray | Yes | Ray | default-ray | CM, Secret, SA, Svc, ClusterRole/Binding, Deployment, SCC | CRD (by label), CodeFlare GVK (sanity) | CodeFlare conflict check | opendatahub-io/kuberay |
| sparkoperator | Yes | SparkOperator | default-sparkoperator | CM, SA, Svc, ClusterRole/Binding, Deployment, PodMonitor, MWH, VWH | CRD (by label) | None | opendatahub-io/spark-operator |
| feastoperator | Yes | FeastOperator | default-feastoperator | CM, SA, Svc, Role/Binding, ClusterRole/Binding, Deployment | CRD (by label) | Gateway OIDC (read config) | opendatahub-io/feast |
| ogx | Yes | OGX | default-ogx | CM, SA, Svc, Role/Binding, ClusterRole/Binding, Deployment, PDB | CRD (by label) | DSC (deprecation check for LlamaStack) | opendatahub-io/ogx-k8s-operator |
| mlflowoperator | Yes | MLflowOperator | default-mlflowoperator | CM, SA, Svc, ClusterRole/Binding, ConsoleLink, ServiceMonitor, Deployment, MLflow CRD | CRD (by label), HTTPRoute | Gateway domain (read config) | opendatahub-io/mlflow-operator |
| trustyai | Yes | TrustyAI | default-trustyai | CM, SA, Svc, ClusterRole/Binding, Role/Binding, Deployment | CRD (by label + InferenceServices CRD) | KServe CRD must exist | opendatahub-io/trustyai-service-operator |
| trainer | No | Trainer | default-trainer | CM, SA, Svc, ClusterRole/Binding, Deployment, PodMonitor, ClusterTrainingRuntime | CRD (by label), JobSetOperator | JobSet operator status | opendatahub-io/trainer |

## Medium Components

| Component | Migrated | CRD Kind | Instance Name | Notes |
|-----------|----------|----------|---------------|-------|
| datasciencepipelines | No | DataSciencePipelines | default-datasciencepipelines | Argo Workflows CRD precondition, SCC ownership |
| modelregistry | No | ModelRegistry | default-modelregistry | Gateway domain dependency, template rendering, extra manifests |
| modelcontroller | No | ModelController | default-modelcontroller | Depends on KServe being Managed, KEDA subscription watch, WVA manifests |

## Complex Components

| Component | Migrated | CRD Kind | Instance Name | Notes |
|-----------|----------|----------|---------------|-------|
| kserve | No | Kserve | default-kserve | Many dynamic GVKs, Istio/cert-manager CRD watches, model cache, LLM configs, xKS support |
| dashboard | No | Dashboard | default-dashboard | Dynamic GVKs, OdhDashboardConfig, observability, hardware profiles, gateway dep |
| workbenches | No | Workbenches | default-workbenches | 3 manifest sets, MLflowOperator watch, ImageStream tracking, notebook namespace |

## Cross-Component Dependencies

When a module needs awareness of another module's CR, it should **discover via
CRD watch** (watch the CRD resource, check if it exists) rather than importing
the other module's types. Pattern: `Watches(&apiextensionsv1.CustomResourceDefinition{})`.

| Module | Needs to know about | Discovery method |
|--------|--------------------|--------------------|
| trustyai | KServe (InferenceServices CRD) | CRD existence check |
| modelcontroller | KServe (management state) | Discover KServe CR |
| ray | CodeFlare (must NOT exist) | CRD existence check |
| ogx | LlamaStackOperator (deprecated) | DSC check → replace with CRD discovery |
| workbenches | MLflowOperator | Watch MLflowOperator CR |
| kueue | Auth service | Watch Auth CR |
| kserve | LeaderWorkerSet operator | Subscription/CRD watch |
