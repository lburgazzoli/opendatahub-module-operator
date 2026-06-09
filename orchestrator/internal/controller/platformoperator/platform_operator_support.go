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

	engineTypes "github.com/k8s-manifest-kit/engine/pkg/types"
	kitMaps "github.com/k8s-manifest-kit/pkg/util/maps"
	odhLabels "github.com/opendatahub-io/opendatahub-operator/v2/pkg/metadata/labels"
	"github.com/opendatahub-io/operator-actions-framework/controller/types"
	"github.com/opendatahub-io/operator-actions-framework/resources"
	corev1 "k8s.io/api/core/v1"
	k8serr "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"

	configApi "github.com/lburgazzoli/opendatahub-module-operator/orchestrator/api/config/v1alpha1"
	"github.com/lburgazzoli/opendatahub-module-operator/orchestrator/pkg/resources/gvk"
)

// FlattenValues flattens nested maps into dot-separated keys.
// e.g. {"a": {"b": "c"}} → {"a.b": "c"}
func FlattenValues(m map[string]any) map[string]string {
	result := make(map[string]string)
	flattenRecursive("", m, result)
	return result
}

func flattenRecursive(prefix string, m map[string]any, result map[string]string) {
	for k, v := range m {
		key := k
		if prefix != "" {
			key = prefix + "." + k
		}

		switch val := v.(type) {
		case map[string]any:
			flattenRecursive(key, val, result)
		default:
			result[key] = fmt.Sprintf("%v", val)
		}
	}
}

func ToResourceRefs(objects []unstructured.Unstructured) []configApi.ResourceRef {
	refs := make([]configApi.ResourceRef, 0, len(objects))
	for i := range objects {
		refs = append(refs, configApi.ResourceRef{
			APIVersion: objects[i].GetAPIVersion(),
			Kind:       objects[i].GetKind(),
			Namespace:  objects[i].GetNamespace(),
			Name:       objects[i].GetName(),
		})
	}
	return refs
}

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

func (r *ModuleReconciler) moduleNamespace(_ context.Context, rr *types.ReconciliationRequest) (string, error) {
	mc, err := r.getContext(rr)
	if err != nil {
		return "", err
	}

	return mc.module.Namespace, nil
}

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

func (r *ModuleReconciler) readModuleVersion(
	ctx context.Context,
	c client.Client,
	mc *moduleContext,
) (string, error) {
	list := &unstructured.UnstructuredList{}
	list.SetGroupVersionKind(mc.module.GVK)

	err := c.List(ctx, list)

	switch {
	case k8serr.IsNotFound(err):
		return "", nil
	case err != nil:
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

func (r *ModuleReconciler) reportRunlevelBlocked(
	obj client.Object,
	moduleName string,
	requiredRunlevel int,
	currentRunlevel int,
) {
	if r.recorder == nil {
		return
	}

	r.recorder.Eventf(
		obj,
		nil,
		corev1.EventTypeNormal,
		"RunlevelBlocked",
		"WaitForRunlevel",
		"module %q: waiting for runlevel %d (current: %d)",
		moduleName,
		requiredRunlevel,
		currentRunlevel,
	)
}
