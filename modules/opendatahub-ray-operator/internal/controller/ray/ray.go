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
	"path"

	common "github.com/opendatahub-io/odh-platform-utilities/api/common"
	fwapi "github.com/opendatahub-io/odh-platform-utilities/framework/api"
	fwtypes "github.com/opendatahub-io/odh-platform-utilities/framework/controller/types"
	kparams "github.com/opendatahub-io/odh-platform-utilities/framework/render/kustomize/params"
	"sigs.k8s.io/kustomize/kyaml/filesys"

	componentApi "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-ray-operator/api/components/v1alpha1"
	moduleconfig "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-ray-operator/pkg/config"
)

const (
	componentName = componentApi.RayComponentName

	// LegacyComponentName is the label value assigned to deployments via
	// Kustomize. Deployment selectors are immutable so this must match
	// whatever the upstream manifests use.
	LegacyComponentName = "ray"

	overlayOpenShift = "openshift"
)

var imageParamMap = map[string]string{
	"odh-kuberay-operator-controller-image": "RELATED_IMAGE_ODH_KUBERAY_OPERATOR_CONTROLLER_IMAGE",
}

// Module holds process-lifetime state for the ray controller.
type Module struct {
	cfg             *moduleconfig.Config
	platformRelease fwapi.Release
	releases        []common.ComponentRelease
	manifestInfo    fwtypes.ManifestInfo
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

	return &Module{
		cfg: cfg,
		manifestInfo: fwtypes.ManifestInfo{
			Path:       "manifests",
			ContextDir: componentName,
			SourcePath: overlayOpenShift,
		},
		kustomizeFS: kustomizeFS,
	}, nil
}

// Init applies image parameters from environment and parses the platform
// release version — called once at startup.
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

	m.platformRelease = m.cfg.PlatformRelease()
	m.releases = releases

	return nil
}
