# Component Catalog

Each row describes one component from the monolithic operator that becomes an
independent module operator.

## Simple Components (do first)

| Component | CRD Kind | Instance Name | Owns | Watches | Cross-Deps | Manifest Source |
|-----------|----------|---------------|------|---------|------------|-----------------|
| trainingoperator | TrainingOperator | default-trainingoperator | CM, SA, Svc, ClusterRole/Binding, Deployment, PodMonitor | CRD (by label) | None | opendatahub-io/training-operator |
| ray | Ray | default-ray | CM, Secret, SA, Svc, ClusterRole/Binding, Deployment, SCC | CRD (by label), CodeFlare GVK (sanity) | CodeFlare conflict check | opendatahub-io/kuberay |
| sparkoperator | SparkOperator | default-sparkoperator | CM, SA, Svc, ClusterRole/Binding, Deployment, PodMonitor, MWH, VWH | CRD (by label) | None | opendatahub-io/spark-operator |
| feastoperator | FeastOperator | default-feastoperator | CM, SA, Svc, Role/Binding, ClusterRole/Binding, Deployment | CRD (by label) | Gateway OIDC (read config) | opendatahub-io/feast |
| ogx | OGX | default-ogx | CM, SA, Svc, Role/Binding, ClusterRole/Binding, Deployment, PDB | CRD (by label) | DSC (deprecation check for LlamaStack) | opendatahub-io/ogx-k8s-operator |
| mlflowoperator | MLflowOperator | default-mlflowoperator | CM, SA, Svc, ClusterRole/Binding, ConsoleLink, ServiceMonitor, Deployment, MLflow CRD | CRD (by label), HTTPRoute | Gateway domain (read config) | opendatahub-io/mlflow-operator |
| trustyai | TrustyAI | default-trustyai | CM, SA, Svc, ClusterRole/Binding, Role/Binding, Deployment | CRD (by label + InferenceServices CRD) | KServe CRD must exist | opendatahub-io/trustyai-service-operator |
| trainer | Trainer | default-trainer | CM, SA, Svc, ClusterRole/Binding, Deployment, PodMonitor, ClusterTrainingRuntime | CRD (by label), JobSetOperator | JobSet operator status | opendatahub-io/trainer |

## Medium Components

| Component | CRD Kind | Instance Name | Notes |
|-----------|----------|---------------|-------|
| datasciencepipelines | DataSciencePipelines | default-datasciencepipelines | Argo Workflows CRD precondition, SCC ownership |
| modelregistry | ModelRegistry | default-modelregistry | Gateway domain dependency, template rendering, extra manifests |
| modelcontroller | ModelController | default-modelcontroller | Depends on KServe being Managed, KEDA subscription watch, WVA manifests |

## Complex Components

| Component | CRD Kind | Instance Name | Notes |
|-----------|----------|---------------|-------|
| kserve | Kserve | default-kserve | Many dynamic GVKs, Istio/cert-manager CRD watches, model cache, LLM configs, xKS support |
| kueue | Kueue | default-kueue | Auth service cross-dep, operator monitoring, default queue management, Managed/Unmanaged modes |
| dashboard | Dashboard | default-dashboard | Dynamic GVKs, OdhDashboardConfig, observability, hardware profiles, gateway dep |
| workbenches | Workbenches | default-workbenches | 3 manifest sets, MLflowOperator watch, ImageStream tracking, notebook namespace |
| modelsasservice | ModelsAsService | default-modelsasservice | Dynamic kustomize bundle, maas-controller CRD, Config CR ownership |

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
