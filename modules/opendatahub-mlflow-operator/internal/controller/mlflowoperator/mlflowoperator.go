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
	"sort"

	"github.com/opendatahub-io/opendatahub-operator/v2/api/common"
	"github.com/opendatahub-io/opendatahub-operator/v2/pkg/cluster"
	odhtypes "github.com/opendatahub-io/opendatahub-operator/v2/pkg/controller/types"
	odhdeploy "github.com/opendatahub-io/opendatahub-operator/v2/pkg/deploy"

	componentApi "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-mlflow-operator/api/components/v1alpha1"
	moduleconfig "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-mlflow-operator/pkg/config"
	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-mlflow-operator/pkg/version"
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
	cfg             *moduleconfig.Config
	version         componentApi.SemVer
	platformVersion componentApi.SemVer
	manifestInfo    odhtypes.ManifestInfo
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
	v, err := componentApi.NewSemVer(version.Version)
	if err != nil {
		return nil, fmt.Errorf("parsing module version %q: %w", version.Version, err)
	}

	pv, _ := componentApi.NewSemVer(cfg.PlatformVersion)

	platform := common.Platform(cfg.PlatformType)
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
		version:             v,
		platformVersion:     pv,
		manifestInfo:        mi,
		consoleSectionTitle: consoleSectionTitleFor(platform),
	}, nil
}

// initialize appends the pre-resolved manifest info to the pipeline.
func (m *Module) initialize(_ context.Context, rr *odhtypes.ReconciliationRequest) error {
	rr.Manifests = append(rr.Manifests, m.manifestInfo)
	return nil
}

// reportStatus populates the module status with version, platform, and source information.
func (m *Module) reportStatus(_ context.Context, rr *odhtypes.ReconciliationRequest) error {
	obj, ok := rr.Instance.(*componentApi.MLflowOperator)
	if !ok {
		return fmt.Errorf("instance is not a MLflowOperator")
	}

	obj.Status.Module = componentApi.ModuleStatus{
		Version:     m.version,
		BuildSource: version.Repo + "@" + version.Branch + "/" + version.Commit,
		Platform: componentApi.PlatformStatus{
			Name:    m.cfg.PlatformType,
			Version: m.platformVersion,
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
		sources = append(sources, componentApi.SourceStatus{Path: t.Path, Renderer: componentApi.SourceRendererTemplate})
	}
	for _, h := range rr.HelmCharts {
		sources = append(sources, componentApi.SourceStatus{Path: h.Chart, Renderer: componentApi.SourceRendererHelm})
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
