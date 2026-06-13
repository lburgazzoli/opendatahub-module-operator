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
	"context"
	"fmt"

	odhtypes "github.com/opendatahub-io/odh-platform-utilities/framework/controller/types"
	fwparams "github.com/opendatahub-io/odh-platform-utilities/framework/utils/params"
	odhcluster "github.com/opendatahub-io/odh-platform-utilities/pkg/cluster"
	ofVersion "github.com/operator-framework/api/pkg/lib/version"

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
	overlay := overlayODH
	platform := componentApi.Platform(cfg.PlatformName)
	if platform == componentApi.Platform(odhcluster.SelfManagedRhoai) || platform == componentApi.Platform(odhcluster.ManagedRhoai) {
		overlay = overlayRhoai
	}

	mi := odhtypes.ManifestInfo{
		Path:       cfg.ManifestsPath,
		ContextDir: componentName,
		SourcePath: overlay,
	}

	// Apply image params once at startup (equivalent to Init in the monolith).
	if err := fwparams.Apply(
		mi.String(),
		"params.env",
		fwparams.Replacement(fwparams.FromEnv(imageParamMap)),
	); err != nil {
		return nil, fmt.Errorf("failed to update images on path %s: %w", mi, err)
	}

	return &Module{
		cfg:          cfg,
		manifestInfo: mi,
	}, nil
}

// initialize appends the pre-resolved manifest info to the pipeline.
func (m *Module) initialize(_ context.Context, rr *odhtypes.ReconciliationRequest) error {
	rr.Manifests = append(rr.Manifests, m.manifestInfo)
	return nil
}

// reportStatus populates the release status and config values.
func (m *Module) reportStatus(_ context.Context, rr *odhtypes.ReconciliationRequest) error {
	obj, ok := rr.Instance.(*componentApi.OGX)
	if !ok {
		return fmt.Errorf("instance is not an OGX")
	}

	obj.Status.Release = componentApi.Release{
		Name:    componentApi.Platform(rr.Release.Name),
		Version: ofVersion.OperatorVersion{Version: rr.Release.Version},
	}

	return nil
}
