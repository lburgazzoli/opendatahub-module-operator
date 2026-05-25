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
	"github.com/opendatahub-io/opendatahub-operator/v2/api/common"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// ModelRegistryComponentName is the component name used in labels and status.
	ModelRegistryComponentName = "modelregistry"

	// ModelRegistryInstanceName is the singleton instance name enforced by the CEL rule below.
	ModelRegistryInstanceName = "default-" + ModelRegistryComponentName

	// ModelRegistryKind is the Kubernetes kind string.
	ModelRegistryKind = "ModelRegistry"
)

// Compile-time interface assertion.
var _ common.PlatformObject = (*ModelRegistry)(nil)

// ModelRegistrySpec defines the desired state of ModelRegistry.
type ModelRegistrySpec struct {
	// model registry spec exposed to the operator
	ModelRegistryCommonSpec `json:",inline"`
	// Gateway configuration for model registry ingress.
	// +optional
	Gateway *common.GatewaySpec `json:"gateway,omitempty"`
}

// ModelRegistryCommonStatus defines the shared observed state of ModelRegistry.
type ModelRegistryCommonStatus struct {
	RegistriesNamespace string `json:"registriesNamespace,omitempty"`
}

// ModelRegistryStatus defines the observed state of ModelRegistry.
type ModelRegistryStatus struct {
	common.Status                 `json:",inline"`
	common.ComponentReleaseStatus `json:",inline"`
	ModelRegistryCommonStatus     `json:",inline"`

	// Module reports the module operator's runtime information.
	Module ModuleStatus `json:"module,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:validation:XValidation:rule="self.metadata.name == 'default-modelregistry'",message="ModelRegistry name must be default-modelregistry"
// +kubebuilder:validation:XValidation:rule="(oldSelf.spec.registriesNamespace == '') || (self.spec.registriesNamespace == oldSelf.spec.registriesNamespace)",message="RegistriesNamespace is immutable once set"
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`,description="Ready"
// +kubebuilder:printcolumn:name="Reason",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].reason`,description="Reason"
// +kubebuilder:printcolumn:name="Version",type=string,JSONPath=`.status.module.version`,description="Module Version"

// ModelRegistry is the Schema for the modelregistries API.
type ModelRegistry struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ModelRegistrySpec   `json:"spec,omitempty"`
	Status ModelRegistryStatus `json:"status,omitempty"`
}

func (c *ModelRegistry) GetStatus() *common.Status {
	return &c.Status.Status
}

func (c *ModelRegistry) GetConditions() []common.Condition {
	return c.Status.GetConditions()
}

func (c *ModelRegistry) SetConditions(conditions []common.Condition) {
	c.Status.SetConditions(conditions)
}

func (c *ModelRegistry) GetReleaseStatus() *[]common.ComponentRelease {
	return &c.Status.Releases
}

func (c *ModelRegistry) SetReleaseStatus(releases []common.ComponentRelease) {
	c.Status.Releases = releases
}

// +kubebuilder:object:root=true

// ModelRegistryList contains a list of ModelRegistry.
type ModelRegistryList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ModelRegistry `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ModelRegistry{}, &ModelRegistryList{})
}
