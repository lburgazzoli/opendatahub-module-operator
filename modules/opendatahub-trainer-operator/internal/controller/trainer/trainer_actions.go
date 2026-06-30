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
	"slices"

	ofVersion "github.com/operator-framework/api/pkg/lib/version"

	componentApi "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-trainer-operator/api/components/v1alpha1"
	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-trainer-operator/assets"
	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-trainer-operator/pkg/module"
	common "github.com/opendatahub-io/odh-platform-utilities/api/common"
	odherrors "github.com/opendatahub-io/odh-platform-utilities/framework/controller/actions/errors"
	fwconditions "github.com/opendatahub-io/odh-platform-utilities/framework/controller/conditions"
	odhtypes "github.com/opendatahub-io/odh-platform-utilities/framework/controller/types"
)

const openShiftConfigGrantsTemplatePath = "manifests/ext/openshift-config-grants.yaml.tmpl"

// ensureDependenciesAvailable halts reconcile until JobSet dependencies exist.
func (m *Module) ensureDependenciesAvailable(ctx context.Context, rr *odhtypes.ReconciliationRequest) error {
	checks := []func(context.Context, *odhtypes.ReconciliationRequest) (preconditionResult, error){
		m.checkPreConditions,
		m.checkJobSetCRD,
	}

	for _, check := range checks {
		result, err := check(ctx, rr)
		if err != nil {
			stopErr := odherrors.NewStopError("%w", err)
			rr.Conditions.MarkUnknown(
				module.ConditionDependenciesAvailable,
				fwconditions.WithObservedGeneration(rr.Instance.GetGeneration()),
				fwconditions.WithReason(module.PreConditionFailedReason),
				fwconditions.WithMessage("%s", stopErr.Error()),
			)

			return stopErr
		}

		if !result.Pass {
			stopErr := odherrors.NewStopError("%s", result.Message)
			rr.Conditions.MarkFalse(
				module.ConditionDependenciesAvailable,
				fwconditions.WithObservedGeneration(rr.Instance.GetGeneration()),
				fwconditions.WithReason(module.PreConditionFailedReason),
				fwconditions.WithMessage("%s", result.Message),
			)

			return stopErr
		}
	}

	rr.Conditions.MarkTrue(
		module.ConditionDependenciesAvailable,
		fwconditions.WithObservedGeneration(rr.Instance.GetGeneration()),
	)

	return nil
}

// stageManifests appends manifests for the trainer component.
func (m *Module) stageManifests(_ context.Context, rr *odhtypes.ReconciliationRequest) error {
	rr.Manifests = append(rr.Manifests, m.manifestInfo)
	rr.Templates = []odhtypes.TemplateInfo{{
		FS:   assets.Manifests,
		Path: openShiftConfigGrantsTemplatePath,
	}}

	return nil
}

// reportStatus refreshes status.releases from the cached static metadata and
// also maintains the legacy Release field for backward compatibility.
func (m *Module) reportStatus(_ context.Context, rr *odhtypes.ReconciliationRequest) error {
	obj, ok := rr.Instance.(*componentApi.Trainer)
	if !ok {
		return fmt.Errorf("instance is not a Trainer")
	}

	obj.SetReleaseStatus(common.ComponentReleaseStatus{
		Releases: slices.Clone(m.releases),
	})

	obj.Status.Release = componentApi.Release{
		Name:    componentApi.Platform(rr.Release.Name),
		Version: ofVersion.OperatorVersion{Version: rr.Release.Version},
	}

	return nil
}
