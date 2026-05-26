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

package trainer

import (
	"context"
	"fmt"

	componentApi "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-trainer-operator/api/components/v1alpha1"
	odhtypes "github.com/opendatahub-io/opendatahub-operator/v2/pkg/controller/types"
)

func (m *Module) upgradeIfNeeded(ctx context.Context, rr *odhtypes.ReconciliationRequest) error {
	obj, ok := rr.Instance.(*componentApi.Trainer)
	if !ok {
		return fmt.Errorf("instance is not a Trainer")
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

func (m *Module) upgrade(_ context.Context, prev componentApi.ModuleStatus, rr *odhtypes.ReconciliationRequest) error {
	_ = prev
	_ = rr

	return nil
}
