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

package trustyai

import (
	"context"
	"fmt"

	componentApi "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-trustyai-operator/api/components/v1alpha1"
	"github.com/opendatahub-io/opendatahub-operator/v2/api/common"
	odhtypes "github.com/opendatahub-io/opendatahub-operator/v2/pkg/controller/types"
)

func (m *Module) upgradeIfNeeded(ctx context.Context, rr *odhtypes.ReconciliationRequest) error {
	obj, ok := rr.Instance.(*componentApi.TrustyAI)
	if !ok {
		return fmt.Errorf("instance is not a TrustyAI")
	}

	prev := obj.Status.Release

	if prev.Version.String() == "" || prev.Version.String() == "0.0.0" {
		return nil
	}

	if !rr.Release.Version.GT(prev.Version.Version) {
		return nil
	}

	return m.upgrade(ctx, prev, rr)
}

func (m *Module) upgrade(_ context.Context, prev common.Release, rr *odhtypes.ReconciliationRequest) error {
	_ = prev
	_ = rr
	return nil
}
