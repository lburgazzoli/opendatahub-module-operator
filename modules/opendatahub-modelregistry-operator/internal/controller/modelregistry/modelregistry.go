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

	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-modelregistry-operator/assets"
	common "github.com/opendatahub-io/odh-platform-utilities/api/common"
	fwapi "github.com/opendatahub-io/odh-platform-utilities/framework/api"
	"sigs.k8s.io/kustomize/kyaml/filesys"

	componentApi "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-modelregistry-operator/api/components/v1alpha1"
	moduleconfig "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-modelregistry-operator/pkg/config"
	modulemeta "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-modelregistry-operator/pkg/module"
)

const (
	componentName = componentApi.ModelRegistryComponentName

	// LegacyComponentName is the label value assigned to deployments via Kustomize.
	// Deployment selectors are immutable so this must match whatever the upstream manifests use.
	LegacyComponentName = "model-registry-operator"

	componentMetadataPath = "manifests/modelregistry/component_metadata.yaml"

	// Gateway constants copied from the monolith's internal/controller/services/gateway package.
	// These are stable platform-level values; define locally to avoid importing internal packages.
	defaultGatewayName = "data-science-gateway"
	gatewayNamespace   = "openshift-ingress"
)

// Module holds process-lifetime state for the modelregistry controller.
type Module struct {
	cfg             *moduleconfig.Config
	variant         modulemeta.ResolvedVariant
	platformRelease fwapi.Release
	releases        []common.ComponentRelease
	kustomizeFS     filesys.FileSystem
}

// NewModule creates a Module with one-shot computed state.
// Model Registry currently renders from the ODH manifest layout and extras
// only; platformType is reported in status/config but does not switch overlays.
func NewModule(cfg *moduleconfig.Config) (*Module, error) {
	kustomizeFS, err := newKustomizeFS()
	if err != nil {
		return nil, err
	}

	variant, err := modulemeta.LoadVariant(
		assets.Manifests,
		modulemeta.DescriptorPath,
		modulemeta.VariantODH,
	)
	if err != nil {
		return nil, fmt.Errorf("loading variant %q: %w", modulemeta.VariantODH, err)
	}

	return &Module{
		cfg:         cfg,
		variant:     variant,
		kustomizeFS: kustomizeFS,
	}, nil
}

// Init applies static parameter substitutions once at process startup.
func (m *Module) Init() error {
	if err := modulemeta.ApplyStaticParams(m.kustomizeFS, m.variant.Kustomize); err != nil {
		return err
	}

	releases, err := m.loadReleases()
	if err != nil {
		return fmt.Errorf("failed to load releases: %w", err)
	}

	m.releases = releases
	m.platformRelease = m.cfg.PlatformRelease()

	return nil
}
