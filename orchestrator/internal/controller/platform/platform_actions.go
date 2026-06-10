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

package platform

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"strconv"
	"time"

	corev1 "k8s.io/api/core/v1"
	k8serr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/tools/events"
	"sigs.k8s.io/controller-runtime/pkg/client"

	configApi "github.com/lburgazzoli/opendatahub-module-operator/orchestrator/api/config/v1alpha1"
	"github.com/lburgazzoli/opendatahub-module-operator/orchestrator/pkg/config"
	"github.com/lburgazzoli/opendatahub-module-operator/orchestrator/pkg/module"
	"github.com/opendatahub-io/operator-actions-framework/controller/conditions"
	"github.com/opendatahub-io/operator-actions-framework/controller/types"
)

const (
	adminAckPauseDelay = 30 * time.Second
)

type PlatformReconciler struct {
	registry *module.Registry
	cfg      *config.Config
	recorder events.EventRecorder
}

// initialize sets the runlevel from Platform CR status. On a fresh Platform
// (runlevel 0), initializes to the first runlevel that has modules.
func (a *PlatformReconciler) initialize(_ context.Context, rr *types.ReconciliationRequest) error {
	obj, ok := rr.Instance.(*configApi.Platform)
	if !ok {
		return fmt.Errorf("instance is not a Platform")
	}

	obj.Status.Distribution.Target = configApi.Distribution{
		Name:    a.cfg.Distribution.Name,
		Version: a.cfg.Distribution.Version,
	}

	if obj.Status.Runlevel == 0 {
		obj.Status.Runlevel = a.registry.FirstRunlevel()
	}
	if len(obj.Spec.Modules) == 0 {
		obj.Status.Distribution.Current = obj.Status.Distribution.Target
	}

	return nil
}

// checkAdminAcks blocks reconciliation until all admin ack keys required by
// the enabled modules are set to boolean true in the dedicated admin-acks
// ConfigMap.
func (a *PlatformReconciler) checkAdminAcks(ctx context.Context, rr *types.ReconciliationRequest) error {
	obj, ok := rr.Instance.(*configApi.Platform)
	if !ok {
		return fmt.Errorf("instance is not a Platform")
	}

	required := a.requiredAdminAcks(obj)
	if len(required) == 0 {
		return nil
	}

	adminAcksConfigMap := &corev1.ConfigMap{}
	adminAcksConfigMap.SetName(config.AdminAcksConfigMapName)
	adminAcksConfigMap.SetNamespace(a.cfg.Namespace())

	err := rr.Client.Get(ctx, client.ObjectKeyFromObject(adminAcksConfigMap), adminAcksConfigMap)
	switch {
	case k8serr.IsNotFound(err):
		unsatisfied := missingAdminAcks(required)
		a.reportUnsatisfiedAdminAcks(obj, unsatisfied)
		return adminAcksPauseError(a.cfg.Namespace(), unsatisfied)
	case err != nil:
		return fmt.Errorf(
			"getting admin-acks ConfigMap %s/%s: %w",
			a.cfg.Namespace(),
			config.AdminAcksConfigMapName,
			err,
		)
	}

	unsatisfied := unsatisfiedAdminAcks(required, adminAcksConfigMap.Data, strconv.ParseBool)

	if len(unsatisfied) > 0 {
		a.reportUnsatisfiedAdminAcks(obj, unsatisfied)
		return adminAcksPauseError(a.cfg.Namespace(), unsatisfied)
	}

	return nil
}

// ensureModules builds PlatformOperator resources from spec.modules.
func (a *PlatformReconciler) ensureModules(ctx context.Context, rr *types.ReconciliationRequest) error {
	obj, ok := rr.Instance.(*configApi.Platform)
	if !ok {
		return fmt.Errorf("instance is not a Platform")
	}

	for _, name := range obj.Spec.Modules {
		m := a.registry.ModuleByName(name)
		if m == nil {
			return fmt.Errorf("module %q not registered", name)
		}

		po := configApi.PlatformOperator{}
		po.SetName(m.Name)

		if err := rr.AddResources(&po); err != nil {
			return fmt.Errorf("%w", err)
		}
	}

	rr.Generated = true

	return nil
}

// pruneModules deletes PlatformOperators that are no longer present in spec.modules.
func (a *PlatformReconciler) pruneModules(ctx context.Context, rr *types.ReconciliationRequest) error {
	obj, ok := rr.Instance.(*configApi.Platform)
	if !ok {
		return fmt.Errorf("instance is not a Platform")
	}

	desired := sets.New[string]()
	for _, name := range obj.Spec.Modules {
		m := a.registry.ModuleByName(name)
		if m != nil {
			desired.Insert(m.Name)
			continue
		}

		desired.Insert(name)
	}

	var poList configApi.PlatformOperatorList
	if err := rr.Client.List(ctx, &poList); err != nil {
		return fmt.Errorf("listing PlatformOperators: %w", err)
	}

	for i := range poList.Items {
		po := &poList.Items[i]
		if desired.Has(po.Name) || !po.GetDeletionTimestamp().IsZero() {
			continue
		}

		err := rr.Client.Delete(ctx, po, client.PropagationPolicy(metav1.DeletePropagationForeground))

		switch {
		case k8serr.IsNotFound(err):
		case err != nil:
			return fmt.Errorf("deleting PlatformOperator %q: %w", po.Name, err)
		default:
			a.reportModulePruned(obj, po.Name)
		}
	}

	return nil
}

// advanceRunlevel checks whether all enabled modules at the current runlevel
// have reported the expected distribution version. If so, advances to the
// next runlevel that has enabled modules.
func (a *PlatformReconciler) advanceRunlevel(ctx context.Context, rr *types.ReconciliationRequest) error {
	obj, ok := rr.Instance.(*configApi.Platform)
	if !ok {
		return fmt.Errorf("instance is not a Platform")
	}

	complete, err := a.runlevelComplete(ctx, rr, obj, obj.Status.Runlevel)
	if err != nil || !complete {
		return err
	}

	level := obj.Status.Runlevel
	for {
		next, hasNext := a.registry.NextRunlevel(level)
		if !hasNext {
			break
		}

		if len(a.enabledModulesAtRunlevel(obj, next)) > 0 {
			obj.Status.Runlevel = next
			return nil
		}

		level = next
	}

	return nil
}

// aggregateStatus populates Platform status from PlatformOperator statuses.
// Modules are sorted by runlevel then name for stable ordering.
func (a *PlatformReconciler) aggregateStatus(ctx context.Context, rr *types.ReconciliationRequest) error {
	obj, ok := rr.Instance.(*configApi.Platform)
	if !ok {
		return fmt.Errorf("instance is not a Platform")
	}

	obj.Status.Distribution.Target = configApi.Distribution{
		Name:    a.cfg.Distribution.Name,
		Version: a.cfg.Distribution.Version,
	}

	obj.Status.Modules = nil
	allUpToDate := true

	for _, name := range obj.Spec.Modules {
		m := a.registry.ModuleByName(name)
		if m == nil {
			continue
		}

		summary, err := a.moduleStatus(ctx, rr.Client, m)
		if err != nil {
			return err
		}

		if summary.Distribution != obj.Status.Distribution.Target {
			allUpToDate = false
		}

		obj.Status.Modules = append(obj.Status.Modules, summary)
	}

	if allUpToDate {
		obj.Status.Distribution.Current = obj.Status.Distribution.Target
		rr.Conditions.MarkTrue(
			configApi.ConditionUpToDate,
			conditions.WithReason("UpToDate"),
			conditions.WithMessage("current distribution matches target"),
		)
	} else {
		rr.Conditions.MarkFalse(
			configApi.ConditionUpToDate,
			conditions.WithReason("Updating"),
			conditions.WithMessage("current distribution does not match target"),
		)
	}

	slices.SortFunc(obj.Status.Modules, func(a, b configApi.ModuleStatusSummary) int {
		if c := cmp.Compare(a.Runlevel, b.Runlevel); c != 0 {
			return c
		}
		return cmp.Compare(a.Name, b.Name)
	})

	return nil
}
