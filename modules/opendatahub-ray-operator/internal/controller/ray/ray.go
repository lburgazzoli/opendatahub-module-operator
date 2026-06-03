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
	"context"
	"fmt"

	"github.com/opendatahub-io/opendatahub-operator/v2/api/common"
	odhtypes "github.com/opendatahub-io/opendatahub-operator/v2/pkg/controller/types"
	odhdeploy "github.com/opendatahub-io/opendatahub-operator/v2/pkg/deploy"

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
	manifestInfo odhtypes.ManifestInfo
}

// NewModule creates a Module with one-shot computed state.
func NewModule(cfg *moduleconfig.Config) (*Module, error) {
	mi := odhtypes.ManifestInfo{
		Path:       cfg.ManifestsPath,
		ContextDir: componentName,
		SourcePath: overlayOpenShift,
	}

	// Apply image parameters once at startup (equivalent to Init in the
	// monolith's componentHandler).
	if err := odhdeploy.ApplyParams(mi.String(), "params.env", imageParamMap); err != nil {
		return nil, fmt.Errorf("failed to update images on path %s: %w", mi, err)
	}

	return &Module{
		cfg:          cfg,
		manifestInfo: mi,
	}, nil
}

// initialize appends manifests and applies image/namespace parameters.
func (m *Module) initialize(_ context.Context, rr *odhtypes.ReconciliationRequest) error {
	rr.Manifests = append(rr.Manifests, m.manifestInfo)

	if err := odhdeploy.ApplyParams(
		m.manifestInfo.String(),
		"params.env",
		nil,
		map[string]string{"namespace": m.cfg.ApplicationsNamespace},
	); err != nil {
		return fmt.Errorf("failed to update params.env: %w", err)
	}

	return nil
}

// reportStatus populates the release status with platform version and name.
func (m *Module) reportStatus(_ context.Context, rr *odhtypes.ReconciliationRequest) error {
	obj, ok := rr.Instance.(*componentApi.Ray)
	if !ok {
		return fmt.Errorf("instance is not a Ray")
	}

	obj.Status.Release = common.Release{
		Name:    rr.Release.Name,
		Version: rr.Release.Version,
	}

	return nil
}
