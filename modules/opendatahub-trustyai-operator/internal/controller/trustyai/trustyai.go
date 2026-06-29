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
	"path"

	componentApi "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-trustyai-operator/api/components/v1alpha1"
	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-trustyai-operator/assets"
	moduleconfig "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-trustyai-operator/pkg/config"
	common "github.com/opendatahub-io/odh-platform-utilities/api/common"
	fwapi "github.com/opendatahub-io/odh-platform-utilities/framework/api"
	fwreleases "github.com/opendatahub-io/odh-platform-utilities/framework/controller/actions/status/releases"
	fwtypes "github.com/opendatahub-io/odh-platform-utilities/framework/controller/types"
	kparams "github.com/opendatahub-io/odh-platform-utilities/framework/render/kustomize/params"
	"sigs.k8s.io/kustomize/kyaml/filesys"
)

const (
	componentName = componentApi.TrustyAIComponentName

	// LegacyComponentName matches the monolith's LegacyComponentName.
	LegacyComponentName = "trustyai"

	overlayODH   = "overlays/odh"
	overlayRhoai = "overlays/rhoai"
	overlayMCP   = "overlays/mcp-guardrails"
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
	cfg             *moduleconfig.Config
	platformRelease fwapi.Release
	releases        []common.ComponentRelease
	// manifestInfo is the standard platform overlay (odh/rhoai).
	manifestInfo fwtypes.ManifestInfo
	// mcpManifestInfo is used when MCPGuardrailsMode is enabled.
	mcpManifestInfo fwtypes.ManifestInfo
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
		manifestInfo: fwtypes.ManifestInfo{
			Path:       "manifests",
			ContextDir: componentName,
			SourcePath: overlay,
		},
		mcpManifestInfo: fwtypes.ManifestInfo{
			Path:       "manifests",
			ContextDir: componentName,
			SourcePath: overlayMCP,
		},
		kustomizeFS: kustomizeFS,
	}, nil
}

// Init applies image parameter substitutions once at process startup.
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

	m.releases = releases
	m.platformRelease = m.cfg.PlatformRelease()

	return nil
}

func (m *Module) loadReleases() ([]common.ComponentRelease, error) {
	releases, err := fwreleases.ReadComponentReleases(
		assets.Manifests,
		path.Join("manifests", componentName, "component_metadata.yaml"),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to read component metadata: %w", err)
	}

	return fwreleases.NormalizeComponentReleases(append(releases, m.cfg.ComponentRelease())), nil
}
