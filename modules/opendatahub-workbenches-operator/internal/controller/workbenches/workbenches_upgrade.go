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

package workbenches

import (
	"context"
	"fmt"

	componentApi "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-workbenches-operator/api/components/v1alpha1"
	odhtypes "github.com/opendatahub-io/opendatahub-operator/v2/pkg/controller/types"
)

// upgradeIfNeeded detects module or platform version advances and runs idempotent migrations.
func (m *Module) upgradeIfNeeded(ctx context.Context, rr *odhtypes.ReconciliationRequest) error {
	obj, ok := rr.Instance.(*componentApi.Workbenches)
	if !ok {
		return fmt.Errorf("instance is not a Workbenches")
	}

	prev := obj.Status.Module

	moduleVersionChanged := !prev.Version.IsZero() && m.version.GT(prev.Version)
	platformVersionChanged := !prev.Platform.Version.IsZero() &&
		componentApi.SemVer(rr.Release.Version.String()).GT(prev.Platform.Version)

	if !moduleVersionChanged && !platformVersionChanged {
		return nil
	}

	return m.upgrade(ctx, prev, rr)
}

// upgrade runs idempotent migrations when the module version advances or the platform version changes.
// It migrates AcceleratorProfile and container-size annotations on Notebooks to HardwareProfile
// annotations, and creates the corresponding HardwareProfile CRs when they do not yet exist.
func (m *Module) upgrade(ctx context.Context, _ componentApi.ModuleStatus, rr *odhtypes.ReconciliationRequest) error {
	return migrateHardwareProfilesForNotebooks(ctx, rr.Client, m.cfg.ApplicationsNamespace, rr.ManifestsBasePath)
}
