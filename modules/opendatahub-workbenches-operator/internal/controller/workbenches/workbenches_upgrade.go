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

	componentApi "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-workbenches-operator/api/components/v1alpha1"
	odhtypes "github.com/opendatahub-io/opendatahub-operator/v2/pkg/controller/types"
)

const upgradeEventReasonStarted = "UpgradeStarted"

// upgradeIfNeeded compares the desired versions from config/build metadata with
// the last applied versions recorded in status, and runs idempotent migrations
// when either desired version advances.
func (m *Module) upgradeIfNeeded(ctx context.Context, rr *odhtypes.ReconciliationRequest) error {
	log := logf.FromContext(ctx)
	obj, ok := rr.Instance.(*componentApi.Workbenches)
	if !ok {
		return fmt.Errorf("instance is not a Workbenches")
	}

	applied := obj.Status.Module
	desiredModuleVersion := m.version
	desiredPlatformVersion := componentApi.SemVer(rr.Release.Version.String())

	moduleVersionChanged := !applied.Version.IsZero() && desiredModuleVersion.GT(applied.Version)
	platformVersionChanged := !applied.Platform.Version.IsZero() &&
		desiredPlatformVersion.GT(applied.Platform.Version)

	log.Info(
		"Evaluated upgrade requirements",
		"module", obj.GetName(),
		"appliedModuleVersion", applied.Version.String(),
		"desiredModuleVersion", desiredModuleVersion.String(),
		"appliedPlatformVersion", applied.Platform.Version.String(),
		"desiredPlatformVersion", desiredPlatformVersion.String(),
		"moduleVersionChanged", moduleVersionChanged,
		"platformVersionChanged", platformVersionChanged,
	)

	if !moduleVersionChanged && !platformVersionChanged {
		log.Info("Upgrade not required", "module", obj.GetName())
		return nil
	}

	message := fmt.Sprintf(
		"Upgrade started: applied module %s -> desired %s, applied platform %s -> desired %s",
		applied.Version.String(),
		desiredModuleVersion.String(),
		applied.Platform.Version.String(),
		desiredPlatformVersion.String(),
	)
	if err := m.recordModuleUpgradeEvent(ctx, rr.Client, obj, upgradeEventReasonStarted, corev1.EventTypeNormal, message); err != nil {
		return fmt.Errorf("recording upgrade started event: %w", err)
	}
	log.Info("Upgrade triggered", "module", obj.GetName(), "message", message)

	return m.upgrade(ctx, applied, rr)
}

// upgrade runs idempotent migrations when the module version advances or the platform version changes.
// It migrates AcceleratorProfile and container-size annotations on Notebooks to HardwareProfile
// annotations, and creates the corresponding HardwareProfile CRs when they do not yet exist.
func (m *Module) upgrade(ctx context.Context, _ componentApi.ModuleStatus, rr *odhtypes.ReconciliationRequest) error {
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
