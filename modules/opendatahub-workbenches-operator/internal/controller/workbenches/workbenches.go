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
	"sigs.k8s.io/kustomize/kyaml/filesys"

	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-workbenches-operator/assets"
	moduleconfig "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-workbenches-operator/pkg/config"
	modulemeta "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-workbenches-operator/pkg/module"
	common "github.com/opendatahub-io/odh-platform-utilities/api/common"
	fwapi "github.com/opendatahub-io/odh-platform-utilities/framework/api"
)

// Module holds process-lifetime state for the workbenches controller.
type Module struct {
	cfg      *moduleconfig.Config
	release  fwapi.Release
	releases []common.ComponentRelease
	variant  modulemeta.ResolvedVariant
	renderFS filesys.FileSystem

	// apiReader is the uncached reader used by webhooks and upgrade migrations
	// when they need fresh API state instead of informer-backed cache state.
	// The remaining webhook fields are set by RegisterWebhooks.
	decoder       admission.Decoder
	apiReader     client.Reader
	webhookClient client.Client
}

// NewModule creates a Module and pre-computes manifest paths.
// Workbenches intentionally renders from the embedded ODH layout; platformType
// affects namespace defaults and params, but does not switch the manifest roots.
func NewModule(cfg *moduleconfig.Config) (*Module, error) {
	renderFS, err := newKustomizeFS()
	if err != nil {
		return nil, err
	}

	variantName := modulemeta.VariantODH
	switch cfg.PlatformType {
	case moduleconfig.PlatformTypeSelfManagedRhoai, moduleconfig.PlatformTypeManagedRhoai:
		variantName = modulemeta.VariantRhoai
	}

	variant, err := modulemeta.LoadVariant(
		assets.Manifests,
		modulemeta.DescriptorPath,
		variantName,
	)
	if err != nil {
		return nil, fmt.Errorf("loading variant %q: %w", variantName, err)
	}

	return &Module{
		cfg:      cfg,
		variant:  variant,
		renderFS: renderFS,
	}, nil
}

// Init applies image parameter substitutions once at process startup.
func (m *Module) Init() error {
	if err := modulemeta.ApplyStaticParams(m.renderFS, m.variant.Kustomize); err != nil {
		return fmt.Errorf("applying static params for variant %q: %w", m.variant.Name, err)
	}

	releases, err := loadReleases(m.cfg)
	if err != nil {
		return fmt.Errorf("loading releases: %w", err)
	}

	m.release = m.cfg.PlatformRelease()
	m.releases = releases

	return nil
}
