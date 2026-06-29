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
	"fmt"
	"path"

	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-mlflow-operator/assets"
	common "github.com/opendatahub-io/odh-platform-utilities/api/common"
	fwapi "github.com/opendatahub-io/odh-platform-utilities/framework/api"
	fwreleases "github.com/opendatahub-io/odh-platform-utilities/framework/controller/actions/status/releases"
	fwtypes "github.com/opendatahub-io/odh-platform-utilities/framework/controller/types"
	kparams "github.com/opendatahub-io/odh-platform-utilities/framework/render/kustomize/params"
	"sigs.k8s.io/kustomize/kyaml/filesys"

	componentApi "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-mlflow-operator/api/components/v1alpha1"
	moduleconfig "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-mlflow-operator/pkg/config"
)

const (
	componentName         = componentApi.MLflowOperatorComponentName
	manifestsRoot         = "manifests"
	componentMetadataFile = "component_metadata.yaml"

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
	platformRelease fwapi.Release
	releases        []common.ComponentRelease
	manifestInfo    fwtypes.ManifestInfo
	kustomizeFS     filesys.FileSystem
	// consoleSectionTitle is the section-title kustomize variable, computed once from platform.
	consoleSectionTitle string
}

func consoleSectionTitleFor(platformType string) string {
	if platformType == moduleconfig.PlatformTypeSelfManagedRhoai || platformType == moduleconfig.PlatformTypeManagedRhoai {
		return "OpenShift Self Managed Services"
	}
	return "OpenShift Open Data Hub"
}

// NewModule creates a Module with one-shot computed state.
func NewModule(cfg *moduleconfig.Config) (*Module, error) {
	kustomizeFS, err := newKustomizeFS()
	if err != nil {
		return nil, err
	}

	overlay := overlayODH
	switch cfg.PlatformType {
	case moduleconfig.PlatformTypeSelfManagedRhoai, moduleconfig.PlatformTypeManagedRhoai:
		overlay = overlayRhoai
	}

	return &Module{
		cfg: cfg,
		manifestInfo: fwtypes.ManifestInfo{
			Path:       manifestsRoot,
			ContextDir: componentName,
			SourcePath: overlay,
		},
		kustomizeFS:         kustomizeFS,
		consoleSectionTitle: consoleSectionTitleFor(cfg.PlatformType),
	}, nil
}

// Init applies image parameters to base/ from environment — called once at startup.
func (m *Module) Init() error {
	baseParamsPath := path.Join(manifestsRoot, componentName, paramsSubDir)
	if err := kparams.Apply(
		m.kustomizeFS,
		path.Join(baseParamsPath, "params.env"),
		kparams.Replacement(kparams.FromEnv(imageParamMap)),
	); err != nil {
		return fmt.Errorf("failed to update images on path %s: %w", baseParamsPath, err)
	}

	releases, err := m.loadReleases()
	if err != nil {
		return fmt.Errorf("failed to load releases: %w", err)
	}

	m.releases = releases
	m.platformRelease = m.cfg.PlatformRelease()

	return nil
}

func (m *Module) loadReleases() ([]common.ComponentRelease, error) {
	metadataPath := path.Join(manifestsRoot, componentName, componentMetadataFile)
	releases, err := fwreleases.ReadComponentReleases(assets.Manifests, metadataPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read component metadata from %s: %w", metadataPath, err)
	}

	return fwreleases.NormalizeComponentReleases(append(releases, m.cfg.ComponentRelease())), nil
}
