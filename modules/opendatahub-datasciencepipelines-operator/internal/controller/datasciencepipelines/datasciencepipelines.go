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
	"strconv"

	"sigs.k8s.io/controller-runtime/pkg/client"

	componentApi "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-datasciencepipelines-operator/api/components/v1alpha1"
	moduleconfig "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-datasciencepipelines-operator/pkg/config"
	fwapi "github.com/opendatahub-io/odh-platform-utilities/framework/api"
	fwerrors "github.com/opendatahub-io/odh-platform-utilities/framework/controller/actions/errors"
	fwtypes "github.com/opendatahub-io/odh-platform-utilities/framework/controller/types"
	fwparams "github.com/opendatahub-io/odh-platform-utilities/framework/utils/params"
	odhcluster "github.com/opendatahub-io/odh-platform-utilities/pkg/cluster"
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

	ErrArgoWorkflowAPINotOwned = fwerrors.NewStopError(
		"Failed upgrade. DataSciencePipelines component found existing Argo Workflow CRD, which is not managed by ODH.",
	)
	ErrArgoWorkflowCRDMissing = fwerrors.NewStopError(
		"DataSciencePipelines component is configured not to manage Argo Workflow controllers, but workflows.argoproj.io CRD is missing.",
	)
)

type Module struct {
	cfg          *moduleconfig.Config
	release      fwapi.Release
	manifestInfo fwtypes.ManifestInfo
}

func NewModule(cfg *moduleconfig.Config) (*Module, error) {
	overlay := overlayODH
	switch cfg.PlatformType {
	case moduleconfig.PlatformTypeSelfManagedRhoai, moduleconfig.PlatformTypeManagedRhoai:
		overlay = overlayRhoai
	}

	return &Module{
		cfg: cfg,
		manifestInfo: fwtypes.ManifestInfo{
			Path:       cfg.ManifestsPath,
			ContextDir: componentName,
			SourcePath: overlay,
		},
	}, nil
}

func (m *Module) Init(ctx context.Context, reader client.Reader) error {
	info, err := odhcluster.DetectClusterInfo(ctx, reader)
	if err != nil {
		return fmt.Errorf("detecting cluster info: %w", err)
	}

	pp := path.Join(m.cfg.ManifestsPath, componentName, "base")

	if err := fwparams.Apply(
		pp,
		"params.env",
		fwparams.Replacement(
			fwparams.FromEnv(imageParamMap),
		),
		fwparams.Values(map[string]string{
			platformVersionParamsKey: m.cfg.PlatformVersion.String(),
			fipsEnabledParamsKey:     strconv.FormatBool(info.FipsEnabled),
		}),
	); err != nil {
		return fmt.Errorf("failed to update params on path %s: %w", pp, err)
	}

	m.release = m.cfg.PlatformRelease()

	return nil
}
