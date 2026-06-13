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

	componentApi "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-datasciencepipelines-operator/api/components/v1alpha1"
	module "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-datasciencepipelines-operator/pkg/module"
	fwcluster "github.com/opendatahub-io/odh-platform-utilities/framework/cluster"
	odherr "github.com/opendatahub-io/odh-platform-utilities/framework/controller/actions/errors"
	"github.com/opendatahub-io/odh-platform-utilities/framework/controller/conditions"
	fwtypes "github.com/opendatahub-io/odh-platform-utilities/framework/controller/types"
	fwparams "github.com/opendatahub-io/odh-platform-utilities/framework/utils/params"
	operatorv1 "github.com/openshift/api/operator/v1"
	k8serr "k8s.io/apimachinery/pkg/api/errors"
)

func checkPreConditions(ctx context.Context, rr *fwtypes.ReconciliationRequest) error {
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

func argoWorkflowsControllersOptions(_ context.Context, rr *fwtypes.ReconciliationRequest) error {
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

	extraParams := map[string]string{
		argoWorkflowsControllersParamsKey: string(awfSpecJSON),
	}

	if err := fwparams.Apply(
		paramsPath(rr.ManifestsBasePath),
		"params.env",
		fwparams.Values(extraParams),
	); err != nil {
		return fmt.Errorf("failed to update params.env: %w", err)
	}

	return nil
}
