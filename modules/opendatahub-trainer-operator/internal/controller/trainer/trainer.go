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

package trainer

import (
	"fmt"

	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-trainer-operator/assets"
	common "github.com/opendatahub-io/odh-platform-utilities/api/common"
	fwapi "github.com/opendatahub-io/odh-platform-utilities/framework/api"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/kustomize/kyaml/filesys"

	componentApi "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-trainer-operator/api/components/v1alpha1"
	moduleconfig "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-trainer-operator/pkg/config"
	modulemeta "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-trainer-operator/pkg/module"
)

const (
	componentName = componentApi.TrainerComponentName
)

// Module holds process-lifetime state for the trainer controller.
type Module struct {
	cfg             *moduleconfig.Config
	variant         modulemeta.ResolvedVariant
	platformRelease fwapi.Release
	releases        []common.ComponentRelease
	apiReader       client.Reader
	kustomizeFS     filesys.FileSystem
}

// NewModule creates a Module with one-shot computed state.
// Trainer currently ships only the RHOAI overlay, so platformType is reported
// in status/config but does not switch the rendered source path.
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

// Init applies image parameters from environment — called once at startup.
func (m *Module) Init() error {
	if err := modulemeta.ApplyStaticParams(m.kustomizeFS, m.variant.Kustomize); err != nil {
		return err
	}

	releases, err := m.loadReleases()
	if err != nil {
		return fmt.Errorf("failed to load releases: %w", err)
	}

	m.platformRelease = m.cfg.PlatformRelease()
	m.releases = releases

	return nil
}
