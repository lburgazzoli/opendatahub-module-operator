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
	"errors"
	"fmt"
	iofs "io/fs"
	"path"

	componentApi "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-trustyai-operator/api/components/v1alpha1"
	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-trustyai-operator/assets"
	moduleconfig "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-trustyai-operator/pkg/config"
	modulemeta "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-trustyai-operator/pkg/module"
	common "github.com/opendatahub-io/odh-platform-utilities/api/common"
	fwapi "github.com/opendatahub-io/odh-platform-utilities/framework/api"
	fwreleases "github.com/opendatahub-io/odh-platform-utilities/framework/controller/actions/status/releases"
	"sigs.k8s.io/kustomize/kyaml/filesys"
)

const (
	componentName = componentApi.TrustyAIComponentName

	// LegacyComponentName matches the monolith's LegacyComponentName.
	LegacyComponentName = "trustyai"
)

// Module holds process-lifetime state for the trustyai controller.
type Module struct {
	cfg             *moduleconfig.Config
	variant         modulemeta.ResolvedVariant
	mcpVariant      modulemeta.ResolvedVariant
	platformRelease fwapi.Release
	releases        []common.ComponentRelease
	kustomizeFS     filesys.FileSystem
}

// NewModule creates a Module with one-shot computed state.
func NewModule(cfg *moduleconfig.Config) (*Module, error) {
	kustomizeFS, err := newKustomizeFS()
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

	mcpVariant, err := modulemeta.LoadVariant(
		assets.Manifests,
		modulemeta.DescriptorPath,
		modulemeta.VariantMCPGuardrails,
	)
	if err != nil {
		return nil, fmt.Errorf("loading variant %q: %w", modulemeta.VariantMCPGuardrails, err)
	}

	return &Module{
		cfg:         cfg,
		variant:     variant,
		mcpVariant:  mcpVariant,
		kustomizeFS: kustomizeFS,
	}, nil
}

// Init applies image parameter substitutions once at process startup.
func (m *Module) Init() error {
	if err := modulemeta.ApplyStaticParams(m.kustomizeFS, m.variant.Kustomize); err != nil {
		return fmt.Errorf("applying static params for variant %q: %w", m.variant.Name, err)
	}
	if err := modulemeta.ApplyStaticParams(m.kustomizeFS, m.mcpVariant.Kustomize); err != nil {
		return fmt.Errorf("applying static params for variant %q: %w", m.mcpVariant.Name, err)
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
		if errors.Is(err, iofs.ErrNotExist) {
			return fwreleases.NormalizeComponentReleases([]common.ComponentRelease{m.cfg.ComponentRelease()}), nil
		}
		return nil, fmt.Errorf("failed to read component metadata: %w", err)
	}

	return fwreleases.NormalizeComponentReleases(append(releases, m.cfg.ComponentRelease())), nil
}
