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

package trustyai

import (
	"fmt"

	fwapi "github.com/opendatahub-io/odh-platform-utilities/framework/api"
	fwtypes "github.com/opendatahub-io/odh-platform-utilities/framework/controller/types"
	fwparams "github.com/opendatahub-io/odh-platform-utilities/framework/utils/params"

	componentApi "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-trustyai-operator/api/components/v1alpha1"
	moduleconfig "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-trustyai-operator/pkg/config"
)

const (
	componentName = componentApi.TrustyAIComponentName

	// LegacyComponentName matches the monolith's LegacyComponentName.
	LegacyComponentName = "trustyai"

	// overlays — note leading slash matches the monolith's overlaysSourcePaths.
	overlayODH   = "/overlays/odh"
	overlayRhoai = "/overlays/rhoai"
	overlayMCP   = "/overlays/mcp-guardrails"
)

// imageParamMap matches the monolith's imageParamMap (trustyai_support.go).
var imageParamMap = map[string]string{
	"trustyaiServiceImage":               "RELATED_IMAGE_ODH_TRUSTYAI_SERVICE_IMAGE",
	"trustyaiOperatorImage":              "RELATED_IMAGE_ODH_TRUSTYAI_SERVICE_OPERATOR_IMAGE",
	"lmes-driver-image":                  "RELATED_IMAGE_ODH_TA_LMES_DRIVER_IMAGE",
	"lmes-pod-image":                     "RELATED_IMAGE_ODH_TA_LMES_JOB_IMAGE",
	"guardrails-orchestrator-image":      "RELATED_IMAGE_ODH_FMS_GUARDRAILS_ORCHESTRATOR_IMAGE",
	"guardrails-sidecar-gateway-image":   "RELATED_IMAGE_ODH_TRUSTYAI_VLLM_ORCHESTRATOR_GATEWAY_IMAGE",
	"guardrails-built-in-detector-image": "RELATED_IMAGE_ODH_BUILT_IN_DETECTOR_IMAGE",
	"ragas-provider-image":               "RELATED_IMAGE_ODH_PYTHON_312_IMAGE",
	"garak-provider-image":               "RELATED_IMAGE_ODH_TRUSTYAI_GARAK_LLS_PROVIDER_DSP_IMAGE",
	"nemo-guardrails-image":              "RELATED_IMAGE_ODH_TRUSTYAI_NEMO_GUARDRAILS_SERVER_IMAGE",
	"evalHubImage":                       "RELATED_IMAGE_ODH_EVAL_HUB_IMAGE",
	"kube-rbac-proxy":                    "RELATED_IMAGE_ODH_KUBE_RBAC_PROXY_IMAGE",
}

// Module holds process-lifetime state for the trustyai controller.
type Module struct {
	cfg     *moduleconfig.Config
	release fwapi.Release
	// manifestInfo is the standard platform overlay (odh/rhoai).
	manifestInfo fwtypes.ManifestInfo
	// mcpManifestInfo is used when MCPGuardrailsMode is enabled.
	mcpManifestInfo fwtypes.ManifestInfo
}

// NewModule creates a Module with one-shot computed state.
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
		mcpManifestInfo: fwtypes.ManifestInfo{
			Path:       cfg.ManifestsPath,
			ContextDir: componentName,
			SourcePath: overlayMCP,
		},
	}, nil
}

// Init applies image parameter substitutions once at process startup.
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
