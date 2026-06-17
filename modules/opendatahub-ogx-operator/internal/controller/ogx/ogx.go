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

package ogx

import (
	"fmt"

	odhtypes "github.com/opendatahub-io/odh-platform-utilities/framework/controller/types"
	fwparams "github.com/opendatahub-io/odh-platform-utilities/framework/utils/params"
	odhcluster "github.com/opendatahub-io/odh-platform-utilities/pkg/cluster"

	componentApi "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-ogx-operator/api/components/v1alpha1"
	moduleconfig "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-ogx-operator/pkg/config"
)

const (
	componentName = componentApi.OGXComponentName

	overlayODH   = "overlays/odh"
	overlayRhoai = "overlays/rhoai"
)

// imageParamMap matches the monolith's imageParamMap.
var imageParamMap = map[string]string{
	"RELATED_IMAGE_ODH_OGX_OPERATOR": "RELATED_IMAGE_ODH_OGX_K8S_OPERATOR_IMAGE",
	"RELATED_IMAGE_RH_DISTRIBUTION":  "RELATED_IMAGE_ODH_OGX_CORE_IMAGE",
}

// Module holds process-lifetime state for the ogx controller.
type Module struct {
	cfg          *moduleconfig.Config
	manifestInfo odhtypes.ManifestInfo
}

// NewModule creates a Module with one-shot computed state.
func NewModule(cfg *moduleconfig.Config) (*Module, error) {
	var overlay string
	switch odhcluster.Platform(cfg.PlatformName) {
	case odhcluster.SelfManagedRhoai:
		overlay = overlayRhoai
	case odhcluster.ManagedRhoai:
		overlay = overlayRhoai
	default:
		overlay = overlayODH
	}

	return &Module{
		cfg: cfg,
		manifestInfo: odhtypes.ManifestInfo{
			Path:       cfg.ManifestsPath,
			ContextDir: componentName,
			SourcePath: overlay,
		},
	}, nil
}

// Init applies image parameter substitutions to the manifest directory.
// It must be called once after NewModule, before the reconciler starts.
func (m *Module) Init() error {
	if err := fwparams.Apply(
		m.manifestInfo.String(),
		"params.env",
		fwparams.Replacement(fwparams.FromEnv(imageParamMap)),
	); err != nil {
		return fmt.Errorf("failed to update images on path %s: %w", m.manifestInfo, err)
	}
	return nil
}
