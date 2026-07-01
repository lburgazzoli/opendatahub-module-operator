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

package ogx

import (
	"context"
	"fmt"
	"slices"

	componentApi "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-ogx-operator/api/components/v1alpha1"
	common "github.com/opendatahub-io/odh-platform-utilities/api/common"
	odhtypes "github.com/opendatahub-io/odh-platform-utilities/framework/controller/types"
)

// stageManifests appends the pre-resolved manifest info to the pipeline.
func (m *Module) stageManifests(_ context.Context, rr *odhtypes.ReconciliationRequest) error {
	rr.Manifests = make([]odhtypes.ManifestInfo, 0, len(m.variant.Kustomize))
	for _, item := range m.variant.Kustomize {
		rr.Manifests = append(rr.Manifests, item.ManifestInfo)
	}
	rr.Templates = m.variant.Templates
	rr.HelmCharts = m.variant.HelmCharts
	return nil
}

// reportStatus refreshes status.releases from the cached static metadata.
func (m *Module) reportStatus(_ context.Context, rr *odhtypes.ReconciliationRequest) error {
	obj, ok := rr.Instance.(*componentApi.OGX)
	if !ok {
		return fmt.Errorf("instance is not an OGX")
	}

	obj.SetReleaseStatus(common.ComponentReleaseStatus{
		Releases: slices.Clone(m.releases),
	})

	return nil
}
