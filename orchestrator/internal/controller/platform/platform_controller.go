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

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	configApi "github.com/lburgazzoli/opendatahub-module-operator/orchestrator/api/config/v1alpha1"
	orchestratorconfig "github.com/lburgazzoli/opendatahub-module-operator/orchestrator/pkg/config"
	"github.com/lburgazzoli/opendatahub-module-operator/orchestrator/pkg/module"
	"github.com/opendatahub-io/operator-actions-framework/api"
	"github.com/opendatahub-io/operator-actions-framework/controller/actions/deploy"
	"github.com/opendatahub-io/operator-actions-framework/controller/reconciler"
)

const (
	ConditionModulesReady = "ModulesReady"
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
	registry *module.ModuleRegistry,
	cfg *orchestratorconfig.Config,
) error {
	rel := cfg.Release()
	actions := &platformActions{
		registry: registry,
		cfg:      cfg,
	}

	_, err := reconciler.ReconcilerFor(mgr, &configApi.Platform{}).
		Owns(&configApi.PlatformOperator{},
			reconciler.WithPredicates(predicate.ResourceVersionChangedPredicate{}),
		).
		WithReconcilerOpts(
			reconciler.WithRelease(api.Release{Name: rel.Name, Version: rel.Version.Version}),
		).
		WithAction(actions.initialize).
		WithAction(actions.checkAdminAcks).
		WithAction(actions.ensureModules).
		WithAction(deploy.NewAction(
			deploy.WithCache()),
		).
		WithAction(actions.pruneModules).
		WithAction(actions.advanceRunlevel).
		WithAction(actions.aggregateStatus).
		WithConditions(
			ConditionModulesReady,
		).
		Build(ctx)

	return err
}
