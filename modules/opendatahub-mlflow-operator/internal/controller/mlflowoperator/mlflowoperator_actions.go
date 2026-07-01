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

package mlflowoperator

import (
	"context"
	"fmt"
	"slices"
	"strings"

	k8serr "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"

	componentApi "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-mlflow-operator/api/components/v1alpha1"
	module "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-mlflow-operator/pkg/module"
	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-mlflow-operator/pkg/resources/gvk"
	odherrors "github.com/opendatahub-io/odh-platform-utilities/framework/controller/actions/errors"
	fwconditions "github.com/opendatahub-io/odh-platform-utilities/framework/controller/conditions"
	fwtypes "github.com/opendatahub-io/odh-platform-utilities/framework/controller/types"
)

// errGatewayDomainEmpty is returned when GatewayConfig exists but Status.Domain is not yet set.
var errGatewayDomainEmpty = fmt.Errorf("GatewayConfig.Status.Domain is empty")

// getGatewayDomain reads the gateway domain from the GatewayConfig singleton CR using
// an unstructured client so no OpenShift service API import is needed.
func getGatewayDomain(ctx context.Context, reader client.Reader) (string, error) {
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(gvk.GatewayConfig)

	if err := reader.Get(ctx, client.ObjectKey{Name: "default-gateway"}, obj); err != nil {
		return "", err
	}

	domain, found, err := unstructured.NestedString(obj.Object, "status", "domain")
	if err != nil {
		return "", fmt.Errorf("reading GatewayConfig.status.domain: %w", err)
	}
	if !found || domain == "" {
		return "", errGatewayDomainEmpty
	}

	return domain, nil
}

// stageManifests appends the pre-resolved manifest info to the pipeline.
func (m *Module) stageManifests(_ context.Context, rr *fwtypes.ReconciliationRequest) error {
	rr.Manifests = make([]fwtypes.ManifestInfo, 0, len(m.variant.Kustomize))
	for _, item := range m.variant.Kustomize {
		if item.SkipRender {
			continue
		}
		rr.Manifests = append(rr.Manifests, item.ManifestInfo)
	}
	rr.Templates = m.variant.Templates
	rr.HelmCharts = m.variant.HelmCharts
	return nil
}

// customizeManifests writes runtime-computed variables (mlflow-url, section-title)
// to base/params.env before kustomize renders the manifests. The gateway domain is
// read live from GatewayConfig.Status.Domain and is treated as a render-time
// dependency because missing values would otherwise produce a broken ConsoleLink
// while the workload can still report Ready.
func (m *Module) customizeManifests(ctx context.Context, rr *fwtypes.ReconciliationRequest) error {
	consoleLinkDomain, err := getGatewayDomain(ctx, rr.Client)
	if err != nil {
		switch {
		case meta.IsNoMatchError(err):
			return m.markDependenciesUnavailable(
				rr,
				"GatewayConfig CRD is not installed: gateway routing must be available before enabling MLflow",
			)
		case k8serr.IsNotFound(err):
			return m.markDependenciesUnavailable(
				rr,
				"GatewayConfig default-gateway was not found: gateway routing must be configured before enabling MLflow",
			)
		case err == errGatewayDomainEmpty:
			return m.markDependenciesUnavailable(
				rr,
				"GatewayConfig default-gateway does not yet report status.domain: gateway routing must be ready before enabling MLflow",
			)
		default:
			return m.markDependenciesUnknown(
				rr,
				fmt.Errorf("getting gateway domain: %w", err),
			)
		}
	}

	rr.Conditions.MarkTrue(
		module.ConditionDependenciesAvailable,
		fwconditions.WithObservedGeneration(rr.Instance.GetGeneration()),
	)

	extraParams := map[string]string{
		"mlflow-url":    fmt.Sprintf("https://%s/", consoleLinkDomain),
		"section-title": m.consoleSectionTitle,
	}

	if err := module.ApplyRuntimeParams(m.kustomizeFS, m.variant.Kustomize, extraParams); err != nil {
		return fmt.Errorf("applying runtime params for variant %q: %w", m.variant.Name, err)
	}

	return nil
}

func (m *Module) markDependenciesUnavailable(
	rr *fwtypes.ReconciliationRequest,
	message string,
) error {
	stopErr := odherrors.NewStopError("%s", message)
	rr.Conditions.MarkFalse(
		module.ConditionDependenciesAvailable,
		fwconditions.WithObservedGeneration(rr.Instance.GetGeneration()),
		fwconditions.WithReason(module.PreConditionFailedReason),
		fwconditions.WithMessage("%s", message),
	)
	return stopErr
}

func (m *Module) markDependenciesUnknown(
	rr *fwtypes.ReconciliationRequest,
	err error,
) error {
	stopErr := odherrors.NewStopError("%w", err)
	rr.Conditions.MarkUnknown(
		module.ConditionDependenciesAvailable,
		fwconditions.WithObservedGeneration(rr.Instance.GetGeneration()),
		fwconditions.WithReason(module.PreConditionFailedReason),
		fwconditions.WithMessage("%s", stopErr.Error()),
	)
	return stopErr
}

// fixDeploymentNamespace amends the rendered mlflow-operator Deployment in rr.Resources
// to replace the hardcoded --namespace arg with the configured ApplicationsNamespace.
//
// TODO(module-team): use the namespace in which the oeprator runs.
func (m *Module) fixDeploymentNamespace(_ context.Context, rr *fwtypes.ReconciliationRequest) error {
	target := "--namespace=" + m.cfg.ApplicationsNamespace

	for i := range rr.Resources {
		res := rr.Resources[i]
		if res.GetKind() != "Deployment" {
			continue
		}

		containers, found, err := unstructured.NestedSlice(res.Object, "spec", "template", "spec", "containers")
		if err != nil || !found {
			continue
		}

		changed := false
		for ci, c := range containers {
			container, ok := c.(map[string]any)
			if !ok {
				continue
			}
			rawArgs, ok := container["args"].([]any)
			if !ok {
				continue
			}
			for ai, a := range rawArgs {
				if s, ok := a.(string); ok && strings.HasPrefix(s, "--namespace=") && s != target {
					rawArgs[ai] = target
					changed = true
				}
			}
			containers[ci] = container
		}

		if changed {
			if err := unstructured.SetNestedSlice(res.Object, containers, "spec", "template", "spec", "containers"); err != nil {
				return fmt.Errorf("patching --namespace in Deployment %s: %w", res.GetName(), err)
			}
		}
	}

	return nil
}

// reportStatus refreshes status.releases from the cached static metadata.
func (m *Module) reportStatus(_ context.Context, rr *fwtypes.ReconciliationRequest) error {
	obj, ok := rr.Instance.(*componentApi.MLflowOperator)
	if !ok {
		return fmt.Errorf("instance is not a MLflowOperator")
	}

	releaseStatus := obj.GetReleaseStatus()
	releaseStatus.Releases = slices.Clone(m.releases)

	return nil
}
