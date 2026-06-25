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

package ray

import (
	"fmt"

	fwapi "github.com/opendatahub-io/odh-platform-utilities/framework/api"
	fwtypes "github.com/opendatahub-io/odh-platform-utilities/framework/controller/types"
	fwparams "github.com/opendatahub-io/odh-platform-utilities/framework/utils/params"

	componentApi "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-ray-operator/api/components/v1alpha1"
	moduleconfig "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-ray-operator/pkg/config"
)

const (
	componentName = componentApi.RayComponentName

	// LegacyComponentName is the label value assigned to deployments via
	// Kustomize. Deployment selectors are immutable so this must match
	// whatever the upstream manifests use.
	LegacyComponentName = "ray"

	overlayOpenShift = "openshift"
)

var imageParamMap = map[string]string{
	"odh-kuberay-operator-controller-image": "RELATED_IMAGE_ODH_KUBERAY_OPERATOR_CONTROLLER_IMAGE",
}

// Module holds process-lifetime state for the ray controller.
type Module struct {
	cfg          *moduleconfig.Config
	release      fwapi.Release
	manifestInfo fwtypes.ManifestInfo
}

// NewModule creates a Module with one-shot computed state.
func NewModule(cfg *moduleconfig.Config) (*Module, error) {
	return &Module{
		cfg: cfg,
		manifestInfo: fwtypes.ManifestInfo{
			Path:       cfg.ManifestsPath,
			ContextDir: componentName,
			SourcePath: overlayOpenShift,
		},
	}, nil
}

// Init applies image parameters from environment and parses the platform
// release version — called once at startup.
func (m *Module) Init() error {
	if err := fwparams.Apply(
		m.manifestInfo.String(),
		"params.env",
		fwparams.Replacement(fwparams.FromEnv(imageParamMap)),
	); err != nil {
		return fmt.Errorf("failed to update images on path %s: %w", m.manifestInfo, err)
	}

	m.release = m.cfg.PlatformRelease()

	return nil
}
