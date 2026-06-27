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
	"fmt"
	"path"

	common "github.com/opendatahub-io/odh-platform-utilities/api/common"
	fwapi "github.com/opendatahub-io/odh-platform-utilities/framework/api"
	odhtypes "github.com/opendatahub-io/odh-platform-utilities/framework/controller/types"
	kparams "github.com/opendatahub-io/odh-platform-utilities/framework/render/kustomize/params"
	"sigs.k8s.io/kustomize/kyaml/filesys"

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

	componentMetadataPath = "manifests/modelregistry/component_metadata.yaml"

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
	cfg             *moduleconfig.Config
	platformRelease fwapi.Release
	releases        []common.ComponentRelease
	manifestInfo    odhtypes.ManifestInfo
	extraManifest   odhtypes.ManifestInfo
	kustomizeFS     filesys.FileSystem
}

// NewModule creates a Module with one-shot computed state.
func NewModule(cfg *moduleconfig.Config) (*Module, error) {
	kustomizeFS, err := newKustomizeFS()
	if err != nil {
		return nil, err
	}

	return &Module{
		cfg: cfg,
		manifestInfo: odhtypes.ManifestInfo{
			Path:       "manifests",
			ContextDir: componentName,
			SourcePath: baseManifestsSourcePath,
		},
		extraManifest: odhtypes.ManifestInfo{
			Path:       "manifests",
			ContextDir: componentName,
			SourcePath: path.Join(baseManifestsSourcePath, "extras"),
		},
		kustomizeFS: kustomizeFS,
	}, nil
}

// Init applies image and cert parameter substitutions once at process startup.
func (m *Module) Init() error {
	if err := kparams.Apply(
		m.kustomizeFS,
		path.Join(m.manifestInfo.String(), "params.env"),
		kparams.Replacement(kparams.FromEnv(imageParamMap)),
		kparams.Values(extraParamMap),
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
