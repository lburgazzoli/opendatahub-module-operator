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

package mlflowoperator

import (
	"context"
	"fmt"
	"path"

	"github.com/opendatahub-io/opendatahub-operator/v2/api/common"
	"github.com/opendatahub-io/opendatahub-operator/v2/pkg/cluster"
	odhtypes "github.com/opendatahub-io/opendatahub-operator/v2/pkg/controller/types"
	odhdeploy "github.com/opendatahub-io/opendatahub-operator/v2/pkg/deploy"

	componentApi "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-mlflow-operator/api/components/v1alpha1"
	moduleconfig "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-mlflow-operator/pkg/config"
)

const (
	componentName = componentApi.MLflowOperatorComponentName

	overlayODH   = "overlays/odh"
	overlayRhoai = "overlays/rhoai"

	// paramsBasePath is the directory within the manifest context where params.env lives.
	// The monolith writes params to base/, not to the overlay.
	paramsSubDir = "base"
)

// imageParamMap matches the monolith's imageParamMap.
var imageParamMap = map[string]string{
	"MLFLOW_IMAGE":          "RELATED_IMAGE_ODH_MLFLOW_IMAGE",
	"MLFLOW_OPERATOR_IMAGE": "RELATED_IMAGE_ODH_MLFLOW_OPERATOR_IMAGE",
	"KUBE_AUTH_PROXY_IMAGE": "RELATED_IMAGE_ODH_KUBE_RBAC_PROXY_IMAGE",
}

// Module holds process-lifetime state for the mlflowoperator controller.
type Module struct {
	cfg          *moduleconfig.Config
	manifestInfo odhtypes.ManifestInfo
	// consoleSectionTitle is the section-title kustomize variable, computed once from platform.
	consoleSectionTitle string
}

func consoleSectionTitleFor(platform common.Platform) string {
	if platform == cluster.SelfManagedRhoai || platform == cluster.ManagedRhoai {
		return "OpenShift Self Managed Services"
	}
	return "OpenShift Open Data Hub"
}

// NewModule creates a Module with one-shot computed state.
func NewModule(cfg *moduleconfig.Config) (*Module, error) {
	platform := common.Platform(cfg.PlatformName)
	overlay := overlayODH
	if platform == cluster.SelfManagedRhoai || platform == cluster.ManagedRhoai {
		overlay = overlayRhoai
	}

	mi := odhtypes.ManifestInfo{
		Path:       cfg.ManifestsPath,
		ContextDir: componentName,
		SourcePath: overlay,
	}

	// Apply image params to base/ (not the overlay) — matches the monolith's paramsPath.
	baseParamsPath := path.Join(cfg.ManifestsPath, componentName, paramsSubDir)
	if err := odhdeploy.ApplyParams(baseParamsPath, "params.env", imageParamMap); err != nil {
		return nil, fmt.Errorf("failed to update images on path %s: %w", baseParamsPath, err)
	}

	return &Module{
		cfg:                 cfg,
		manifestInfo:        mi,
		consoleSectionTitle: consoleSectionTitleFor(platform),
	}, nil
}

// initialize appends the pre-resolved manifest info to the pipeline.
func (m *Module) initialize(_ context.Context, rr *odhtypes.ReconciliationRequest) error {
	rr.Manifests = append(rr.Manifests, m.manifestInfo)
	return nil
}

// reportStatus populates the release status and config values.
func (m *Module) reportStatus(_ context.Context, rr *odhtypes.ReconciliationRequest) error {
	obj, ok := rr.Instance.(*componentApi.MLflowOperator)
	if !ok {
		return fmt.Errorf("instance is not a MLflowOperator")
	}

	obj.Status.Release = common.Release{
		Name:    rr.Release.Name,
		Version: rr.Release.Version,
	}

	return nil
}
