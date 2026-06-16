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

package workbenches

import (
	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	componentApi "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-workbenches-operator/api/components/v1alpha1"
	moduleconfig "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-workbenches-operator/pkg/config"
	fwtypes "github.com/opendatahub-io/odh-platform-utilities/framework/controller/types"
	fwparams "github.com/opendatahub-io/odh-platform-utilities/framework/utils/params"
	odhcluster "github.com/opendatahub-io/odh-platform-utilities/pkg/cluster"
)

// Module holds process-lifetime state for the workbenches controller.
type Module struct {
	cfg *moduleconfig.Config
	// manifestInfos is computed once at startup from the fixed platform and manifests path.
	manifestInfos []fwtypes.ManifestInfo

	// apiReader is the uncached reader used by webhooks and upgrade migrations
	// when they need fresh API state instead of informer-backed cache state.
	// The remaining webhook fields are set by RegisterWebhooks.
	decoder       admission.Decoder
	apiReader     client.Reader
	webhookClient client.Client
}

// NewModule creates a Module and pre-computes manifest paths.
func NewModule(cfg *moduleconfig.Config) (*Module, error) {
	platform := componentApi.Platform(cfg.PlatformName)

	imgSourcePath, ok := notebookImagesManifestSourcePath[platform]
	if !ok {
		imgSourcePath = notebookImagesManifestSourcePath[componentApi.Platform(odhcluster.OpenDataHub)]
	}

	return &Module{
		cfg: cfg,
		manifestInfos: []fwtypes.ManifestInfo{
			notebookControllerManifestInfo(cfg.ManifestsPath, notebookControllerManifestSourcePath),
			kfNotebookControllerManifestInfo(cfg.ManifestsPath, kfNotebookControllerManifestSourcePath),
			notebookImagesManifestInfo(cfg.ManifestsPath, imgSourcePath),
		},
	}, nil
}

// Init applies image parameter substitutions once at process startup.
func (m *Module) Init() error {
	if err := fwparams.Apply(
		m.manifestInfos[0].String(),
		"params.env",
		fwparams.Replacement(fwparams.FromEnv(map[string]string{
			"odh-notebook-controller-image": "RELATED_IMAGE_ODH_NOTEBOOK_CONTROLLER_IMAGE",
			"kube-rbac-proxy":               "RELATED_IMAGE_ODH_KUBE_RBAC_PROXY_IMAGE",
		})),
	); err != nil {
		return fmt.Errorf("updating notebook-controller image params: %w", err)
	}

	if err := fwparams.Apply(
		m.manifestInfos[1].String(),
		"params.env",
		fwparams.Replacement(fwparams.FromEnv(map[string]string{
			"odh-kf-notebook-controller-image": "RELATED_IMAGE_ODH_KF_NOTEBOOK_CONTROLLER_IMAGE",
		})),
	); err != nil {
		return fmt.Errorf("updating kf-notebook-controller image params: %w", err)
	}

	platform := componentApi.Platform(m.cfg.PlatformName)
	nbImgParamsPath := notebookImagesManifestInfo(m.cfg.ManifestsPath, notebookImagesParamsPath[platform])
	if err := fwparams.Apply(
		nbImgParamsPath.String(),
		"params-latest.env",
		fwparams.Replacement(fwparams.FromEnv(notebookImageParamMap)),
	); err != nil {
		return fmt.Errorf("updating notebook image params: %w", err)
	}

	return nil
}
