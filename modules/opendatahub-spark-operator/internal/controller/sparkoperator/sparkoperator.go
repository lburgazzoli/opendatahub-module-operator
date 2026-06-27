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

package sparkoperator

import (
	"fmt"
	"path"

	common "github.com/opendatahub-io/odh-platform-utilities/api/common"
	fwapi "github.com/opendatahub-io/odh-platform-utilities/framework/api"
	odhtypes "github.com/opendatahub-io/odh-platform-utilities/framework/controller/types"
	kparams "github.com/opendatahub-io/odh-platform-utilities/framework/render/kustomize/params"
	"sigs.k8s.io/kustomize/kyaml/filesys"

	componentApi "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-spark-operator/api/components/v1alpha1"
	moduleconfig "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-spark-operator/pkg/config"
)

const (
	componentName = componentApi.SparkOperatorComponentName

	overlayODH   = "overlays/odh"
	overlayRhoai = "overlays/rhoai"
)

// imageParamMap matches the monolith's imageParamMap.
var imageParamMap = map[string]string{
	"SPARK_OPERATOR_CONTROLLER_IMAGE": "RELATED_IMAGE_ODH_SPARK_OPERATOR_IMAGE",
	"SPARK_OPERATOR_WEBHOOK_IMAGE":    "RELATED_IMAGE_ODH_SPARK_OPERATOR_IMAGE",
}

// Module holds process-lifetime state for the sparkoperator controller.
type Module struct {
	cfg             *moduleconfig.Config
	platformRelease fwapi.Release
	releases        []common.ComponentRelease
	manifestInfo    odhtypes.ManifestInfo
	kustomizeFS     filesys.FileSystem
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
		manifestInfo: odhtypes.ManifestInfo{
			Path:       "manifests",
			ContextDir: componentName,
			SourcePath: overlay,
		},
		kustomizeFS: kustomizeFS,
	}, nil
}

// Init applies image parameter substitutions from environment variables into
// the manifest path. Must be called once after NewModule, before the reconciler
// starts processing requests.
func (m *Module) Init() error {
	if err := kparams.Apply(
		m.kustomizeFS,
		path.Join(m.manifestInfo.String(), "params.env"),
		kparams.Replacement(kparams.FromEnv(imageParamMap)),
	); err != nil {
		return fmt.Errorf("failed to update images on path %s: %w", m.manifestInfo, err)
	}

	releases, err := m.loadReleases()
	if err != nil {
		return fmt.Errorf("failed to load releases: %w", err)
	}

	m.platformRelease = m.cfg.PlatformRelease()
	m.releases = releases

	return nil
}
