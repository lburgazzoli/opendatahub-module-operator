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

package mymodule

import (
	"context"
	"fmt"

	networkingv1 "k8s.io/api/networking/v1"
	k8serr "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/opendatahub-io/opendatahub-operator/v2/api/common"
	"github.com/opendatahub-io/opendatahub-operator/v2/pkg/cluster"
	"github.com/opendatahub-io/opendatahub-operator/v2/pkg/controller/precondition"
	odhtypes "github.com/opendatahub-io/opendatahub-operator/v2/pkg/controller/types"

	componentApi "github.com/lburgazzoli/opendatahub-module-operator/api/components/v1alpha1"
	moduleconfig "github.com/lburgazzoli/opendatahub-module-operator/pkg/config"
	"github.com/lburgazzoli/opendatahub-module-operator/pkg/version"
)

const (
	componentName = componentApi.MyModuleComponentName

	overlayRhoai = "overlays/rhoai"
	overlayODH   = "overlays/odh"

	// IngressName is the name of the Ingress that must exist in the
	// application namespace before the module can reconcile. This
	// demonstrates a precondition that validates an external dependency.
	IngressName = "mymodule"

	// ConditionIngressAvailable is the condition type set by the ingress
	// precondition. True when the required Ingress exists, False otherwise.
	ConditionIngressAvailable = "IngressAvailable"
)

// Module holds process-lifetime state for the mymodule controller.
// It is created once at registration time via NewModule and its methods
// are registered as actions.Fn in the reconciliation pipeline.
type Module struct {
	cfg             *moduleconfig.Config
	version         componentApi.SemVer
	platformVersion componentApi.SemVer
	manifestInfo    odhtypes.ManifestInfo

	// Webhook fields — set by RegisterWebhooks.
	decoder   admission.Decoder
	apiReader client.Reader
}

// NewModule creates a Module with one-shot computed state. Called once
// from NewReconciler at module registration; no sync.Once needed.
func NewModule(cfg *moduleconfig.Config) (*Module, error) {
	v, err := componentApi.NewSemVer(version.Version)
	if err != nil {
		return nil, fmt.Errorf("parsing module version %q: %w", version.Version, err)
	}

	// Platform version may be "unknown" or unset; default to zero.
	pv, _ := componentApi.NewSemVer(cfg.PlatformVersion)

	mi := odhtypes.ManifestInfo{
		ContextDir: componentName,
		SourcePath: overlayODH,
	}

	if common.Platform(cfg.PlatformType) == cluster.SelfManagedRhoai {
		mi.SourcePath = overlayRhoai
	}

	return &Module{
		cfg:             cfg,
		version:         v,
		platformVersion: pv,
		manifestInfo:    mi,
	}, nil
}

// initialize sets the manifest base path from the reconciliation request
// and appends the pre-resolved manifest info to the pipeline.
func (m *Module) initialize(_ context.Context, rr *odhtypes.ReconciliationRequest) error {
	info := m.manifestInfo
	info.Path = rr.ManifestsBasePath

	rr.Manifests = append(rr.Manifests, info)

	return nil
}

// checkIngress is a precondition check that verifies the required Ingress
// exists in the application namespace. Used with precondition.NewPreCondition
// and WithStopReconciliation so the pipeline halts when missing.
func (m *Module) checkIngress(ctx context.Context, rr *odhtypes.ReconciliationRequest) (precondition.CheckResult, error) {
	ingress := &networkingv1.Ingress{}
	key := client.ObjectKey{
		Namespace: m.cfg.ApplicationsNamespace,
		Name:      IngressName,
	}

	switch err := rr.Client.Get(ctx, key, ingress); {
	case k8serr.IsNotFound(err):
		return precondition.CheckResult{
			Pass: false,
			Message: fmt.Sprintf(
				"Ingress %q not found in namespace %q",
				IngressName,
				m.cfg.ApplicationsNamespace,
			),
		}, nil
	case err != nil:
		return precondition.CheckResult{}, fmt.Errorf("checking ingress: %w", err)
	default:
		return precondition.CheckResult{Pass: true}, nil
	}
}

// upgradeIfNeeded checks whether the module version advanced or the
// platform version changed since the last reconcile. If so, it calls
// m.upgrade to run idempotent migrations.
func (m *Module) upgradeIfNeeded(_ context.Context, rr *odhtypes.ReconciliationRequest) error {
	obj, ok := rr.Instance.(*componentApi.MyModule)
	if !ok {
		return fmt.Errorf("instance is not a MyModule")
	}

	prev := obj.Status.Module

	moduleVersionChanged := !prev.Version.IsZero() && m.version.GT(prev.Version)
	platformVersionChanged := !prev.Platform.Version.IsZero() && m.platformVersion.GT(prev.Platform.Version)

	if !moduleVersionChanged && !platformVersionChanged {
		return nil
	}

	return m.upgrade(prev, rr)
}

// upgrade runs idempotent migrations when the module version advances
// or the platform version changes. prev is the module status from the
// previous reconcile cycle.
func (m *Module) upgrade(prev componentApi.ModuleStatus, rr *odhtypes.ReconciliationRequest) error {
	_ = prev
	_ = rr

	// Idempotent migrations go here, gated by version ranges or
	// platform version transitions.
	// Examples:
	//   if prev.Version.GT("1.0.0") { ... }
	//   if m.platformVersion.GT(prev.Platform.Version) { ... }

	return nil
}

// reportStatus populates the module status with version, platform,
// source information, and config values.
func (m *Module) reportStatus(_ context.Context, rr *odhtypes.ReconciliationRequest) error {
	obj, ok := rr.Instance.(*componentApi.MyModule)
	if !ok {
		return fmt.Errorf("instance is not a MyModule")
	}

	obj.Status.Module = componentApi.ModuleStatus{
		Version:     m.version,
		BuildSource: version.Repo + "@" + version.Branch + "/" + version.Commit,
		Platform: componentApi.PlatformStatus{
			Name:    m.cfg.PlatformType,
			Version: m.platformVersion,
		},
	}

	var sources []componentApi.SourceStatus

	for _, manifest := range rr.Manifests {
		sources = append(sources, componentApi.SourceStatus{
			Path:     manifest.String(),
			Renderer: componentApi.SourceRendererKustomize,
		})
	}

	for _, t := range rr.Templates {
		sources = append(sources, componentApi.SourceStatus{
			Path:     t.Path,
			Renderer: componentApi.SourceRendererTemplate,
		})
	}

	for _, h := range rr.HelmCharts {
		sources = append(sources, componentApi.SourceStatus{
			Path:     h.Chart,
			Renderer: componentApi.SourceRendererHelm,
		})
	}

	obj.Status.Module.Sources = sources

	obj.Status.ConfigValues = map[string]string{
		moduleconfig.KeyPlatformType:    m.cfg.PlatformType,
		moduleconfig.KeyPlatformVersion: m.cfg.PlatformVersion,
	}

	return nil
}
