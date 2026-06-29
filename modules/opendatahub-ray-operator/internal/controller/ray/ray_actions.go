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
	"path"
	"slices"

	componentApi "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-ray-operator/api/components/v1alpha1"
	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-ray-operator/assets"
	common "github.com/opendatahub-io/odh-platform-utilities/api/common"
	fwtypes "github.com/opendatahub-io/odh-platform-utilities/framework/controller/types"
	kparams "github.com/opendatahub-io/odh-platform-utilities/framework/render/kustomize/params"
)

const openShiftConfigGrantsTemplatePath = "manifests/ext/openshift-config-grants.yaml.tmpl"

// initialize appends manifests and applies image/namespace parameters.
func (m *Module) initialize(_ context.Context, rr *fwtypes.ReconciliationRequest) error {
	rr.Manifests = append(rr.Manifests, m.manifestInfo)
	rr.Templates = []fwtypes.TemplateInfo{{
		FS:   assets.Manifests,
		Path: openShiftConfigGrantsTemplatePath,
	}}

	if err := kparams.Apply(
		m.kustomizeFS,
		path.Join(m.manifestInfo.String(), "params.env"),
		kparams.Values(map[string]string{"namespace": m.cfg.ApplicationsNamespace}),
	); err != nil {
		return fmt.Errorf("failed to update params.env: %w", err)
	}

	return nil
}

// reportStatus refreshes status.releases from the cached static metadata.
func (m *Module) reportStatus(ctx context.Context, rr *fwtypes.ReconciliationRequest) error {
	obj, ok := rr.Instance.(*componentApi.Ray)
	if !ok {
		return fmt.Errorf("instance is not a Ray")
	}

	obj.SetReleaseStatus(common.ComponentReleaseStatus{
		Releases: slices.Clone(m.releases),
	})

	return nil
}
