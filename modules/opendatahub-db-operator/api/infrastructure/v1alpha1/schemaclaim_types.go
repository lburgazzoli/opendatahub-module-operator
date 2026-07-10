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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

const (
	// SchemaClaimKind is the Kubernetes kind string.
	SchemaClaimKind = "SchemaClaim"

	SchemaClaimResource = "schemaclaims"
	SchemaClaimCRDName  = SchemaClaimResource + "." + GroupName
)

// Compile-time interface assertion -- required because SchemaClaim is
// reconciled via the generic reconciler.ReconcilerFor[T api.PlatformObject]
// builder (docs/plan.md §6), same as every other module's CRD types.
var _ common.PlatformObject = (*SchemaClaim)(nil)

// SchemaConnectionStatus is the resolved connection surfaced to consumers
// once a SchemaClaim is Provisioned (docs/plan.md §5). Embeds the fields
// shared with DatabaseConnectionStatus (SecretRef/Host/Port) and adds
// Database/Schema, both required for the same reason: never legitimately
// empty once Connection itself is populated.
type SchemaConnectionStatus struct {
	ConnectionStatus `json:",inline"`

	// +kubebuilder:validation:Required
	Database string `json:"database"`

	// +kubebuilder:validation:Required
	Schema string `json:"schema"`
}

// SchemaClaimSpec defines the desired state of SchemaClaim.
type SchemaClaimSpec struct {
	// Provider selects the DatabaseProvider to provision against.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="provider is immutable once set"
	Provider ProviderRef `json:"provider"`

	// SecretName overrides the name of the credentials Secret projected in the
	// claim namespace. If omitted, the claim name is used. Immutable once set so
	// updates never strand credentials under an old Secret name.
	//
	// +optional
	// +kubebuilder:validation:MaxLength=253
	// +kubebuilder:validation:Pattern=`^[a-z0-9]([-a-z0-9.]*[a-z0-9])?$`
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="secretName is immutable once set"
	SecretName string `json:"secretName,omitempty"`

	// Schema is an optional override for the schema name. If omitted, it
	// defaults to "${namespace}_${name}", sanitized to a valid PostgreSQL
	// identifier (task-06). Immutable once set: changing it mid-life implies
	// a different claim, not an update to this one -- if this proves too
	// strict for a legitimate day-2 use case, revisit rather than silently
	// dropping the rule.
	//
	// +optional
	// +kubebuilder:validation:MaxLength=63
	// +kubebuilder:validation:Pattern=`^[a-z][a-z0-9_]*$`
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="schema is immutable once set"
	Schema string `json:"schema,omitempty"`

	// Access is the privilege level granted to the provisioned user.
	// +kubebuilder:validation:Enum=ReadWrite;ReadOnly
	// +kubebuilder:default=ReadWrite
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="access is immutable once set"
	Access AccessMode `json:"access,omitempty"`

	// DeletionPolicy governs schema+data lifecycle on claim deletion.
	// +kubebuilder:validation:Enum=Retain;Delete
	// +kubebuilder:default=Retain
	DeletionPolicy DeletionPolicy `json:"deletionPolicy,omitempty"`
}

// SchemaClaimStatus defines the observed state of SchemaClaim. Phase (human-
// readable summary only -- consumers must gate on conditions[type=Provisioned],
// never on Phase) comes from the embedded common.Status, not a redundant
// field here.
type SchemaClaimStatus struct {
	common.Status                 `json:",inline"`
	common.ComponentReleaseStatus `json:",inline"`

	// Schema is the actual resolved schema name -- always populated, whether
	// from spec.schema or the "${namespace}_${name}" default.
	Schema string `json:"schema,omitempty"`

	// Connection is the resolved connection surface once Provisioned,
	// including the effective credentials Secret name.
	// +optional
	Connection SchemaConnectionStatus `json:"connection,omitempty"`

	// Provider is the single DatabaseProvider ultimately selected when
	// spec.provider.selector matched more than one candidate (highest
	// selection-priority annotation, ties broken alphabetically by name --
	// docs/plan.md §6). Only the winner is surfaced: a claim binds to exactly
	// one provider, so there's nothing to gain from also listing the
	// candidates that lost.
	// +optional
	Provider string `json:"provider,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced
// +kubebuilder:printcolumn:name="Schema",type=string,JSONPath=`.status.schema`,description="Resolved schema"
// +kubebuilder:printcolumn:name="Provisioned",type=string,JSONPath=`.status.conditions[?(@.type=="Provisioned")].status`,description="Provisioned"

// SchemaClaim is the Schema for the schemaclaims API.
type SchemaClaim struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SchemaClaimSpec   `json:"spec,omitempty"`
	Status SchemaClaimStatus `json:"status,omitempty"`
}

func (c *SchemaClaim) GetStatus() *common.Status {
	return &c.Status.Status
}

func (c *SchemaClaim) GetConditions() []common.Condition {
	return c.Status.GetConditions()
}

func (c *SchemaClaim) SetConditions(conditions []common.Condition) {
	c.Status.SetConditions(conditions)
}

func (c *SchemaClaim) GetReleaseStatus() *common.ComponentReleaseStatus {
	return &c.Status.ComponentReleaseStatus
}

func (c *SchemaClaim) SetReleaseStatus(status common.ComponentReleaseStatus) {
	c.Status.ComponentReleaseStatus = status
}

// +kubebuilder:object:root=true

// SchemaClaimList contains a list of SchemaClaim.
type SchemaClaimList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SchemaClaim `json:"items"`
}

func init() {
	SchemeBuilder.Register(func(s *runtime.Scheme) error {
		s.AddKnownTypes(SchemeGroupVersion, &SchemaClaim{}, &SchemaClaimList{})
		return nil
	})
}
