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

package modelregistry

import (
	"context"
	"fmt"
	"path"

	odhtypes "github.com/opendatahub-io/odh-platform-utilities/framework/controller/types"
	fwparams "github.com/opendatahub-io/odh-platform-utilities/framework/utils/params"
	ofVersion "github.com/operator-framework/api/pkg/lib/version"

	componentApi "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-modelregistry-operator/api/components/v1alpha1"
	moduleconfig "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-modelregistry-operator/pkg/config"
)

const (
	componentName = componentApi.ModelRegistryComponentName

	// LegacyComponentName is the label value assigned to deployments via Kustomize.
	// Deployment selectors are immutable so this must match whatever the upstream manifests use.
	LegacyComponentName = "model-registry-operator"

	baseManifestsSourcePath = "overlays/odh"

	defaultModelRegistryCert = "default-modelregistry-cert"

	// Gateway constants copied from the monolith's internal/controller/services/gateway package.
	// These are stable platform-level values; define locally to avoid importing internal packages.
	defaultGatewayName = "data-science-gateway"
	gatewayNamespace   = "openshift-ingress"
)

var imageParamMap = map[string]string{
	"IMAGES_MODELREGISTRY_OPERATOR": "RELATED_IMAGE_ODH_MODEL_REGISTRY_OPERATOR_IMAGE",
	"IMAGES_REST_SERVICE":           "RELATED_IMAGE_ODH_MODEL_REGISTRY_IMAGE",
	"IMAGES_CATALOG_DATA":           "RELATED_IMAGE_ODH_MODEL_METADATA_COLLECTION_IMAGE",
	"IMAGES_BENCHMARK_DATA":         "RELATED_IMAGE_ODH_MODEL_PERFORMANCE_DATA_IMAGE",
	"IMAGES_JOBS_ASYNC_UPLOAD":      "RELATED_IMAGE_ODH_MODEL_REGISTRY_JOB_ASYNC_UPLOAD_IMAGE",
	"kube-rbac-proxy":               "RELATED_IMAGE_ODH_KUBE_RBAC_PROXY_IMAGE",
	"IMAGES_POSTGRES":               "RELATED_IMAGE_POSTGRESQL_16_IMAGE",
}

var extraParamMap = map[string]string{
	"DEFAULT_CERT": defaultModelRegistryCert,
}

// Module holds process-lifetime state for the modelregistry controller.
type Module struct {
	cfg           *moduleconfig.Config
	manifestInfo  odhtypes.ManifestInfo
	extraManifest odhtypes.ManifestInfo
}

// NewModule creates a Module with one-shot computed state.
func NewModule(cfg *moduleconfig.Config) (*Module, error) {
	mi := odhtypes.ManifestInfo{
		Path:       cfg.ManifestsPath,
		ContextDir: componentName,
		SourcePath: baseManifestsSourcePath,
	}

	extraMi := odhtypes.ManifestInfo{
		Path:       cfg.ManifestsPath,
		ContextDir: componentName,
		SourcePath: path.Join(baseManifestsSourcePath, "extras"),
	}

	// Apply image and cert parameters once at startup (equivalent to Init in the monolith).
	if err := fwparams.Apply(
		mi.String(),
		"params.env",
		fwparams.Replacement(fwparams.FromEnv(imageParamMap)),
		fwparams.Values(extraParamMap),
	); err != nil {
		return nil, fmt.Errorf("failed to update images on path %s: %w", mi, err)
	}

	return &Module{
		cfg:           cfg,
		manifestInfo:  mi,
		extraManifest: extraMi,
	}, nil
}

// initialize sets the per-reconcile manifest list.
func (m *Module) initialize(_ context.Context, rr *odhtypes.ReconciliationRequest) error {
	rr.Manifests = []odhtypes.ManifestInfo{
		m.manifestInfo,
		m.extraManifest,
	}
	return nil
}

// reportStatus populates the release status and platform.
func (m *Module) reportStatus(_ context.Context, rr *odhtypes.ReconciliationRequest) error {
	obj, ok := rr.Instance.(*componentApi.ModelRegistry)
	if !ok {
		return fmt.Errorf("instance is not a ModelRegistry")
	}

	obj.Status.Release = componentApi.Release{
		Name:    componentApi.Platform(rr.Release.Name),
		Version: ofVersion.OperatorVersion{Version: rr.Release.Version},
	}

	return nil
}
