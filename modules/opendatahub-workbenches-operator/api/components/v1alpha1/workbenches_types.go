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
	ofVersion "github.com/operator-framework/api/pkg/lib/version"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// WorkbenchesComponentName is the component name used in labels and status.
	WorkbenchesComponentName = "workbenches"

	// WorkbenchesInstanceName is the singleton instance name enforced by the CEL rule below.
	WorkbenchesInstanceName = "default-workbenches"

	// WorkbenchesKind is the Kubernetes kind string.
	WorkbenchesKind = "Workbenches"
)

type Platform string

// Release reports the operator version and platform.
type Release struct {
	Name    Platform                  `json:"name,omitempty"`
	Version ofVersion.OperatorVersion `json:"version,omitempty"`
}

// Compile-time interface assertions.
var (
	_ common.PlatformObject = (*Workbenches)(nil)
)

// WorkbenchesSpec defines the desired state of Workbenches.
type WorkbenchesSpec struct {
	WorkbenchesCommonSpec `json:",inline"`
}

// WorkbenchesCommonStatus defines the shared observed state of Workbenches.
type WorkbenchesCommonStatus struct {
	common.ComponentReleaseStatus `json:",inline"`
	WorkbenchNamespace            string `json:"workbenchNamespace,omitempty"`
}

// WorkbenchesStatus defines the observed state of Workbenches.
type WorkbenchesStatus struct {
	common.Status           `json:",inline"`
	WorkbenchesCommonStatus `json:",inline"`

	// Release reports the operator version and platform.
	Release Release `json:"release,omitempty"`
}

// DeepCopy returns a deep copy of WorkbenchesCommonStatus.
func (in *WorkbenchesCommonStatus) DeepCopy() *WorkbenchesCommonStatus {
	if in == nil {
		return nil
	}
	out := new(WorkbenchesCommonStatus)
	*out = *in
	return out
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:validation:XValidation:rule="self.metadata.name == 'default-workbenches'",message="Workbenches name must be default-workbenches"
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`,description="Ready"
// +kubebuilder:printcolumn:name="Reason",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].reason`,description="Reason"
// +kubebuilder:printcolumn:name="Version",type=string,JSONPath=`.status.release.version`,description="Module Version"

// Workbenches is the Schema for the workbenches API.
type Workbenches struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   WorkbenchesSpec   `json:"spec,omitempty"`
	Status WorkbenchesStatus `json:"status,omitempty"`
}

func (c *Workbenches) GetStatus() *common.Status {
	return &c.Status.Status
}

func (c *Workbenches) GetConditions() []common.Condition {
	return c.Status.GetConditions()
}

func (c *Workbenches) SetConditions(conditions []common.Condition) {
	c.Status.SetConditions(conditions)
}

func (c *Workbenches) GetReleaseStatus() *common.ComponentReleaseStatus {
	return &c.Status.ComponentReleaseStatus
}

func (c *Workbenches) SetReleaseStatus(status common.ComponentReleaseStatus) {
	c.Status.ComponentReleaseStatus = status
}

// +kubebuilder:object:root=true

// WorkbenchesList contains a list of Workbenches.
type WorkbenchesList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Workbenches `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Workbenches{}, &WorkbenchesList{})
}
