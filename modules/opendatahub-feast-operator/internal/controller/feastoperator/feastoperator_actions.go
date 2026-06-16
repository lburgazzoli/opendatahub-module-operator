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

package feastoperator

import (
	"context"
	"errors"
	"fmt"
	"net/url"

	appsv1 "k8s.io/api/apps/v1"
	k8serr "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	componentApi "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-feast-operator/api/components/v1alpha1"
	odhtypes "github.com/opendatahub-io/odh-platform-utilities/framework/controller/types"
	fwparams "github.com/opendatahub-io/odh-platform-utilities/framework/utils/params"
	ofVersion "github.com/operator-framework/api/pkg/lib/version"
)

const (
	// ParamsEnvKeyOIDCIssuerURL is the params.env key for the external OIDC issuer URL.
	ParamsEnvKeyOIDCIssuerURL = "OIDC_ISSUER_URL"

	// deploymentName is the name of the feast operator Deployment subject to selector migration.
	deploymentName     = "feast-operator-controller-manager"
	selectorLabelKey   = "app.kubernetes.io/name"
	selectorLabelValue = "feast-operator"
)

// initialize appends the pre-resolved manifest info to the pipeline.
// Feast does not require namespace substitution in params.env.
func (m *Module) initialize(_ context.Context, rr *odhtypes.ReconciliationRequest) error {
	rr.Manifests = append(rr.Manifests, m.manifestInfo)
	return nil
}

// customizeManifests merges runtime values from the FeastOperator CR into params.env
// before kustomize renders the manifests. Writes OIDC_ISSUER_URL (empty when OIDC not in use).
func (m *Module) customizeManifests(_ context.Context, rr *odhtypes.ReconciliationRequest) error {
	feast, ok := rr.Instance.(*componentApi.FeastOperator)
	if !ok {
		return errors.New("instance is not a FeastOperator")
	}

	if len(rr.Manifests) == 0 {
		return errors.New("no manifests initialized before customizeManifests")
	}

	issuerURL := ""
	if feast.Spec.OIDC != nil {
		var err error
		issuerURL, err = parseAndValidateOIDCIssuerURL(feast.Spec.OIDC.IssuerURL)
		if err != nil {
			return fmt.Errorf("invalid OIDC issuer URL %q in FeastOperator spec.oidc: %w", feast.Spec.OIDC.IssuerURL, err)
		}
	}

	extraParams := map[string]string{
		ParamsEnvKeyOIDCIssuerURL: issuerURL,
	}

	if err := fwparams.Apply(
		rr.Manifests[0].String(),
		"params.env",
		fwparams.Values(extraParams),
	); err != nil {
		return fmt.Errorf("failed to update params.env with kustomize parameters: %w", err)
	}

	return nil
}

// migrateDeploymentSelector deletes the feast-operator-controller-manager Deployment if its
// spec.selector.matchLabels is missing the app.kubernetes.io/name label. This handles upgrades
// where the selector changed between releases — since spec.selector is immutable, the only way
// to update it is to delete and let the operator recreate it.
func (m *Module) migrateDeploymentSelector(ctx context.Context, rr *odhtypes.ReconciliationRequest) error {
	log := logf.FromContext(ctx)

	deploy := &appsv1.Deployment{}
	err := rr.Client.Get(ctx, client.ObjectKey{
		Name:      deploymentName,
		Namespace: m.cfg.ApplicationsNamespace,
	}, deploy)
	if err != nil {
		if k8serr.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("failed to get Deployment %s/%s: %w", m.cfg.ApplicationsNamespace, deploymentName, err)
	}

	if deploy.Spec.Selector == nil {
		return nil
	}

	if deploy.Spec.Selector.MatchLabels[selectorLabelKey] == selectorLabelValue {
		return nil
	}

	log.Info("Feast operator Deployment has stale selector, deleting for recreation",
		"deployment", deploymentName,
		"namespace", m.cfg.ApplicationsNamespace,
		"currentSelector", deploy.Spec.Selector.MatchLabels,
	)

	if err := rr.Client.Delete(ctx, deploy); err != nil {
		if k8serr.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("failed to delete Deployment %s/%s with stale selector: %w", m.cfg.ApplicationsNamespace, deploymentName, err)
	}

	log.Info("Deleted Feast operator Deployment, it will be recreated with the correct selector",
		"deployment", deploymentName, "namespace", m.cfg.ApplicationsNamespace)

	return nil
}

// reportStatus populates the release status and config values.
func (m *Module) reportStatus(_ context.Context, rr *odhtypes.ReconciliationRequest) error {
	obj, ok := rr.Instance.(*componentApi.FeastOperator)
	if !ok {
		return fmt.Errorf("instance is not a FeastOperator")
	}

	obj.Status.Release = componentApi.Release{
		Name:    componentApi.Platform(rr.Release.Name),
		Version: ofVersion.OperatorVersion{Version: rr.Release.Version},
	}

	return nil
}

// parseAndValidateOIDCIssuerURL validates an OIDC issuer URL: must be an absolute HTTPS URL
// with a host. Returns the normalized form.
func parseAndValidateOIDCIssuerURL(raw string) (string, error) {
	parsed, err := url.ParseRequestURI(raw)
	if err != nil {
		return "", fmt.Errorf("parse OIDC issuer URL: %w", err)
	}
	if parsed.Scheme != "https" {
		return "", errors.New("OIDC issuer URL must use https scheme")
	}
	if parsed.Host == "" {
		return "", errors.New("OIDC issuer URL must include a host")
	}
	return parsed.String(), nil
}
