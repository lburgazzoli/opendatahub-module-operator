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

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	fwtypes "github.com/opendatahub-io/odh-platform-utilities/framework/controller/types"

	componentApi "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-workbenches-operator/api/components/v1alpha1"
	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-workbenches-operator/pkg/releases"
)

const upgradeEventReasonStarted = "UpgradeStarted"

// upgradeIfNeeded compares the desired platform version from the release with
// the last applied version recorded in status.releases, and runs idempotent
// migrations when the version advances.
func (m *Module) upgradeIfNeeded(ctx context.Context, rr *fwtypes.ReconciliationRequest) error {
	log := logf.FromContext(ctx)
	obj, ok := rr.Instance.(*componentApi.Workbenches)
	if !ok {
		return fmt.Errorf("instance is not a Workbenches")
	}

	prev, _ := releases.Get(obj.GetReleaseStatus(), releases.Platform)

	prevVersion, err := releases.ParseVersion(prev.Version)
	if err != nil {
		return fmt.Errorf("parsing previous platform version: %w", err)
	}

	if !rr.Release.Version.GT(prevVersion) {
		return nil
	}

	message := fmt.Sprintf(
		"Upgrade started: applied %s -> desired %s",
		prevVersion.String(),
		rr.Release.Version.String(),
	)
	if err := m.recordModuleUpgradeEvent(ctx, rr.Client, obj, upgradeEventReasonStarted, corev1.EventTypeNormal, message); err != nil {
		return fmt.Errorf("recording upgrade started event: %w", err)
	}
	log.Info("Upgrade triggered", "module", obj.GetName(), "message", message)

	return m.upgrade(ctx, rr)
}

// upgrade runs idempotent migrations when the platform version advances.
// It migrates AcceleratorProfile and container-size annotations on Notebooks to HardwareProfile
// annotations, and creates the corresponding HardwareProfile CRs when they do not yet exist.
func (m *Module) upgrade(ctx context.Context, rr *fwtypes.ReconciliationRequest) error {
	return m.migrateHardwareProfilesForNotebooks(ctx, rr.Client)
}

func (m *Module) recordModuleUpgradeEvent(
	ctx context.Context,
	writer client.Client,
	obj *componentApi.Workbenches,
	reason string,
	eventType string,
	message string,
) error {
	now := metav1.NewTime(metav1.Now().Time)
	event := &corev1.Event{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: obj.GetName() + "-",
			Namespace:    m.cfg.ApplicationsNamespace,
		},
		InvolvedObject: corev1.ObjectReference{
			APIVersion: componentApi.GroupVersion.String(),
			Kind:       componentApi.WorkbenchesKind,
			Name:       obj.GetName(),
			Namespace:  m.cfg.ApplicationsNamespace,
			UID:        obj.GetUID(),
		},
		Reason:              reason,
		Message:             message,
		Type:                eventType,
		FirstTimestamp:      now,
		LastTimestamp:       now,
		Count:               1,
		Source:              corev1.EventSource{Component: upgradeEventSourceComponent},
		ReportingController: upgradeEventSourceComponent,
		ReportingInstance:   upgradeEventSourceComponent,
	}
	return writer.Create(ctx, event)
}
