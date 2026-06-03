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
	"context"
	"fmt"

	"github.com/opendatahub-io/opendatahub-operator/v2/api/common"
	odhtypes "github.com/opendatahub-io/opendatahub-operator/v2/pkg/controller/types"
	odhdeploy "github.com/opendatahub-io/opendatahub-operator/v2/pkg/deploy"
	"sigs.k8s.io/controller-runtime/pkg/client"

	componentApi "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-trainer-operator/api/components/v1alpha1"
	moduleconfig "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-trainer-operator/pkg/config"
)

const (
	componentName = componentApi.TrainerComponentName
)

// Module holds process-lifetime state for the trainer controller.
type Module struct {
	cfg          *moduleconfig.Config
	manifestInfo odhtypes.ManifestInfo
	apiReader    client.Reader
}

// NewModule creates a Module with one-shot computed state.
func NewModule(cfg *moduleconfig.Config) (*Module, error) {
	mi := manifestPath(cfg.ManifestsPath)

	if err := odhdeploy.ApplyParams(mi.String(), "params.env", imageParamMap); err != nil {
		return nil, fmt.Errorf("failed to update images on path %s: %w", mi, err)
	}

	return &Module{
		cfg:          cfg,
		manifestInfo: mi,
	}, nil
}

// initialize appends manifests for the trainer component.
func (m *Module) initialize(_ context.Context, rr *odhtypes.ReconciliationRequest) error {
	rr.Manifests = append(rr.Manifests, m.manifestInfo)

	return nil
}

// reportStatus populates the release status and config values.
func (m *Module) reportStatus(_ context.Context, rr *odhtypes.ReconciliationRequest) error {
	obj, ok := rr.Instance.(*componentApi.Trainer)
	if !ok {
		return fmt.Errorf("instance is not a Trainer")
	}

	obj.Status.Release = common.Release{
		Name:    rr.Release.Name,
		Version: rr.Release.Version,
	}

	return nil
}
