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

package mlflowoperator

import (
	"errors"
	"fmt"
	iofs "io/fs"
	"path"

	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-mlflow-operator/assets"
	common "github.com/opendatahub-io/odh-platform-utilities/api/common"
	fwapi "github.com/opendatahub-io/odh-platform-utilities/framework/api"
	fwreleases "github.com/opendatahub-io/odh-platform-utilities/framework/controller/actions/status/releases"
	"sigs.k8s.io/kustomize/kyaml/filesys"

	componentApi "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-mlflow-operator/api/components/v1alpha1"
	moduleconfig "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-mlflow-operator/pkg/config"
	modulemeta "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-mlflow-operator/pkg/module"
)

const (
	componentName         = componentApi.MLflowOperatorComponentName
	manifestsRoot         = "manifests"
	componentMetadataFile = "component_metadata.yaml"
)

// Module holds process-lifetime state for the mlflowoperator controller.
type Module struct {
	cfg             *moduleconfig.Config
	platformRelease fwapi.Release
	releases        []common.ComponentRelease
	variant         modulemeta.ResolvedVariant
	kustomizeFS     filesys.FileSystem
	// consoleSectionTitle is the section-title kustomize variable, computed once from platform.
	consoleSectionTitle string
}

func consoleSectionTitleFor(platformType string) string {
	if platformType == moduleconfig.PlatformTypeSelfManagedRhoai || platformType == moduleconfig.PlatformTypeManagedRhoai {
		return "OpenShift Self Managed Services"
	}
	return "OpenShift Open Data Hub"
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

	return &Module{
		cfg:                 cfg,
		variant:             variant,
		kustomizeFS:         kustomizeFS,
		consoleSectionTitle: consoleSectionTitleFor(cfg.PlatformType),
	}, nil
}

// Init applies image parameters to base/ from environment — called once at startup.
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

func (m *Module) loadReleases() ([]common.ComponentRelease, error) {
	metadataPath := path.Join(manifestsRoot, componentName, componentMetadataFile)
	releases, err := fwreleases.ReadComponentReleases(assets.Manifests, metadataPath)
	if err != nil {
		if errors.Is(err, iofs.ErrNotExist) {
			return fwreleases.NormalizeComponentReleases([]common.ComponentRelease{m.cfg.ComponentRelease()}), nil
		}
		return nil, fmt.Errorf("failed to read component metadata from %s: %w", metadataPath, err)
	}

	return fwreleases.NormalizeComponentReleases(append(releases, m.cfg.ComponentRelease())), nil
}
