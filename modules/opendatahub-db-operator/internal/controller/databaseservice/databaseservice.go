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

// Package databaseservice contains the DatabaseService module-enablement
// reconciler. DatabaseService is a placeholder at this stage -- it registers
// this module with the ODH Operator without deploying a separate operand
// (docs/plan.md §4 "correction" note). The infrastructure CRDs
// (SchemaClaim/DatabaseClaim/DatabaseProvider) are installed by the Helm
// chart (task-09), not by this reconciler.
package databaseservice

import (
	"context"

	ctrl "sigs.k8s.io/controller-runtime"

	servicesv1alpha1 "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/api/services/v1alpha1"
	moduleconfig "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/pkg/config"
	dbcontroller "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/pkg/controller"
	"github.com/opendatahub-io/odh-platform-utilities/framework/controller/reconciler"
)

// Module holds process-lifetime state for the DatabaseService controller.
// It embeds Options so that task-specific dependencies can be added via
// With* constructors without changing the Module type (docs/framework.md).
type Module struct {
	Options
}

// NewModule creates a Module with one-shot computed state.
func NewModule(cfg *moduleconfig.Config, fns ...Option) *Module {
	r := &Module{
		Options: Options{
			cfg:             cfg,
			platformRelease: cfg.PlatformRelease(),
		},
	}
	for _, fn := range fns {
		fn.applyOption(&r.Options)
	}
	return r
}

// Module operator's own CRD
// +kubebuilder:rbac:groups=services.platform.opendatahub.io,resources=databaseservices,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=services.platform.opendatahub.io,resources=databaseservices/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=services.platform.opendatahub.io,resources=databaseservices/finalizers,verbs=update

// Baseline RBAC required for all module operators
// +kubebuilder:rbac:groups=apiextensions.k8s.io,resources=customresourcedefinitions,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=authentication.k8s.io,resources=tokenreviews,verbs=create
// +kubebuilder:rbac:groups=authorization.k8s.io,resources=subjectaccessreviews,verbs=create
// +kubebuilder:rbac:urls=/metrics,verbs=get

func NewReconciler(
	ctx context.Context,
	mgr ctrl.Manager,
	cfg *moduleconfig.Config,
) error {
	m := NewModule(cfg)

	_, err := reconciler.ReconcilerFor(mgr, &servicesv1alpha1.DatabaseService{}).
		WithReconcilerOpts(
			reconciler.WithRelease(m.platformRelease),
		).
		WithAction(dbcontroller.UpgradeIfNeeded()).
		WithAction(m.reportStatus).
		Build(ctx)

	return err
}
