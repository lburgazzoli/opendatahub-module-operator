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

package ray

import (
	"fmt"

	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-ray-operator/assets"
	common "github.com/opendatahub-io/odh-platform-utilities/api/common"
	fwapi "github.com/opendatahub-io/odh-platform-utilities/framework/api"
	"sigs.k8s.io/kustomize/kyaml/filesys"

	componentApi "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-ray-operator/api/components/v1alpha1"
	moduleconfig "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-ray-operator/pkg/config"
	modulemeta "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-ray-operator/pkg/module"
)

const (
	componentName = componentApi.RayComponentName

	// LegacyComponentName is the label value assigned to deployments via
	// Kustomize. Deployment selectors are immutable so this must match
	// whatever the upstream manifests use.
	LegacyComponentName = "ray"
)

// Module holds process-lifetime state for the ray controller.
type Module struct {
	cfg             *moduleconfig.Config
	variant         modulemeta.ResolvedVariant
	platformRelease fwapi.Release
	releases        []common.ComponentRelease
	kustomizeFS     filesys.FileSystem
}

// NewModule creates a Module with one-shot computed state.
// Ray currently ships only the OpenShift manifest layout, so platformType is
// reported in status/config but does not switch the rendered source path.
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
		cfg:         cfg,
		variant:     variant,
		kustomizeFS: kustomizeFS,
	}, nil
}

// Init applies image parameters from environment and parses the platform
// release version — called once at startup.
func (m *Module) Init() error {
	if err := modulemeta.ApplyStaticParams(m.kustomizeFS, m.variant.Kustomize); err != nil {
		return err
	}
	if err := modulemeta.ApplyRuntimeParams(m.kustomizeFS, m.variant.Kustomize, map[string]string{
		"namespace": m.cfg.ApplicationsNamespace,
	}); err != nil {
		return fmt.Errorf("applying init params for variant %q: %w", m.variant.Name, err)
	}

	releases, err := m.loadReleases()
	if err != nil {
		return fmt.Errorf("failed to load releases: %w", err)
	}

	m.platformRelease = m.cfg.PlatformRelease()
	m.releases = releases

	return nil
}
