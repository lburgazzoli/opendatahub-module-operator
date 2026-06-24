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

package sparkoperator

import (
	"context"
	"fmt"

	odhtypes "github.com/opendatahub-io/odh-platform-utilities/framework/controller/types"

	componentApi "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-spark-operator/api/components/v1alpha1"
)

func (m *Module) upgradeIfNeeded(ctx context.Context, rr *odhtypes.ReconciliationRequest) error {
	obj, ok := rr.Instance.(*componentApi.SparkOperator)
	if !ok {
		return fmt.Errorf("instance is not a SparkOperator")
	}

	prev := obj.Status.Release

	if !rr.Release.Version.GT(prev.Version.Version) {
		return nil
	}

	return m.upgrade(ctx, prev, rr)
}

// upgrade runs idempotent migrations when the platform version advances.
// Implement version-gated migrations here.
func (m *Module) upgrade(_ context.Context, prev componentApi.Release, rr *odhtypes.ReconciliationRequest) error {
	_ = prev
	_ = rr
	// Add version-gated migrations here, e.g.:
	// if rr.Release.Version.GT(semver.MustParse("1.0.0")) { ... }
	return nil
}
