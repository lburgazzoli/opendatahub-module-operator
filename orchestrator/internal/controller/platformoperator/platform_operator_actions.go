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

	"github.com/k8s-manifest-kit/engine/pkg/render"
	engineTypes "github.com/k8s-manifest-kit/engine/pkg/types"
	kitMaps "github.com/k8s-manifest-kit/pkg/util/maps"
	k8serr "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/controller-runtime/pkg/client"

	configApi "github.com/lburgazzoli/opendatahub-module-operator/orchestrator/api/config/v1alpha1"
	actionerrors "github.com/opendatahub-io/operator-actions-framework/controller/actions/errors"
	"github.com/opendatahub-io/operator-actions-framework/controller/types"
)

// checkRunlevel verifies the module is allowed to proceed.
func (r *ModuleReconciler) checkRunlevel(_ context.Context, _ *types.ReconciliationRequest) error {
	if !r.o.ShouldReconcileModule(r.m) {
		return actionerrors.NewStopError(
			"module %q: waiting for runlevel %d",
			r.m.EffectiveName(), r.m.Runlevel,
		)
	}

	return nil
}

// renderChart renders the module's Helm chart and populates rr.Resources.
// Labels are applied by the engine's transformer, not manually.
func (r *ModuleReconciler) renderChart(ctx context.Context, rr *types.ReconciliationRequest) error {
	obj, ok := rr.Instance.(*configApi.PlatformOperator)
	if !ok {
		return fmt.Errorf("instance is not a PlatformOperator")
	}

	values, err := r.computeValues(ctx, rr.Client)
	if err != nil {
		return err
	}

	if cfg, ok := values["config"].(map[string]any); ok && len(cfg) > 0 {
		obj.Status.Config = FlattenValues(cfg)
	}

	resources, err := r.engine.Render(ctx, render.WithValues(values))
	if err != nil {
		return fmt.Errorf("rendering chart for module %q: %w", r.m.EffectiveName(), err)
	}

	rr.Resources = append(rr.Resources, resources...)

	return nil
}

// computeValues merges auto-injected values (module GVK, distribution info),
// module values, and config from Configurable ext.
func (r *ModuleReconciler) computeValues(ctx context.Context, c client.Client) (engineTypes.Values, error) {
	values := make(engineTypes.Values, len(r.m.Values))
	maps.Copy(values, r.m.Values)

	if r.m.Config != nil {
		configValues, err := r.m.Config(ctx, c)
		if err != nil {
			return nil, fmt.Errorf("computing config values for module %q: %w", r.m.EffectiveName(), err)
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

	for ref := range previous.Difference(current) {
		target := &unstructured.Unstructured{}
		target.SetGroupVersionKind(schema.FromAPIVersionAndKind(ref.APIVersion, ref.Kind))
		target.SetNamespace(ref.Namespace)
		target.SetName(ref.Name)

		if err := rr.Client.Delete(ctx, target); err != nil && !k8serr.IsNotFound(err) {
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

	obj.Status.Resources = ToResourceRefs(rr.Resources)
	obj.Status.Runlevel = r.m.Runlevel

	obj.Status.Chart = r.chartInfo

	version, err := r.readModuleVersion(ctx, rr.Client)
	if err != nil {
		return err
	}
	obj.Status.DeployedVersion = version

	return nil
}

// readModuleVersion reads the module CR (singleton, by GVK) and extracts the version.
func (r *ModuleReconciler) readModuleVersion(ctx context.Context, c client.Client) (string, error) {
	list := &unstructured.UnstructuredList{}
	list.SetGroupVersionKind(r.m.GVK)

	if err := c.List(ctx, list); err != nil {
		if k8serr.IsNotFound(err) {
			return "", nil
		}
		return "", fmt.Errorf("listing module CR %s: %w", r.m.GVK.Kind, err)
	}

	if len(list.Items) == 0 {
		return "", nil
	}

	version, found, err := unstructured.NestedString(list.Items[0].Object, "status", "release", "version")
	if err != nil {
		return "", fmt.Errorf("reading version from module CR %s: %w", r.m.GVK.Kind, err)
	}
	if !found {
		return "", nil
	}

	return version, nil
}
