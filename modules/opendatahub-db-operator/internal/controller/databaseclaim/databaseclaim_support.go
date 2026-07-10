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

package databaseclaim

import (
	"context"

	"sigs.k8s.io/controller-runtime/pkg/client"

	dbcontroller "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/pkg/controller"
)

const (
	// ConditionProvisioned is re-exported from pkg/controller so that action
	// code in this package can reference it locally without an extra import.
	ConditionProvisioned = dbcontroller.ConditionProvisioned

	FinalizerName = "infrastructure.opendatahub.io/databaseclaim-cleanup"
)

// withGrace delegates to the shared pkg/controller.WithGrace free function,
// binding this controller's Recorder and GracePeriod.
func (c *Controller) withGrace(ctx context.Context, obj client.Object, fn func(context.Context) error) error {
	return dbcontroller.WithGrace(ctx, c.Recorder, c.cfg.GracePeriod, obj, fn)
}
