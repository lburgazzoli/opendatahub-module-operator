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

// Package databaseprovider reconciles databaseprovider objects.
// Task-specific action implementations live here.
package databaseprovider

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	infraApi "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/api/infrastructure/v1alpha1"
	dbcontroller "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/pkg/controller"
	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/pkg/postgres"
	"github.com/opendatahub-io/odh-platform-utilities/framework/controller/conditions"
	odhtypes "github.com/opendatahub-io/odh-platform-utilities/framework/controller/types"
)

func (m *Controller) reconcileExternalAction(
	ctx context.Context,
	rr *odhtypes.ReconciliationRequest,
) error {
	obj, ok := rr.Instance.(*infraApi.DatabaseProvider)
	if !ok {
		return fmt.Errorf("instance is not a DatabaseProvider")
	}
	if obj.Spec.Type != infraApi.ProviderTypeExternal {
		return nil
	}

	cfg, err := loadExternalConfig(ctx, rr.Client, obj)
	if err != nil {
		rr.Conditions.Mark(ConditionReachable, metav1.ConditionFalse,
			conditions.WithReason(externalFailureReason(err)),
			conditions.WithMessage("%s", err.Error()))
		return nil
	}

	if err := postgres.Ping(ctx, cfg); err != nil {
		rr.Conditions.Mark(ConditionReachable, metav1.ConditionFalse,
			conditions.WithReason(externalFailureReason(err)),
			conditions.WithMessage("%s", err.Error()))
		if retryErr := dbcontroller.StopWithQuickRetryIfConnectionRefused(err); retryErr != nil {
			return retryErr
		}
		return nil
	}

	rr.Conditions.Mark(ConditionReachable, metav1.ConditionTrue,
		conditions.WithReason("ConnectionVerified"),
		conditions.WithMessage("Connection verified"))

	return nil
}

func (m *Controller) reconcileEmbeddedAction(
	_ context.Context,
	rr *odhtypes.ReconciliationRequest,
) error {
	obj, ok := rr.Instance.(*infraApi.DatabaseProvider)
	if !ok {
		return fmt.Errorf("instance is not a DatabaseProvider")
	}
	if obj.Spec.Type != infraApi.ProviderTypeEmbedded {
		return nil
	}

	return nil
}
