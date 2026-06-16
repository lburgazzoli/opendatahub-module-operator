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

package datasciencepipelines

import (
	"context"
	"encoding/json"
	"fmt"
	"path"

	componentApi "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-datasciencepipelines-operator/api/components/v1alpha1"
	module "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-datasciencepipelines-operator/pkg/module"
	fwcluster "github.com/opendatahub-io/odh-platform-utilities/framework/cluster"
	odherr "github.com/opendatahub-io/odh-platform-utilities/framework/controller/actions/errors"
	"github.com/opendatahub-io/odh-platform-utilities/framework/controller/conditions"
	fwtypes "github.com/opendatahub-io/odh-platform-utilities/framework/controller/types"
	fwparams "github.com/opendatahub-io/odh-platform-utilities/framework/utils/params"
	operatorv1 "github.com/openshift/api/operator/v1"
	ofVersion "github.com/operator-framework/api/pkg/lib/version"
	k8serr "k8s.io/apimachinery/pkg/api/errors"
)

func (m *Module) initialize(_ context.Context, rr *fwtypes.ReconciliationRequest) error {
	rr.Manifests = append(rr.Manifests, m.manifestInfo)
	return nil
}

func (m *Module) checkPreConditions(ctx context.Context, rr *fwtypes.ReconciliationRequest) error {
	dsp, ok := rr.Instance.(*componentApi.DataSciencePipelines)
	if !ok {
		return fmt.Errorf("resource instance %T is not a DataSciencePipelines", rr.Instance)
	}

	awfSpec := dsp.Spec.ArgoWorkflowsControllers
	awfRemoved := awfSpec != nil && awfSpec.ManagementState == operatorv1.Removed

	rr.Conditions.MarkTrue(module.ConditionArgoWorkflowAvailable)

	crd, err := fwcluster.GetCRD(ctx, rr.Client, ArgoWorkflowCRD)
	switch {
	case k8serr.IsNotFound(err):
		if awfRemoved {
			rr.Conditions.MarkFalse(
				module.ConditionArgoWorkflowAvailable,
				conditions.WithObservedGeneration(rr.Instance.GetGeneration()),
				conditions.WithReason(module.DataSciencePipelinesArgoWorkflowsCRDMissingReason),
				conditions.WithMessage(module.DataSciencePipelinesArgoWorkflowsCRDMissingMessage),
			)

			return ErrArgoWorkflowCRDMissing
		}

		return nil
	case err != nil:
		stopErr := odherr.NewStopError("failed to check for existing %s CRD: %w", ArgoWorkflowCRD, err)
		rr.Conditions.MarkFalse(
			module.ConditionArgoWorkflowAvailable,
			conditions.WithObservedGeneration(rr.Instance.GetGeneration()),
			conditions.WithError(stopErr),
		)

		return stopErr
	}

	if awfRemoved {
		rr.Conditions.MarkTrue(
			module.ConditionArgoWorkflowAvailable,
			conditions.WithObservedGeneration(rr.Instance.GetGeneration()),
			conditions.WithReason(module.DataSciencePipelinesArgoWorkflowsNotManagedReason),
			conditions.WithMessage(module.DataSciencePipelinesArgoWorkflowsNotManagedMessage),
		)

		return nil
	}

	odhLabelValue, odhLabelExists := crd.Labels[appLabelPrefix+"/"+LegacyComponentName]
	if !odhLabelExists || odhLabelValue != "true" {
		rr.Conditions.MarkFalse(
			module.ConditionArgoWorkflowAvailable,
			conditions.WithObservedGeneration(rr.Instance.GetGeneration()),
			conditions.WithReason(module.DataSciencePipelinesDoesntOwnArgoCRDReason),
			conditions.WithMessage(module.DataSciencePipelinesDoesntOwnArgoCRDMessage),
		)

		return ErrArgoWorkflowAPINotOwned
	}

	return nil
}

func (m *Module) argoWorkflowsControllersOptions(_ context.Context, rr *fwtypes.ReconciliationRequest) error {
	dsp, ok := rr.Instance.(*componentApi.DataSciencePipelines)
	if !ok {
		return fmt.Errorf("resource instance %T is not a DataSciencePipelines", rr.Instance)
	}

	awfSpec := dsp.Spec.ArgoWorkflowsControllers
	if awfSpec == nil {
		awfSpec = &componentApi.ArgoWorkflowsControllersSpec{
			ManagementState: operatorv1.Managed,
		}
	}

	awfSpecJSON, err := json.Marshal(awfSpec)
	if err != nil {
		return fmt.Errorf("failed to marshal spec.argoWorkflowsControllers: %w", err)
	}

	pp := path.Join(m.cfg.ManifestsPath, componentName, "base")

	if err := fwparams.Apply(
		pp,
		"params.env",
		fwparams.Values(map[string]string{
			argoWorkflowsControllersParamsKey: string(awfSpecJSON),
		}),
	); err != nil {
		return fmt.Errorf("failed to update params.env on path %s:: %w", pp, err)
	}

	return nil
}

func (m *Module) reportStatus(_ context.Context, rr *fwtypes.ReconciliationRequest) error {
	obj, ok := rr.Instance.(*componentApi.DataSciencePipelines)
	if !ok {
		return fmt.Errorf("instance is not a DataSciencePipelines")
	}

	obj.Status.Release = componentApi.Release{
		Name:    componentApi.Platform(rr.Release.Name),
		Version: ofVersion.OperatorVersion{Version: rr.Release.Version},
	}

	return nil
}
