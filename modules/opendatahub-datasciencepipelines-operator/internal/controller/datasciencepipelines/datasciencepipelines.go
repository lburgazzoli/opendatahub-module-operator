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

package datasciencepipelines

import (
	"context"
	"fmt"
	"path"
	"sort"
	"strconv"

	componentApi "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-datasciencepipelines-operator/api/components/v1alpha1"
	moduleconfig "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-datasciencepipelines-operator/pkg/config"
	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-datasciencepipelines-operator/pkg/version"
	"github.com/opendatahub-io/opendatahub-operator/v2/api/common"
	"github.com/opendatahub-io/opendatahub-operator/v2/pkg/cluster"
	odherrors "github.com/opendatahub-io/opendatahub-operator/v2/pkg/controller/actions/errors"
	odhtypes "github.com/opendatahub-io/opendatahub-operator/v2/pkg/controller/types"
	odhdeploy "github.com/opendatahub-io/opendatahub-operator/v2/pkg/deploy"
)

const (
	ArgoWorkflowCRD     = "workflows.argoproj.io"
	componentName       = componentApi.DataSciencePipelinesComponentName
	LegacyComponentName = "data-science-pipelines-operator"

	platformVersionParamsKey          = "PLATFORMVERSION"
	fipsEnabledParamsKey              = "FIPSENABLED"
	argoWorkflowsControllersParamsKey = "ARGOWORKFLOWSCONTROLLERS"

	overlayODH   = "overlays/odh"
	overlayRhoai = "overlays/rhoai"
)

var (
	imageParamMap = map[string]string{
		"IMAGES_DSPO":                     "RELATED_IMAGE_ODH_DATA_SCIENCE_PIPELINES_OPERATOR_CONTROLLER_IMAGE",
		"IMAGES_APISERVER":                "RELATED_IMAGE_ODH_ML_PIPELINES_API_SERVER_V2_IMAGE",
		"IMAGES_PERSISTENCEAGENT":         "RELATED_IMAGE_ODH_ML_PIPELINES_PERSISTENCEAGENT_V2_IMAGE",
		"IMAGES_SCHEDULEDWORKFLOW":        "RELATED_IMAGE_ODH_ML_PIPELINES_SCHEDULEDWORKFLOW_V2_IMAGE",
		"IMAGES_ARGO_EXEC":                "RELATED_IMAGE_ODH_DATA_SCIENCE_PIPELINES_ARGO_ARGOEXEC_IMAGE",
		"IMAGES_ARGO_WORKFLOWCONTROLLER":  "RELATED_IMAGE_ODH_DATA_SCIENCE_PIPELINES_ARGO_WORKFLOWCONTROLLER_IMAGE",
		"IMAGES_DRIVER":                   "RELATED_IMAGE_ODH_ML_PIPELINES_DRIVER_IMAGE",
		"IMAGES_LAUNCHER":                 "RELATED_IMAGE_ODH_ML_PIPELINES_LAUNCHER_IMAGE",
		"IMAGES_MLMDGRPC":                 "RELATED_IMAGE_ODH_MLMD_GRPC_SERVER_IMAGE",
		"IMAGES_PIPELINESRUNTIMEGENERIC":  "RELATED_IMAGE_ODH_ML_PIPELINES_RUNTIME_GENERIC_IMAGE",
		"IMAGES_MLMDENVOY":                "RELATED_IMAGE_DSP_PROXYV2_IMAGE",
		"IMAGES_MARIADB":                  "RELATED_IMAGE_DSP_MARIADB_IMAGE",
		"IMAGES_TOOLBOX":                  "RELATED_IMAGE_DSP_TOOLBOX_IMAGE",
		"IMAGES_RHELAI":                   "RELATED_IMAGE_DSP_INSTRUCTLAB_NVIDIA_IMAGE",
		"kube-rbac-proxy":                 "RELATED_IMAGE_ODH_KUBE_RBAC_PROXY_IMAGE",
		"IMAGES_PIPELINES_COMPONENTS":     "RELATED_IMAGE_ODH_PIPELINES_COMPONENTS_IMAGE",
		"RELATED_IMAGE_ODH_AUTOML_IMAGE":  "RELATED_IMAGE_ODH_AUTOML_IMAGE",
		"RELATED_IMAGE_ODH_AUTORAG_IMAGE": "RELATED_IMAGE_ODH_AUTORAG_IMAGE",
	}

	ErrArgoWorkflowAPINotOwned = odherrors.NewStopError(
		"Failed upgrade. DataSciencePipelines component found existing Argo Workflow CRD, which is not managed by ODH.",
	)
	ErrArgoWorkflowCRDMissing = odherrors.NewStopError(
		"DataSciencePipelines component is configured not to manage Argo Workflow controllers, but workflows.argoproj.io CRD is missing.",
	)
)

type Module struct {
	cfg          *moduleconfig.Config
	version      componentApi.SemVer
	manifestInfo odhtypes.ManifestInfo
}

func NewModule(cfg *moduleconfig.Config) (*Module, error) {
	v, err := componentApi.NewSemVer(version.Version)
	if err != nil {
		return nil, fmt.Errorf("parsing module version %q: %w", version.Version, err)
	}

	overlay := overlayODH
	platform := common.Platform(cfg.PlatformType)
	if platform == cluster.SelfManagedRhoai || platform == cluster.ManagedRhoai {
		overlay = overlayRhoai
	}

	mi := odhtypes.ManifestInfo{
		Path:       cfg.ManifestsPath,
		ContextDir: componentName,
		SourcePath: overlay,
	}

	extraParams := map[string]string{
		platformVersionParamsKey: cfg.PlatformVersion,
		fipsEnabledParamsKey:     strconv.FormatBool(cluster.GetClusterInfo().FipsEnabled),
	}

	if err := odhdeploy.ApplyParams(paramsPath(cfg.ManifestsPath), "params.env", imageParamMap, extraParams); err != nil {
		return nil, fmt.Errorf("failed to update images on path %s: %w", paramsPath(cfg.ManifestsPath), err)
	}

	return &Module{
		cfg:          cfg,
		version:      v,
		manifestInfo: mi,
	}, nil
}

func paramsPath(basePath string) string {
	return path.Join(basePath, componentName, "base")
}

func (m *Module) initialize(_ context.Context, rr *odhtypes.ReconciliationRequest) error {
	rr.Manifests = append(rr.Manifests, m.manifestInfo)
	return nil
}

func (m *Module) reportStatus(_ context.Context, rr *odhtypes.ReconciliationRequest) error {
	obj, ok := rr.Instance.(*componentApi.DataSciencePipelines)
	if !ok {
		return fmt.Errorf("instance is not a DataSciencePipelines")
	}

	obj.Status.Module = componentApi.ModuleStatus{
		Version:     m.version,
		BuildSource: version.Repo + "@" + version.Branch + "/" + version.Commit,
		Platform: componentApi.PlatformStatus{
			Name:    string(rr.Release.Name),
			Version: componentApi.SemVer(rr.Release.Version.String()),
		},
	}

	var sources []componentApi.SourceStatus
	for _, manifest := range rr.Manifests {
		sources = append(sources, componentApi.SourceStatus{
			Path:     manifest.String(),
			Renderer: componentApi.SourceRendererKustomize,
		})
	}
	for _, t := range rr.Templates {
		sources = append(sources, componentApi.SourceStatus{
			Path:     t.Path,
			Renderer: componentApi.SourceRendererTemplate,
		})
	}
	for _, h := range rr.HelmCharts {
		sources = append(sources, componentApi.SourceStatus{
			Path:     h.Chart,
			Renderer: componentApi.SourceRendererHelm,
		})
	}

	sort.Slice(sources, func(i int, j int) bool {
		if sources[i].Path == sources[j].Path {
			return sources[i].Renderer < sources[j].Renderer
		}

		return sources[i].Path < sources[j].Path
	})

	obj.Status.Module.Sources = sources

	return nil
}
