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
	"errors"
	"fmt"
	"maps"

	engineTypes "github.com/k8s-manifest-kit/engine/pkg/types"
	kitMaps "github.com/k8s-manifest-kit/pkg/util/maps"
	helm "github.com/k8s-manifest-kit/renderer-helm/pkg"
	odhLabels "github.com/opendatahub-io/opendatahub-operator/v2/pkg/metadata/labels"
	"github.com/opendatahub-io/operator-actions-framework/controller/types"
	corev1 "k8s.io/api/core/v1"
	k8serr "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	configApi "github.com/lburgazzoli/opendatahub-module-operator/orchestrator/api/config/v1alpha1"
	orchestratorconfig "github.com/lburgazzoli/opendatahub-module-operator/orchestrator/pkg/config"
	"github.com/lburgazzoli/opendatahub-module-operator/orchestrator/pkg/module"
	"github.com/lburgazzoli/opendatahub-module-operator/orchestrator/pkg/resources"
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

func (r *PlatformOperatorReconciler) getContext(rr *types.ReconciliationRequest) (*moduleContext, error) {
	name := rr.Instance.GetName()

	mc, ok := r.contexts[name]

	if !ok {
		return nil, fmt.Errorf("module context for %q not found", name)
	}

	return mc, nil
}

func newModuleContext(
	m *module.Module,
	cfg *orchestratorconfig.Config,
) (*moduleContext, error) {
	e, err := helm.NewEngine(
		helm.Source{
			Chart:            m.Manifests.Chart.Path,
			ReleaseName:      m.Name,
			ReleaseNamespace: m.Namespace,
			ReleaseVersion:   cfg.Distribution.Version,
			Values: helm.Values(map[string]any{
				"module": map[string]any{
					"group":   m.GVK.Group,
					"version": m.GVK.Version,
					"kind":    m.GVK.Kind,
				},
				"distribution": map[string]any{
					"name":    cfg.Distribution.Name,
					"version": cfg.Distribution.Version,
				},
			}),
		},
		helm.WithTransformer(moduleMetadata(m.Name)),
	)
	if err != nil {
		return nil, fmt.Errorf("creating engine for module %q: %w", m.Name, err)
	}

	return &moduleContext{
		module: m,
		engine: e,
	}, nil
}

func (r *PlatformOperatorReconciler) moduleNamespace(_ context.Context, rr *types.ReconciliationRequest) (string, error) {
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

func (r *PlatformOperatorReconciler) computeValues(
	ctx context.Context,
	c client.Client,
	mc *moduleContext,
) (engineTypes.Values, error) {
	values := make(engineTypes.Values, len(mc.module.Values))
	maps.Copy(values, mc.module.Values)

	if mc.module.Config == nil {
		return values, nil
	}

	vals, err := mc.module.Config(ctx, c)
	if err != nil {
		return nil, fmt.Errorf("computing config values for module %q: %w", mc.module.Name, err)
	}

	if len(vals) > 0 {
		values["config"] = kitMaps.DeepMerge(
			func() map[string]any {
				if existing, ok := values["config"].(map[string]any); ok {
					return existing
				}
				return nil
			}(),
			vals,
		)
	}

	return values, nil
}

func (r *PlatformOperatorReconciler) readModuleRelease(
	ctx context.Context,
	c client.Client,
	mod *module.Module,
) (configApi.Distribution, bool, error) {
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(mod.GVK)

	err := resources.GetSingleton(ctx, c, obj)
	switch {
	case errors.Is(err, resources.ErrNoInstance):
		return configApi.Distribution{}, false, nil
	case err != nil:
		return configApi.Distribution{}, false, fmt.Errorf("getting singleton module CR %s: %w", mod.GVK.Kind, err)
	}

	release, _, err := unstructured.NestedStringMap(obj.Object, "status", "release")
	if err != nil {
		return configApi.Distribution{}, true, fmt.Errorf("reading release from module CR %s: %w", mod.GVK.Kind, err)
	}

	return configApi.Distribution{
		Name:    release["name"],
		Version: release["version"],
	}, true, nil
}

func (r *PlatformOperatorReconciler) reportRunlevelBlocked(
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

// platformChangePredicate triggers only when the Platform CR's runlevel or
// target distribution changes.
func (r *PlatformOperatorReconciler) platformChangePredicate() predicate.Predicate {
	return predicate.Funcs{
		CreateFunc:  func(_ event.CreateEvent) bool { return true },
		DeleteFunc:  func(_ event.DeleteEvent) bool { return false },
		GenericFunc: func(_ event.GenericEvent) bool { return false },
		UpdateFunc: func(e event.UpdateEvent) bool {
			oldPlatform, oldErr := resources.Decode[*configApi.Platform](e.ObjectOld)
			if oldErr != nil {
				ctrl.Log.Error(oldErr, "reading old Platform")
				return false
			}

			newPlatform, newErr := resources.Decode[*configApi.Platform](e.ObjectNew)
			if newErr != nil {
				ctrl.Log.Error(newErr, "reading new Platform")
				return false
			}

			switch {
			case oldPlatform.Status.Distribution.Target != newPlatform.Status.Distribution.Target:
				return true
			case oldPlatform.Status.Runlevel != newPlatform.Status.Runlevel:
				return true
			default:
				return false
			}
		},
	}
}

// eligibleModuleRequests maps a Platform CR event to reconcile requests for
// modules that actually need a wakeup. If a module is already at the Platform
// target distribution, no wakeup is needed. Otherwise, modules whose own target
// differs from the Platform target are woken immediately; if the target already
// matches, only modules at or above the current Platform runlevel are woken.
func (r *PlatformOperatorReconciler) eligibleModuleRequests(
	ctx context.Context,
	obj client.Object,
) []ctrl.Request {
	platform, err := resources.Decode[*configApi.Platform](obj)
	if err != nil {
		ctrl.Log.Error(err, "reading Platform")
		return nil
	}

	requests := make([]ctrl.Request, 0, len(platform.Spec.Modules))

	for _, name := range platform.Spec.Modules {
		m := r.registry.ModuleByName(name)
		if m == nil {

			// Skip disabled or unknown PlatformOperators; the Platform controller
			// will prune leftovers separately.
			continue
		}

		o := configApi.PlatformOperator{}
		o.SetName(m.Name)

		err := r.client.Get(ctx, client.ObjectKeyFromObject(&o), &o)
		switch {
		case k8serr.IsNotFound(err):
			continue
		case err != nil:
			ctrl.Log.Error(err, "getting PlatformOperator", "name", name)
			continue
		}

		switch {
		case !o.GetDeletionTimestamp().IsZero():
			// PlatformOperator is being deleted, so there is nothing to wake up.
			continue
		case o.Status.Distribution.Current == platform.Status.Distribution.Target:
			// Already on the Platform target version; no forced wakeup needed.
			continue
		case o.Status.Distribution.Target != platform.Status.Distribution.Target:
			// A target-version change should wake the module immediately.
			requests = append(requests, ctrl.Request{
				NamespacedName: client.ObjectKey{Name: m.Name},
			})
		case m.Runlevel >= platform.Status.Runlevel:
			// Runlevel-only changes wake modules at or above the current runlevel.
			requests = append(requests, ctrl.Request{
				NamespacedName: client.ObjectKey{Name: m.Name},
			})
		}
	}

	return requests
}
