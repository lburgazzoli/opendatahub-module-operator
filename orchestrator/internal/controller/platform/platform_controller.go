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
	"context"

	"k8s.io/apimachinery/pkg/runtime/schema"
	ctrl "sigs.k8s.io/controller-runtime"

	configApi "github.com/lburgazzoli/opendatahub-module-operator/orchestrator/api/config/v1alpha1"
	"github.com/opendatahub-io/operator-actions-framework/api"
	"github.com/opendatahub-io/operator-actions-framework/controller/actions/deploy"
	"github.com/opendatahub-io/operator-actions-framework/controller/actions/gc"
	"github.com/opendatahub-io/operator-actions-framework/controller/reconciler"
	odhTypes "github.com/opendatahub-io/operator-actions-framework/controller/types"
)

const (
	ConditionModulesReady       = "ModulesReady"
	ConditionUpgradeProgressing = "UpgradeProgressing"
)

// +kubebuilder:rbac:groups=config.opendatahub.io,resources=platforms,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=config.opendatahub.io,resources=platforms/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=config.opendatahub.io,resources=platforms/finalizers,verbs=update
// +kubebuilder:rbac:groups=config.opendatahub.io,resources=platformoperators,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=config.opendatahub.io,resources=platformoperators/status,verbs=get;update;patch
// +kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch

func NewReconciler(
	ctx context.Context,
	mgr ctrl.Manager,
	o *Orchestrator,
) error {
	rel := o.cfg.Release()
	ns := o.cfg.Namespace()

	_, err := reconciler.ReconcilerFor(mgr, &configApi.Platform{}).
		Owns(&configApi.PlatformOperator{}).
		WithReconcilerOpts(
			reconciler.WithRelease(api.Release{Name: rel.Name, Version: rel.Version.Version}),
		).
		WithAction(o.initialize).
		WithAction(o.checkAdminAcks).
		WithAction(o.ensureModules).
		WithAction(deploy.NewAction(
			deploy.WithCache()),
		).
		WithAction(o.checkAdvancement).
		WithAction(o.advanceOrSwitch).
		WithAction(o.aggregateStatus).
		WithConditions(
			ConditionModulesReady,
			ConditionUpgradeProgressing,
		).
		WithAction(gc.NewAction(
			func(_ context.Context, _ *odhTypes.ReconciliationRequest) (string, error) {
				return ns, nil
			},
			gc.WithTypePredicate(
				func(rr *odhTypes.ReconciliationRequest, objGVK schema.GroupVersionKind) (bool, error) {
					return rr.Controller.Owns(objGVK), nil
				},
			),
		)).
		Build(ctx)

	return err
}
