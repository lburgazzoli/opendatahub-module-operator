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

package platformoperator

import (
	"context"
	"fmt"

	"github.com/k8s-manifest-kit/engine/pkg/transformer/meta/labels"
	"github.com/k8s-manifest-kit/engine/pkg/transformer/meta/namespace"
	helm "github.com/k8s-manifest-kit/renderer-helm/pkg"
	"helm.sh/helm/v4/pkg/chart"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	configApi "github.com/lburgazzoli/opendatahub-module-operator/orchestrator/api/config/v1alpha1"
	"github.com/lburgazzoli/opendatahub-module-operator/orchestrator/internal/controller/platform"
	"github.com/lburgazzoli/opendatahub-module-operator/orchestrator/pkg/module"
	odhLabels "github.com/opendatahub-io/opendatahub-operator/v2/pkg/metadata/labels"
	"github.com/opendatahub-io/operator-actions-framework/controller/actions/deploy"
	"github.com/opendatahub-io/operator-actions-framework/controller/actions/status/deployments"
	"github.com/opendatahub-io/operator-actions-framework/controller/reconciler"
)

// SpawnModuleReconciler creates and starts a ReconcilerFor for a specific module.
func SpawnModuleReconciler(
	ctx context.Context,
	mgr ctrl.Manager,
	o *platform.Orchestrator,
	m *module.Module,
) error {
	e, err := helm.NewEngine(
		helm.Source{
			Chart:       m.ChartPath,
			ReleaseName: m.EffectiveName(),
			Values: helm.Values(map[string]any{
				"module": map[string]any{
					"group":   m.GVK.Group,
					"version": m.GVK.Version,
					"kind":    m.GVK.Kind,
				},
				"distribution": map[string]any{
					"name":    o.Config().PlatformName,
					"version": o.Config().PlatformVersion,
				},
			}),
		},
		helm.WithTransformer(namespace.EnsureDefault(m.Namespace)),
		helm.WithTransformer(labels.Set(map[string]string{
			odhLabels.PlatformPartOf: m.EffectiveName(),
		})),
	)
	if err != nil {
		return fmt.Errorf("creating engine for module %q: %w", m.EffectiveName(), err)
	}

	ci := configApi.ChartInfo{Path: m.ChartPath}

	if chrt, chartErr := m.Chart(); chartErr == nil && chrt != nil {
		if acc, accErr := chart.NewAccessor(chrt); accErr == nil {
			ci.Name = acc.Name()
			if md := acc.MetadataAsMap(); md != nil {
				if v, ok := md["version"].(string); ok {
					ci.Version = v
				}
				if v, ok := md["appVersion"].(string); ok {
					ci.AppVersion = v
				}
			}
		}
	}

	r := &ModuleReconciler{o: o, m: m, engine: e, chartInfo: ci}

	_, err = reconciler.ReconcilerFor(
		mgr,
		&configApi.PlatformOperator{},
		builder.WithPredicates(predicate.NewPredicateFuncs(func(obj client.Object) bool {
			return obj.GetName() == m.EffectiveName()
		})),
	).
		WithInstanceName(m.EffectiveName()).
		WithDynamicOwnership().
		WithAction(r.checkRunlevel).
		WithAction(r.renderChart).
		WithAction(deploy.NewAction(
			deploy.WithCache(),
			deploy.WithApplyOrder(),
		)).
		WithAction(deployments.NewAction(
			deployments.InNamespace(m.Namespace),
		)).
		WithAction(r.pruneOrphans).
		WithAction(r.reportStatus).
		Build(ctx)

	return err
}
