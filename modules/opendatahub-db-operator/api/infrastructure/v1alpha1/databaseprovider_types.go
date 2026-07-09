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
	common "github.com/opendatahub-io/odh-platform-utilities/api/common"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

const (
	// DatabaseProviderKind is the Kubernetes kind string.
	DatabaseProviderKind = "DatabaseProvider"

	DatabaseProviderResource = "databaseproviders"
	DatabaseProviderCRDName  = DatabaseProviderResource + "." + GroupName
)

// Compile-time interface assertion -- required because DatabaseProvider is
// reconciled via the generic reconciler.ReconcilerFor[T api.PlatformObject]
// builder (docs/plan.md §6), same as every other module's CRD types.
var _ common.PlatformObject = (*DatabaseProvider)(nil)

// ProviderType selects which of DatabaseProviderSpec.External/Embedded is
// populated. Mutually exclusive with the other, enforced by CEL rules on
// DatabaseProviderSpec below (docs/plan.md §5).
type ProviderType string

const (
	// ProviderTypeExternal points at a database instance this service does
	// not own or manage the lifecycle of.
	ProviderTypeExternal ProviderType = "External"

	// ProviderTypeEmbedded is a controller-managed, single-instance
	// PostgreSQL convenience -- not a DBaaS (docs/plan.md §2, §7).
	ProviderTypeEmbedded ProviderType = "Embedded"
)

// ExternalProviderSpec points at an admin-managed PostgreSQL instance this
// service validates connectivity to but never owns the lifecycle of
// (docs/plan.md §6, task-04).
type ExternalProviderSpec struct {
	// ConnectionSecretRef points at a Secret holding admin-level connection
	// info, which may live in a different namespace than any claim (unlike
	// claim credential Secrets, which always live in the claim's own
	// namespace).
	// +kubebuilder:validation:Required
	ConnectionSecretRef corev1.SecretReference `json:"connectionSecretRef"`
}

// ProviderConnectionStatus is the non-secret connection surface for a
// DatabaseProvider. It intentionally excludes credentials and secret references.
type ProviderConnectionStatus struct {
	Host     string `json:"host,omitempty"`
	Port     int32  `json:"port,omitempty"`
	Database string `json:"database,omitempty"`
}

// StorageSpec configures the Embedded provider's PersistentVolumeClaim
// (docs/plan.md §7.3).
type StorageSpec struct {
	// Size is the requested PVC storage size.
	// +kubebuilder:validation:Required
	Size resource.Quantity `json:"size"`

	// StorageClassName is passed through to the PVC verbatim when set.
	// +optional
	StorageClassName *string `json:"storageClassName,omitempty"`
}

// EmbeddedProviderSpec configures a controller-owned, single-instance
// PostgreSQL StatefulSet (docs/plan.md §7). There is deliberately no image
// field -- letting admins point a platform-managed instance at an arbitrary
// image reopens supply-chain and support-surface problems; the image is
// resolved from Extensions via pkg/config compiled defaults (task-08).
type EmbeddedProviderSpec struct {
	// Namespace overrides where the embedded PostgreSQL resources are created.
	// When unset, the operator namespace is used.
	// +optional
	Namespace string `json:"namespace,omitempty"`

	// DeletionPolicy governs the instance's lifecycle when zero claims
	// reference this provider (docs/plan.md §7.7) -- distinct from a claim's
	// own per-schema/database deletionPolicy.
	// +kubebuilder:validation:Enum=Retain;Delete
	// +kubebuilder:default=Retain
	DeletionPolicy DeletionPolicy `json:"deletionPolicy,omitempty"`

	// Storage configures the instance's PersistentVolumeClaim.
	// +kubebuilder:validation:Required
	Storage StorageSpec `json:"storage"`

	// Resources are applied to the Postgres container.
	// +optional
	Resources corev1.ResourceRequirements `json:"resources,omitempty"`

	// Extensions lists PostgreSQL extension names to make available,
	// mapped to a built-in container image (task-08 §1) -- an unmapped
	// combination fails reconciliation with an actionable condition rather
	// than silently ignoring the request.
	// +optional
	// +kubebuilder:validation:items:Pattern=`^[a-z][a-z0-9_]*$`
	Extensions []string `json:"extensions,omitempty"`
}

// DatabaseProviderSpec defines the desired state of DatabaseProvider.
//
// +kubebuilder:validation:XValidation:rule="(has(self.external) ? 1 : 0) + (has(self.embedded) ? 1 : 0) == 1",message="exactly one of external or embedded must be set"
// +kubebuilder:validation:XValidation:rule="self.type == 'External' ? has(self.external) : has(self.embedded)",message="spec.type must match the set provider type"
type DatabaseProviderSpec struct {
	// Type selects which of External/Embedded below is populated.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Enum=External;Embedded
	Type ProviderType `json:"type"`

	// External configures a provider pointing at an admin-managed instance.
	// Mutually exclusive with Embedded.
	// +optional
	External *ExternalProviderSpec `json:"external,omitempty"`

	// Embedded configures a controller-owned instance. Mutually exclusive
	// with External.
	// +optional
	Embedded *EmbeddedProviderSpec `json:"embedded,omitempty"`
}

// DatabaseProviderStatus defines the observed state of DatabaseProvider.
// Unlike the two claim kinds, spec.md's status example has no phase field --
// common.Status's own Phase/Conditions/ObservedGeneration are sufficient.
type DatabaseProviderStatus struct {
	common.Status                 `json:",inline"`
	common.ComponentReleaseStatus `json:",inline"`

	// Connection is the non-secret connection surface for the provider.
	// +optional
	Connection ProviderConnectionStatus `json:"connection,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:printcolumn:name="Type",type=string,JSONPath=`.spec.type`,description="Provider type"
// +kubebuilder:printcolumn:name="Host",type=string,JSONPath=`.status.connection.host`,description="Resolved host"
// +kubebuilder:printcolumn:name="Database",type=string,JSONPath=`.status.connection.database`,description="Resolved admin database"
// +kubebuilder:printcolumn:name="Reachable",type=string,JSONPath=`.status.conditions[?(@.type=="Reachable")].status`,description="Reachable"

// DatabaseProvider is the Schema for the databaseproviders API.
type DatabaseProvider struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DatabaseProviderSpec   `json:"spec,omitempty"`
	Status DatabaseProviderStatus `json:"status,omitempty"`
}

func (c *DatabaseProvider) GetStatus() *common.Status {
	return &c.Status.Status
}

func (c *DatabaseProvider) GetConditions() []common.Condition {
	return c.Status.GetConditions()
}

func (c *DatabaseProvider) SetConditions(conditions []common.Condition) {
	c.Status.SetConditions(conditions)
}

func (c *DatabaseProvider) GetReleaseStatus() *common.ComponentReleaseStatus {
	return &c.Status.ComponentReleaseStatus
}

func (c *DatabaseProvider) SetReleaseStatus(status common.ComponentReleaseStatus) {
	c.Status.ComponentReleaseStatus = status
}

// +kubebuilder:object:root=true

// DatabaseProviderList contains a list of DatabaseProvider.
type DatabaseProviderList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DatabaseProvider `json:"items"`
}

func init() {
	SchemeBuilder.Register(func(s *runtime.Scheme) error {
		s.AddKnownTypes(SchemeGroupVersion, &DatabaseProvider{}, &DatabaseProviderList{})
		return nil
	})
}
