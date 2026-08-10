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
	fwapi "github.com/opendatahub-io/odh-platform-utilities/framework/api"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// SparkOperatorComponentName is the component name used in labels and status.
	SparkOperatorComponentName = "sparkoperator"

	// SparkOperatorInstanceName is the singleton instance name enforced by the CEL rule below.
	SparkOperatorInstanceName = "default-sparkoperator"

	// SparkOperatorKind is the Kubernetes kind string.
	SparkOperatorKind = "SparkOperator"

	SparkOperatorResource = "sparkoperators"
	SparkOperatorCRDName  = SparkOperatorResource + "." + GroupName
)

// Compile-time interface assertion.
var _ fwapi.PlatformObject = (*SparkOperator)(nil)

// SparkOperatorSpec defines the desired state of SparkOperator.
type SparkOperatorSpec struct{}

// SparkOperatorStatus defines the observed state of SparkOperator.
type SparkOperatorStatus struct {
	fwapi.Status                  `json:",inline"`
	common.ComponentReleaseStatus `json:",inline"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:validation:XValidation:rule="self.metadata.name == 'default-sparkoperator'",message="SparkOperator name must be default-sparkoperator"
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`,description="Ready"
// +kubebuilder:printcolumn:name="Reason",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].reason`,description="Reason"
// +kubebuilder:printcolumn:name="Version",type=string,JSONPath=`.status.releases[?(@.name=="platform")].version`,description="Module Version"

// SparkOperator is the Schema for the sparkoperators API.
type SparkOperator struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SparkOperatorSpec   `json:"spec,omitempty"`
	Status SparkOperatorStatus `json:"status,omitempty"`
}

func (c *SparkOperator) GetStatus() *fwapi.Status {
	return &c.Status.Status
}

func (c *SparkOperator) GetConditions() []fwapi.Condition {
	return c.Status.GetConditions()
}

func (c *SparkOperator) SetConditions(conditions []fwapi.Condition) {
	c.Status.SetConditions(conditions)
}

func (c *SparkOperator) GetReleaseStatus() *common.ComponentReleaseStatus {
	return &c.Status.ComponentReleaseStatus
}

func (c *SparkOperator) SetReleaseStatus(status common.ComponentReleaseStatus) {
	c.Status.ComponentReleaseStatus = status
}

// +kubebuilder:object:root=true

// SparkOperatorList contains a list of SparkOperator.
type SparkOperatorList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SparkOperator `json:"items"`
}

func init() {
	SchemeBuilder.Register(&SparkOperator{}, &SparkOperatorList{})
}
