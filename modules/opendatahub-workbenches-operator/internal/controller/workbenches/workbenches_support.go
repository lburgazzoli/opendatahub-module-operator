/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package workbenches

import (
	"context"
	"fmt"
	"path"
	"strconv"
	"strings"

	k8serr "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/opendatahub-io/opendatahub-operator/v2/api/common"
	serviceApi "github.com/opendatahub-io/opendatahub-operator/v2/api/services/v1alpha1"
	"github.com/opendatahub-io/opendatahub-operator/v2/pkg/cluster"

	gvk "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-workbenches-operator/pkg/resources/gvk"
	pkgstatus "github.com/opendatahub-io/opendatahub-operator/v2/pkg/controller/status"
	odhtypes "github.com/opendatahub-io/opendatahub-operator/v2/pkg/controller/types"
)

const (
	ComponentName = "workbenches"

	// LegacyComponentName is the label value assigned to deployments via Kustomize.
	// Deployment selectors are immutable so this must stay "workbenches".
	LegacyComponentName = "workbenches"

	ReadyConditionType = "WorkbenchesReady"

	notebooksPath = "notebooks"

	notebookControllerPath               = "odh-notebook-controller"
	notebookControllerManifestSourcePath = "base"

	kfNotebookControllerPath               = "kf-notebook-controller"
	kfNotebookControllerManifestSourcePath = "overlays/openshift"

	// Gateway constants replicated from the monolith's internal gateway package.
	defaultGatewayName      = "data-science-gateway"
	gatewayNamespace        = "openshift-ingress"
	defaultGatewaySubdomain = "rh-ai"

	// mlflowOperatorCRDName is the cluster-scoped CRD name for MLflowOperator.
	// Watching this CRD lets the controller react when mlflow is installed or removed.
	mlflowOperatorCRDName = "mlflowoperators.components.platform.opendatahub.io"
)

var (
	sectionTitle = map[common.Platform]string{
		cluster.SelfManagedRhoai: "OpenShift Self Managed Services",
		cluster.ManagedRhoai:     "OpenShift Managed Services",
		cluster.OpenDataHub:      "OpenShift Open Data Hub",
	}

	notebookControllerContextDir   = path.Join(ComponentName, notebookControllerPath)
	kfNotebookControllerContextDir = path.Join(ComponentName, kfNotebookControllerPath)
	notebookContextDir             = path.Join(ComponentName, notebooksPath)

	notebookImagesManifestSourcePath = map[common.Platform]string{
		cluster.SelfManagedRhoai: "rhoai/overlays/additional",
		cluster.ManagedRhoai:     "rhoai/overlays/additional",
		cluster.OpenDataHub:      "odh/overlays/additional",
	}

	notebookImagesParamsPath = map[common.Platform]string{
		cluster.SelfManagedRhoai: "rhoai/base",
		cluster.ManagedRhoai:     "rhoai/base",
		cluster.OpenDataHub:      "odh/base",
	}

	// conditionTypes contributing to the controller Ready status.
	// ConditionImageStreamsAvailable is intentionally excluded: some upstream images
	// (CUDA/ROCm) may not be published yet so including it would prevent Workbenches
	// from reaching Ready=True on those clusters.
	conditionTypes = []string{
		pkgstatus.ConditionDeploymentsAvailable,
	}

	// notebookImageParamMap maps params-latest.env keys to RELATED_IMAGE env vars.
	notebookImageParamMap = map[string]string{
		"odh-workbench-codeserver-datascience-cpu-py312-ubi9-n":         "RELATED_IMAGE_ODH_WORKBENCH_CODESERVER_DATASCIENCE_CPU_PY312_IMAGE",
		"odh-workbench-jupyter-datascience-cpu-py312-ubi9-n":            "RELATED_IMAGE_ODH_WORKBENCH_JUPYTER_DATASCIENCE_CPU_PY312_IMAGE",
		"odh-workbench-jupyter-minimal-cpu-py312-ubi9-n":                "RELATED_IMAGE_ODH_WORKBENCH_JUPYTER_MINIMAL_CPU_PY312_IMAGE",
		"odh-workbench-jupyter-minimal-cuda-py312-ubi9-n":               "RELATED_IMAGE_ODH_WORKBENCH_JUPYTER_MINIMAL_CUDA_PY312_IMAGE",
		"odh-workbench-jupyter-minimal-rocm-py312-ubi9-n":               "RELATED_IMAGE_ODH_WORKBENCH_JUPYTER_MINIMAL_ROCM_PY312_IMAGE",
		"odh-workbench-jupyter-pytorch-cuda-py312-ubi9-n":               "RELATED_IMAGE_ODH_WORKBENCH_JUPYTER_PYTORCH_CUDA_PY312_IMAGE",
		"odh-workbench-jupyter-pytorch-rocm-py312-ubi9-n":               "RELATED_IMAGE_ODH_WORKBENCH_JUPYTER_PYTORCH_ROCM_PY312_IMAGE",
		"odh-workbench-jupyter-tensorflow-cuda-py312-ubi9-n":            "RELATED_IMAGE_ODH_WORKBENCH_JUPYTER_TENSORFLOW_CUDA_PY312_IMAGE",
		"odh-workbench-jupyter-tensorflow-rocm-py312-ubi9-n":            "RELATED_IMAGE_ODH_WORKBENCH_JUPYTER_TENSORFLOW_ROCM_PY312_IMAGE",
		"odh-workbench-jupyter-trustyai-cpu-py312-ubi9-n":               "RELATED_IMAGE_ODH_WORKBENCH_JUPYTER_TRUSTYAI_CPU_PY312_IMAGE",
		"odh-workbench-jupyter-pytorch-llmcompressor-cuda-py312-ubi9-n": "RELATED_IMAGE_ODH_WORKBENCH_JUPYTER_PYTORCH_LLMCOMPRESSOR_CUDA_PY312_IMAGE",
		"odh-pipeline-runtime-datascience-cpu-py312-ubi9-n":             "RELATED_IMAGE_ODH_PIPELINE_RUNTIME_DATASCIENCE_CPU_PY312_IMAGE",
		"odh-pipeline-runtime-minimal-cpu-py312-ubi9-n":                 "RELATED_IMAGE_ODH_PIPELINE_RUNTIME_MINIMAL_CPU_PY312_IMAGE",
		"odh-pipeline-runtime-tensorflow-cuda-py312-ubi9-n":             "RELATED_IMAGE_ODH_PIPELINE_RUNTIME_TENSORFLOW_CUDA_PY312_IMAGE",
		"odh-pipeline-runtime-tensorflow-rocm-py312-ubi9-n":             "RELATED_IMAGE_ODH_PIPELINE_RUNTIME_TENSORFLOW_ROCM_PY312_IMAGE",
		"odh-pipeline-runtime-pytorch-cuda-py312-ubi9-n":                "RELATED_IMAGE_ODH_PIPELINE_RUNTIME_PYTORCH_CUDA_PY312_IMAGE",
		"odh-pipeline-runtime-pytorch-rocm-py312-ubi9-n":                "RELATED_IMAGE_ODH_PIPELINE_RUNTIME_PYTORCH_ROCM_PY312_IMAGE",
		"odh-pipeline-runtime-pytorch-llmcompressor-cuda-py312-ubi9-n":  "RELATED_IMAGE_ODH_PIPELINE_RUNTIME_PYTORCH_LLMCOMPRESSOR_CUDA_PY312_IMAGE",
	}
)

func notebookControllerManifestInfo(basePath string, sourcePath string) odhtypes.ManifestInfo {
	return odhtypes.ManifestInfo{
		Path:       basePath,
		ContextDir: notebookControllerContextDir,
		SourcePath: sourcePath,
	}
}

func kfNotebookControllerManifestInfo(basePath string, sourcePath string) odhtypes.ManifestInfo {
	return odhtypes.ManifestInfo{
		Path:       basePath,
		ContextDir: kfNotebookControllerContextDir,
		SourcePath: sourcePath,
	}
}

func notebookImagesManifestInfo(basePath string, sourcePath string) odhtypes.ManifestInfo {
	return odhtypes.ManifestInfo{
		Path:       basePath,
		ContextDir: notebookContextDir,
		SourcePath: sourcePath,
	}
}

// ComputeKustomizeVariable builds the dynamic kustomize parameter map.
func ComputeKustomizeVariable(ctx context.Context, cli client.Client, platform common.Platform) (map[string]string, error) {
	mlflowEnabled, err := isMLflowEnabled(ctx, cli)
	if err != nil {
		return nil, fmt.Errorf("checking MLflow status: %w", err)
	}

	title, ok := sectionTitle[platform]
	if !ok {
		title = sectionTitle[cluster.SelfManagedRhoai]
	}

	consoleLinkDomain, err := getGatewayDomain(ctx, cli)
	if err != nil {
		return nil, fmt.Errorf("getting gateway domain: %w", err)
	}

	gatewayURL := ""
	if consoleLinkDomain != "" {
		if strings.ContainsAny(consoleLinkDomain, "\n\r=") {
			return nil, fmt.Errorf("invalid gateway domain %q: contains illegal characters", consoleLinkDomain)
		}
		gatewayURL = consoleLinkDomain
	}

	return map[string]string{
		"gateway-url":    gatewayURL,
		"section-title":  title,
		"mlflow-enabled": strconv.FormatBool(mlflowEnabled),
	}, nil
}

// isMLflowEnabled checks whether an MLflowOperator singleton exists in the cluster.
// Passing a pre-keyed *unstructured.Unstructured avoids scheme registration for
// the upstream MLflowOperator type, which would conflict with the module's own
// Workbenches type registered at the same API group.
func isMLflowEnabled(ctx context.Context, cli client.Client) (bool, error) {
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(gvk.MLflowOperator)
	switch err := cluster.GetSingleton(ctx, cli, obj); {
	case err == nil:
		return true, nil
	case k8serr.IsNotFound(err), meta.IsNoMatchError(err):
		return false, nil
	default:
		return false, fmt.Errorf("getting MLflowOperator singleton: %w", err)
	}
}

// getGatewayDomain reads the gateway hostname from the data-science-gateway Gateway CR,
// falls back to GatewayConfig, then to cluster domain + default subdomain.
// Both lookups use *unstructured.Unstructured with a pre-set GVK so neither
// Gateway nor GatewayConfig needs to be registered in the manager scheme.
func getGatewayDomain(ctx context.Context, cli client.Client) (string, error) {
	gw := &unstructured.Unstructured{}
	gw.SetGroupVersionKind(gvk.KubernetesGateway)
	if err := cli.Get(ctx, client.ObjectKey{
		Name:      defaultGatewayName,
		Namespace: gatewayNamespace,
	}, gw); err == nil {
		if hostname, ok, _ := unstructured.NestedString(gw.Object, "spec", "listeners", "0", "hostname"); ok && hostname != "" {
			return hostname, nil
		}
	}

	gwCfg := &unstructured.Unstructured{}
	gwCfg.SetGroupVersionKind(gvk.GatewayConfig)
	switch err := cli.Get(ctx, client.ObjectKey{Name: serviceApi.GatewayConfigName}, gwCfg); {
	case err == nil:
		return getFQDNFromConfig(ctx, cli, gwCfg)
	case k8serr.IsNotFound(err), meta.IsNoMatchError(err):
		// GatewayConfig not present — derive from cluster domain.
		return getDefaultFQDN(ctx, cli)
	default:
		return "", fmt.Errorf("getting GatewayConfig: %w", err)
	}
}

func getFQDNFromConfig(ctx context.Context, cli client.Client, gwCfg *unstructured.Unstructured) (string, error) {
	subdomain, _, _ := unstructured.NestedString(gwCfg.Object, "spec", "subdomain")
	subdomain = strings.TrimSpace(subdomain)
	if subdomain == "" {
		subdomain = defaultGatewaySubdomain
	}

	baseDomain, _, _ := unstructured.NestedString(gwCfg.Object, "spec", "domain")
	if baseDomain = strings.TrimSpace(baseDomain); baseDomain != "" {
		return fmt.Sprintf("%s.%s", subdomain, baseDomain), nil
	}

	clusterDomain, err := cluster.GetDomain(ctx, cli)
	if err != nil {
		return "", fmt.Errorf("getting cluster domain: %w", err)
	}
	return fmt.Sprintf("%s.%s", subdomain, clusterDomain), nil
}

func getDefaultFQDN(ctx context.Context, cli client.Client) (string, error) {
	clusterDomain, err := cluster.GetDomain(ctx, cli)
	if err != nil {
		return "", fmt.Errorf("getting cluster domain: %w", err)
	}
	return fmt.Sprintf("%s.%s", defaultGatewaySubdomain, clusterDomain), nil
}
