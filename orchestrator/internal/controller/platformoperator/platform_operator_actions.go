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
	"time"

	"github.com/k8s-manifest-kit/engine/pkg/render"
	corev1 "k8s.io/api/core/v1"
	k8serr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/controller-runtime/pkg/client"

	configApi "github.com/lburgazzoli/opendatahub-module-operator/orchestrator/api/config/v1alpha1"
	"github.com/lburgazzoli/opendatahub-module-operator/orchestrator/pkg/resources/gvk"
	actionerrors "github.com/opendatahub-io/operator-actions-framework/controller/actions/errors"
	"github.com/opendatahub-io/operator-actions-framework/controller/types"
)

const (
	defaultPauseDelay        = 5 * time.Second
	platformLookupPauseDelay = 500 * time.Millisecond
)

// checkRunlevel reads the Platform CR status and verifies the module's
// runlevel is at or below the current platform runlevel.
func (r *PlatformOperatorReconciler) checkRunlevel(ctx context.Context, rr *types.ReconciliationRequest) error {
	if !rr.Instance.GetDeletionTimestamp().IsZero() {
		return nil
	}

	mc, err := r.getContext(rr)
	if err != nil {
		return err
	}

	p := &configApi.Platform{}
	p.SetName(configApi.PlatformInstanceName)
	err = rr.Client.Get(ctx, client.ObjectKeyFromObject(p), p)

	switch {
	case k8serr.IsNotFound(err):
		return actionerrors.NewPauseError(
			platformLookupPauseDelay,
			"waiting for Platform %q",
			configApi.PlatformInstanceName,
		)
	case err != nil:
		return fmt.Errorf("getting Platform CR: %w", err)
	}

	upgradeInProgress := p.Status.Distribution.Current != p.Status.Distribution.Target

	if upgradeInProgress && mc.module.Runlevel > p.Status.Runlevel {
		r.reportRunlevelBlocked(rr.Instance, mc.module.Name, mc.module.Runlevel, p.Status.Runlevel)
		return actionerrors.NewPauseError(
			defaultPauseDelay,
			"module %q: waiting for runlevel %d (current: %d)",
			mc.module.Name, mc.module.Runlevel, p.Status.Runlevel,
		)
	}

	return nil
}

// ensureNamespace prepends the module namespace to rr.Resources.
// The namespace is marked as unmanaged so the deploy action only creates
// it if missing and never sets an ownerRef on it.
func (r *PlatformOperatorReconciler) ensureNamespace(_ context.Context, rr *types.ReconciliationRequest) error {
	mc, err := r.getContext(rr)
	if err != nil {
		return err
	}

	ns := corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: mc.module.Namespace,
			Annotations: map[string]string{
				"opendatahub.io/managed": "false",
			},
		},
	}

	return rr.AddResources(&ns)
}

// renderChart renders the module's Helm chart and populates rr.Resources.
func (r *PlatformOperatorReconciler) renderChart(ctx context.Context, rr *types.ReconciliationRequest) error {
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
		return fmt.Errorf("rendering chart for module %q: %w", mc.module.Name, err)
	}

	rr.Resources = append(rr.Resources, rendered...)

	return nil
}

// pruneOrphans diffs current resources against status.resources and deletes orphans.
func (r *PlatformOperatorReconciler) pruneOrphans(ctx context.Context, rr *types.ReconciliationRequest) error {
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
		refGVK := schema.FromAPIVersionAndKind(ref.APIVersion, ref.Kind)

		switch refGVK {
		case gvk.CustomResourceDefinition,
			gvk.Namespace:
			continue
		}

		target := &unstructured.Unstructured{}
		target.SetGroupVersionKind(refGVK)
		target.SetNamespace(ref.Namespace)
		target.SetName(ref.Name)

		err := rr.Client.Delete(ctx, target, client.PropagationPolicy(metav1.DeletePropagationForeground))

		switch {
		case k8serr.IsNotFound(err):
		case err != nil:
			return fmt.Errorf("pruning orphan %s/%s %s/%s: %w", ref.APIVersion, ref.Kind, ref.Namespace, ref.Name, err)
		}
	}

	return nil
}

// reportStatus updates the PlatformOperator status.
func (r *PlatformOperatorReconciler) reportStatus(ctx context.Context, rr *types.ReconciliationRequest) error {
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
	obj.Status.Chart = configApi.ChartInfo{
		Path:       mc.module.Manifests.Chart.Path,
		Name:       mc.module.Manifests.Chart.Name,
		Version:    mc.module.Manifests.Chart.Version,
		AppVersion: mc.module.Manifests.Chart.AppVersion,
	}

	release, found, err := r.readModuleRelease(ctx, rr.Client, mc.module)
	if err != nil {
		return err
	}

	obj.Status.Distribution.Target = configApi.Distribution{
		Name:    r.cfg.Distribution.Name,
		Version: r.cfg.Distribution.Version,
	}
	if !found {
		release = obj.Status.Distribution.Target
	}
	obj.Status.Distribution.Current = release

	recordOperatorInfo(obj.Name, mc.module.Runlevel, obj.Status.Distribution.Current, obj.Status.Distribution.Target)

	return nil
}
