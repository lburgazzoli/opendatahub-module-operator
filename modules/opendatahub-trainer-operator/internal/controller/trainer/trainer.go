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
	"path"

	fwapi "github.com/opendatahub-io/odh-platform-utilities/framework/api"
	odhtypes "github.com/opendatahub-io/odh-platform-utilities/framework/controller/types"
	kfs "github.com/opendatahub-io/odh-platform-utilities/framework/render/kustomize/fs"
	kparams "github.com/opendatahub-io/odh-platform-utilities/framework/render/kustomize/params"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/kustomize/kyaml/filesys"

	componentApi "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-trainer-operator/api/components/v1alpha1"
	moduleconfig "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-trainer-operator/pkg/config"
)

const (
	componentName = componentApi.TrainerComponentName

	// trainer only ships a rhoai overlay; no separate odh overlay exists.
	overlayRhoai = "rhoai"
)

// Module holds process-lifetime state for the trainer controller.
type Module struct {
	cfg          *moduleconfig.Config
	release      fwapi.Release
	manifestInfo odhtypes.ManifestInfo
	apiReader    client.Reader
	renderFS     filesys.FileSystem
}

// NewModule creates a Module with one-shot computed state.
func NewModule(cfg *moduleconfig.Config) (*Module, error) {
	baseFS, err := kfs.NewBasePathFs(kfs.NewReadOnlyFs(kfs.NewFsOnDisk()), cfg.ManifestsPath)
	if err != nil {
		return nil, fmt.Errorf("creating base render filesystem: %w", err)
	}

	renderFS, err := kfs.NewUnionFs(baseFS)
	if err != nil {
		return nil, fmt.Errorf("creating render filesystem: %w", err)
	}

	return &Module{
		cfg: cfg,
		manifestInfo: odhtypes.ManifestInfo{
			Path:       ".",
			ContextDir: componentName,
			SourcePath: overlayRhoai,
		},
		renderFS: renderFS,
	}, nil
}

// Init applies image parameters from environment — called once at startup.
func (m *Module) Init() error {
	if err := kparams.Apply(
		m.renderFS,
		path.Join(m.manifestInfo.String(), "params.env"),
		kparams.Replacement(kparams.FromEnv(imageParamMap)),
	); err != nil {
		return fmt.Errorf("failed to update images on path %s: %w", m.manifestInfo, err)
	}

	m.release = m.cfg.PlatformRelease()

	return nil
}
