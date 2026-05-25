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
	"path"
	"strings"

	k8serr "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-mlflow-operator/pkg/resources/gvk"
	odhtypes "github.com/opendatahub-io/opendatahub-operator/v2/pkg/controller/types"
	odhdeploy "github.com/opendatahub-io/opendatahub-operator/v2/pkg/deploy"
)

// errGatewayDomainEmpty is returned when GatewayConfig exists but Status.Domain is not yet set.
var errGatewayDomainEmpty = fmt.Errorf("GatewayConfig.Status.Domain is empty")

// getGatewayDomain reads the gateway domain from the GatewayConfig singleton CR using
// an unstructured client so no OpenShift service API import is needed.
func getGatewayDomain(ctx context.Context, cli client.Client) (string, error) {
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(gvk.GatewayConfig)

	if err := cli.Get(ctx, client.ObjectKey{Name: "default-gateway"}, obj); err != nil {
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

// fixDeploymentNamespace amends the rendered mlflow-operator Deployment in rr.Resources
// to replace the hardcoded --namespace arg with the configured ApplicationsNamespace.
//
// TODO(module-team): use the namespace in which the oeprator runs.
func (m *Module) fixDeploymentNamespace(_ context.Context, rr *odhtypes.ReconciliationRequest) error {
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

// setKustomizedParams writes runtime-computed variables (mlflow-url, section-title)
// to base/params.env before kustomize renders the manifests. The gateway domain is
// read live from GatewayConfig.Status.Domain.
//
// TODO(module-team): validate error-handling behavior for your deployment. Currently,
// if GatewayConfig is absent or domain is not yet set, the params are skipped and
// mlflow-url / section-title will not be rendered. The ConsoleLink will be broken
// until GatewayConfig is available.
func (m *Module) setKustomizedParams(ctx context.Context, rr *odhtypes.ReconciliationRequest) error {
	consoleLinkDomain, err := getGatewayDomain(ctx, rr.Client)
	if err != nil {
		switch {
		case meta.IsNoMatchError(err):
			// GatewayConfig CRD not installed — gateway service not deployed.
			// TODO(module-team): decide whether to requeue instead of silently skipping.
			return nil
		case k8serr.IsNotFound(err):
			// GatewayConfig CR does not exist yet — may appear later.
			// TODO(module-team): decide whether to requeue instead of silently skipping.
			return nil
		case err == errGatewayDomainEmpty:
			// GatewayConfig exists but Status.Domain is not populated yet.
			// TODO(module-team): decide whether to requeue instead of silently skipping.
			return nil
		default:
			return fmt.Errorf("error getting gateway domain: %w", err)
		}
	}

	extraParams := map[string]string{
		"mlflow-url":    fmt.Sprintf("https://%s/", consoleLinkDomain),
		"section-title": m.consoleSectionTitle,
	}

	// The monolith writes params to base/, not to the overlay path.
	paramsPath := path.Join(m.cfg.ManifestsPath, componentName, paramsSubDir)

	if err := odhdeploy.ApplyParams(paramsPath, "params.env", nil, extraParams); err != nil {
		return fmt.Errorf("failed to update params.env from %s: %w", paramsPath, err)
	}

	return nil
}
