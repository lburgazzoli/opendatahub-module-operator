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

package schemaclaim

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	ctrl "sigs.k8s.io/controller-runtime"

	infraApi "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/api/infrastructure/v1alpha1"
	moduleconfig "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/pkg/config"
	dbcontroller "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/pkg/controller"
	"github.com/opendatahub-io/odh-platform-utilities/framework/controller/actions/deploy"
	"github.com/opendatahub-io/odh-platform-utilities/framework/controller/reconciler"
)

// Module holds process-lifetime state for this controller.
// It embeds Options so that task-specific dependencies can be added via
// With* constructors without changing the Module type. cfg and platformRelease
// live in Options but are not user-configurable (docs/framework.md).
type Controller struct {
	Options
}

func NewController(cfg *moduleconfig.Config, fns ...Option) *Controller {
	r := &Controller{
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

// SchemaClaim CRD
// +kubebuilder:rbac:groups=infrastructure.opendatahub.io,resources=schemaclaims,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=infrastructure.opendatahub.io,resources=schemaclaims/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=infrastructure.opendatahub.io,resources=schemaclaims/finalizers,verbs=update

// Credentials Secret owned by each claim
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update;patch;delete

func NewReconciler(
	ctx context.Context,
	mgr ctrl.Manager,
	cfg *moduleconfig.Config,
) error {
	m := NewController(cfg, Options{
		Recorder: mgr.GetEventRecorderFor(infraApi.DatabaseClaimResource),
	})

	_, err := reconciler.ReconcilerFor(mgr, &infraApi.SchemaClaim{}).
		Owns(&corev1.Secret{}).
		WithReconcilerOpts(
			reconciler.WithRelease(m.platformRelease),
			reconciler.WithDefaultRequeueAfter(cfg.SchemaClaim.RetryInterval),
			reconciler.WithFinalizerName(FinalizerName),
		).
		WithAction(dbcontroller.UpgradeIfNeeded()).
		WithFinalizer(m.cleanupAction).
		WithAction(m.provisionAction).
		WithAction(deploy.NewAction(
			deploy.WithCache(),
			deploy.WithApplyOrder(),
			deploy.WithLabel("app.opendatahub.io/db-operator", "true"),
		)).
		WithConditions(ConditionProvisioned).
		Build(ctx)

	return err
}
