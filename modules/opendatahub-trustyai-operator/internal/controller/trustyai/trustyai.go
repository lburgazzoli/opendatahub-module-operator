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
	"context"
	"fmt"

	"github.com/opendatahub-io/opendatahub-operator/v2/api/common"
	"github.com/opendatahub-io/opendatahub-operator/v2/pkg/cluster"
	odhtypes "github.com/opendatahub-io/opendatahub-operator/v2/pkg/controller/types"
	odhdeploy "github.com/opendatahub-io/opendatahub-operator/v2/pkg/deploy"

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
	cfg *moduleconfig.Config
	// manifestInfo is the standard platform overlay (odh/rhoai).
	manifestInfo odhtypes.ManifestInfo
	// mcpManifestInfo is used when MCPGuardrailsMode is enabled.
	mcpManifestInfo odhtypes.ManifestInfo
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

	mcpMI := odhtypes.ManifestInfo{
		Path:       cfg.ManifestsPath,
		ContextDir: componentName,
		SourcePath: overlayMCP,
	}

	// Apply image params once at startup (equivalent to Init in the monolith).
	if err := odhdeploy.ApplyParams(mi.String(), "params.env", imageParamMap); err != nil {
		return nil, fmt.Errorf("failed to update images on path %s: %w", mi, err)
	}

	return &Module{
		cfg:             cfg,
		manifestInfo:    mi,
		mcpManifestInfo: mcpMI,
	}, nil
}

// initialize selects the manifest overlay based on MCPGuardrailsMode.
func (m *Module) initialize(_ context.Context, rr *odhtypes.ReconciliationRequest) error {
	tai, ok := rr.Instance.(*componentApi.TrustyAI)
	if !ok {
		return fmt.Errorf("instance is not a TrustyAI")
	}

	if tai.Spec.MCPGuardrailsMode {
		rr.Manifests = append(rr.Manifests, m.mcpManifestInfo)
	} else {
		rr.Manifests = append(rr.Manifests, m.manifestInfo)
	}

	return nil
}

// reportStatus populates the release status and config values.
func (m *Module) reportStatus(_ context.Context, rr *odhtypes.ReconciliationRequest) error {
	obj, ok := rr.Instance.(*componentApi.TrustyAI)
	if !ok {
		return fmt.Errorf("instance is not a TrustyAI")
	}

	obj.Status.Release = common.Release{
		Name:    rr.Release.Name,
		Version: rr.Release.Version,
	}

	return nil
}
