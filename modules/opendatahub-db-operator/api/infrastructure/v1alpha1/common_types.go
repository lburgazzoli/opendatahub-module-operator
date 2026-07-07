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

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// AccessMode is the set of privileges granted to a claim's provisioned user
// (docs/plan.md §5).
type AccessMode string

const (
	// AccessModeReadWrite grants full read/write privileges (the default).
	AccessModeReadWrite AccessMode = "ReadWrite"

	// AccessModeReadOnly grants read-only privileges.
	AccessModeReadOnly AccessMode = "ReadOnly"
)

// DeletionPolicy governs schema/data lifecycle on claim or provider deletion
// (docs/plan.md §5, §7.7).
type DeletionPolicy string

const (
	// DeletionPolicyRetain leaves the underlying schema/data/instance intact
	// on deletion (the default).
	DeletionPolicyRetain DeletionPolicy = "Retain"

	// DeletionPolicyDelete drops the underlying schema/data/instance on
	// deletion.
	DeletionPolicyDelete DeletionPolicy = "Delete"
)

// ProviderRef selects a DatabaseProvider by exact name or by a label
// selector matched against DatabaseProvider capability labels -- mutually
// exclusive, enforced by the CEL rule below (mirrors PVC.spec.storageClassName
// vs PVC.spec.selector, docs/plan.md §1).
//
// +kubebuilder:validation:XValidation:rule="(has(self.name) ? 1 : 0) + (has(self.selector) ? 1 : 0) == 1",message="exactly one of name or selector must be set"
type ProviderRef struct {
	// Name is the exact DatabaseProvider name to bind to.
	// +optional
	Name string `json:"name,omitempty"`

	// Selector matches against DatabaseProvider capability labels. When
	// multiple providers match, the one with the highest
	// db.infrastructure.opendatahub.io/selection-priority annotation wins,
	// ties broken alphabetically by name (docs/plan.md §6).
	// +optional
	Selector *metav1.LabelSelector `json:"selector,omitempty"`
}

// ConnectionStatus is the common connection surface shared by both claim
// kinds' status.connection (docs/plan.md §5). SchemaConnectionStatus and
// DatabaseConnectionStatus embed this and add only the fields that
// legitimately differ (Schema is meaningless for a DatabaseClaim -- see the
// comment on DatabaseConnectionStatus). All three fields here are required:
// once a claim's connection is populated at all, none of them can
// legitimately be empty -- they're written atomically by the reconciler when
// it sets Provisioned: True, never partially.
type ConnectionStatus struct {
	// SecretRef always equals the claim's own metadata.name and lives in the
	// claim's own namespace -- no independent naming scheme to look up.
	// +kubebuilder:validation:Required
	SecretRef corev1.LocalObjectReference `json:"secretRef"`

	// +kubebuilder:validation:Required
	Host string `json:"host"`

	// +kubebuilder:validation:Required
	Port int32 `json:"port"`
}
