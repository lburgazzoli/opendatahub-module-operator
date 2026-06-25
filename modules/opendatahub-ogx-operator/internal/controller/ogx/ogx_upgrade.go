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

package ogx

import (
	"context"
	"fmt"

	"github.com/blang/semver/v4"
	odhtypes "github.com/opendatahub-io/odh-platform-utilities/framework/controller/types"

	componentApi "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-ogx-operator/api/components/v1alpha1"
	moduleconfig "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-ogx-operator/pkg/config"
)

func (m *Module) upgradeIfNeeded(ctx context.Context, rr *odhtypes.ReconciliationRequest) error {
	obj, ok := rr.Instance.(*componentApi.OGX)
	if !ok {
		return fmt.Errorf("instance is not an OGX")
	}

	prev, _ := GetRelease(obj.GetReleaseStatus(), moduleconfig.ReleasePlatform)

	var prevVersion semver.Version
	if prev.Version != "" {
		var err error
		prevVersion, err = semver.ParseTolerant(prev.Version)
		if err != nil {
			return fmt.Errorf("parsing previous platform version: %w", err)
		}
	}

	if !rr.Release.Version.GT(prevVersion) {
		return nil
	}

	return m.upgrade(ctx, rr)
}

// upgrade runs idempotent migrations when the platform version advances.
// Implement version-gated migrations here.
func (m *Module) upgrade(_ context.Context, rr *odhtypes.ReconciliationRequest) error {
	_ = rr
	// Add version-gated migrations here, e.g.:
	// if rr.Release.Version.GT(semver.MustParse("1.0.0")) { ... }
	return nil
}
