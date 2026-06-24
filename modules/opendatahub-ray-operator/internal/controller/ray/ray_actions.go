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
	"context"
	"fmt"

	componentApi "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-ray-operator/api/components/v1alpha1"
	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-ray-operator/pkg/releases"
	fwtypes "github.com/opendatahub-io/odh-platform-utilities/framework/controller/types"
	fwparams "github.com/opendatahub-io/odh-platform-utilities/framework/utils/params"
)

// initialize appends manifests and applies image/namespace parameters.
func (m *Module) initialize(_ context.Context, rr *fwtypes.ReconciliationRequest) error {
	rr.Manifests = append(rr.Manifests, m.manifestInfo)

	if err := fwparams.Apply(
		m.manifestInfo.String(),
		"params.env",
		fwparams.Values(map[string]string{"namespace": m.cfg.ApplicationsNamespace}),
	); err != nil {
		return fmt.Errorf("failed to update params.env: %w", err)
	}

	return nil
}

// reportStatus writes the platform version handshake entry into status.releases.
func (m *Module) reportStatus(ctx context.Context, rr *fwtypes.ReconciliationRequest) error {
	obj, ok := rr.Instance.(*componentApi.Ray)
	if !ok {
		return fmt.Errorf("instance is not a Ray")
	}

	releases.Upsert(obj.GetReleaseStatus(), m.cfg.Release())

	return nil
}
