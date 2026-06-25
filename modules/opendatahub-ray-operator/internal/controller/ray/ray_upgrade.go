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

package ray

import (
	"context"
	"fmt"

	"github.com/blang/semver/v4"
	fwtypes "github.com/opendatahub-io/odh-platform-utilities/framework/controller/types"

	componentApi "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-ray-operator/api/components/v1alpha1"
	moduleconfig "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-ray-operator/pkg/config"
)

func (m *Module) upgradeIfNeeded(ctx context.Context, rr *fwtypes.ReconciliationRequest) error {
	obj, ok := rr.Instance.(*componentApi.Ray)
	if !ok {
		return fmt.Errorf("instance is not a Ray")
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

func (m *Module) upgrade(_ context.Context, rr *fwtypes.ReconciliationRequest) error {
	_ = rr

	return nil
}
