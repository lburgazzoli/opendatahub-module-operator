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
	"maps"
	"time"

	"github.com/k8s-manifest-kit/engine/pkg/render"
	"github.com/k8s-manifest-kit/engine/pkg/transformer/meta/namespace"
	engineTypes "github.com/k8s-manifest-kit/engine/pkg/types"
	kitMaps "github.com/k8s-manifest-kit/pkg/util/maps"
	helm "github.com/k8s-manifest-kit/renderer-helm/pkg"
	"helm.sh/helm/v4/pkg/chart"
	k8serr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/controller-runtime/pkg/client"

	configApi "github.com/lburgazzoli/opendatahub-module-operator/orchestrator/api/config/v1alpha1"
	"github.com/lburgazzoli/opendatahub-module-operator/orchestrator/pkg/resources/gvk"
	odhLabels "github.com/opendatahub-io/opendatahub-operator/v2/pkg/metadata/labels"
	actionerrors "github.com/opendatahub-io/operator-actions-framework/controller/actions/errors"
	"github.com/opendatahub-io/operator-actions-framework/controller/types"
	"github.com/opendatahub-io/operator-actions-framework/resources"
)

const defaultPauseDelay = 5 * time.Second

// resolveModule looks up the module by PlatformOperator name and lazily
// creates the Helm engine. Must be the first action in the pipeline.
func (r *ModuleReconciler) resolveModule(_ context.Context, rr *types.ReconciliationRequest) error {
	name := rr.Instance.GetName()

	r.mu.RLock()
	_, ok := r.contexts[name]
	r.mu.RUnlock()

	if ok {
		return nil
	}

	m := r.o.ModuleByName(name)
	if m == nil {
		return fmt.Errorf("module %q not registered", name)
	}

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
					"name":    r.cfg.PlatformName,
					"version": r.cfg.PlatformVersion,
				},
			}),
		},
		helm.WithTransformer(namespace.EnsureDefault(m.Namespace)),
		helm.WithTransformer(moduleMetadata(m.EffectiveName())),
	)
	if err != nil {
		return fmt.Errorf("creating engine for module %q: %w", name, err)
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

	r.mu.Lock()
	r.contexts[name] = &moduleContext{module: m, engine: e, chartInfo: ci}
	r.mu.Unlock()

	return nil
}

// getContext returns the cached module context for the current PlatformOperator.
func (r *ModuleReconciler) getContext(rr *types.ReconciliationRequest) (*moduleContext, error) {
	name := rr.Instance.GetName()

	r.mu.RLock()
	mc, ok := r.contexts[name]
	r.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("module context for %q not found", name)
	}

	return mc, nil
}

// moduleNamespace returns the namespace for the module being reconciled.
func (r *ModuleReconciler) moduleNamespace(_ context.Context, rr *types.ReconciliationRequest) (string, error) {
	mc, err := r.getContext(rr)
	if err != nil {
		return "", err
	}

	return mc.module.Namespace, nil
}

// checkRunlevel verifies the module is allowed to proceed.
func (r *ModuleReconciler) checkRunlevel(_ context.Context, rr *types.ReconciliationRequest) error {
	mc, err := r.getContext(rr)
	if err != nil {
		return err
	}

	if !r.o.ShouldReconcileModule(mc.module) {
		return actionerrors.NewPauseError(
			defaultPauseDelay,
			"module %q: waiting for runlevel %d",
			mc.module.EffectiveName(), mc.module.Runlevel,
		)
	}

	return nil
}

// moduleMetadata returns a transformer that stamps resources with the
// module's part-of label (skipping CRDs) and config annotation.
func moduleMetadata(name string) engineTypes.Transformer {
	annotationKey := "config.opendatahub.io/" + name

	return func(_ context.Context, obj unstructured.Unstructured) (unstructured.Unstructured, error) {
		if obj.GroupVersionKind() != gvk.CustomResourceDefinition {
			resources.SetLabel(&obj, odhLabels.PlatformPartOf, name)
		}

		resources.SetAnnotation(&obj, annotationKey, "true")

		return obj, nil
	}
}

// renderChart renders the module's Helm chart and populates rr.Resources.
func (r *ModuleReconciler) renderChart(ctx context.Context, rr *types.ReconciliationRequest) error {
	obj, ok := rr.Instance.(*configApi.PlatformOperator)
	if !ok {
		return fmt.Errorf("instance is not a PlatformOperator")
	}

	mc, err := r.getContext(rr)
	if err != nil {
		return err
	}

	values, err := r.computeValues(ctx, rr.Client, mc)
	if err != nil {
		return err
	}

	if cfg, ok := values["config"].(map[string]any); ok && len(cfg) > 0 {
		obj.Status.Config = FlattenValues(cfg)
	}

	rendered, err := mc.engine.Render(ctx, render.WithValues(values))
	if err != nil {
		return fmt.Errorf("rendering chart for module %q: %w", mc.module.EffectiveName(), err)
	}

	rr.Resources = append(rr.Resources, rendered...)

	return nil
}

func (r *ModuleReconciler) computeValues(
	ctx context.Context,
	c client.Client,
	mc *moduleContext,
) (engineTypes.Values, error) {
	values := make(engineTypes.Values, len(mc.module.Values))
	maps.Copy(values, mc.module.Values)

	if mc.module.Config != nil {
		configValues, err := mc.module.Config(ctx, c)
		if err != nil {
			return nil, fmt.Errorf("computing config values for module %q: %w", mc.module.EffectiveName(), err)
		}
		if len(configValues) > 0 {
			values["config"] = kitMaps.DeepMerge(
				func() map[string]any {
					if existing, ok := values["config"].(map[string]any); ok {
						return existing
					}
					return nil
				}(),
				configValues,
			)
		}
	}

	return values, nil
}

// pruneOrphans diffs current resources against status.resources and deletes orphans.
func (r *ModuleReconciler) pruneOrphans(ctx context.Context, rr *types.ReconciliationRequest) error {
	obj, ok := rr.Instance.(*configApi.PlatformOperator)
	if !ok {
		return fmt.Errorf("instance is not a PlatformOperator")
	}

	if len(obj.Status.Resources) == 0 {
		return nil
	}

	current := sets.New(ToResourceRefs(rr.Resources)...)
	previous := sets.New(obj.Status.Resources...)

	propagation := metav1.DeletePropagationForeground

	for ref := range previous.Difference(current) {
		refGVK := schema.FromAPIVersionAndKind(ref.APIVersion, ref.Kind)

		switch refGVK {
		case gvk.CustomResourceDefinition:
			continue
		}

		target := &unstructured.Unstructured{}
		target.SetGroupVersionKind(refGVK)
		target.SetNamespace(ref.Namespace)
		target.SetName(ref.Name)

		err := rr.Client.Delete(ctx, target, &client.DeleteOptions{
			PropagationPolicy: &propagation,
		})
		if err != nil && !k8serr.IsNotFound(err) {
			return fmt.Errorf("pruning orphan %s/%s %s/%s: %w", ref.APIVersion, ref.Kind, ref.Namespace, ref.Name, err)
		}
	}

	return nil
}

// reportStatus updates the PlatformOperator status.
func (r *ModuleReconciler) reportStatus(ctx context.Context, rr *types.ReconciliationRequest) error {
	obj, ok := rr.Instance.(*configApi.PlatformOperator)
	if !ok {
		return fmt.Errorf("instance is not a PlatformOperator")
	}

	mc, err := r.getContext(rr)
	if err != nil {
		return err
	}

	obj.Status.Resources = ToResourceRefs(rr.Resources)
	obj.Status.Runlevel = mc.module.Runlevel
	obj.Status.Chart = mc.chartInfo

	version, err := r.readModuleVersion(ctx, rr.Client, mc)
	if err != nil {
		return err
	}
	obj.Status.DeployedVersion = version

	return nil
}

func (r *ModuleReconciler) readModuleVersion(
	ctx context.Context,
	c client.Client,
	mc *moduleContext,
) (string, error) {
	list := &unstructured.UnstructuredList{}
	list.SetGroupVersionKind(mc.module.GVK)

	if err := c.List(ctx, list); err != nil {
		if k8serr.IsNotFound(err) {
			return "", nil
		}
		return "", fmt.Errorf("listing module CR %s: %w", mc.module.GVK.Kind, err)
	}

	if len(list.Items) == 0 {
		return "", nil
	}

	version, found, err := unstructured.NestedString(list.Items[0].Object, "status", "release", "version")
	if err != nil {
		return "", fmt.Errorf("reading version from module CR %s: %w", mc.module.GVK.Kind, err)
	}
	if !found {
		return "", nil
	}

	return version, nil
}
