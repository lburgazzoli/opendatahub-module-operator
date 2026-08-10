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
	ofVersion "github.com/operator-framework/api/pkg/lib/version"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	TrainerComponentName = "trainer"
	TrainerInstanceName  = "default-" + TrainerComponentName
	TrainerKind          = "Trainer"

	TrainerResource = "trainers"
	TrainerCRDName  = TrainerResource + "." + GroupName
)

type Platform string

// Release reports the operator version and platform.
type Release struct {
	Name    Platform                  `json:"name,omitempty"`
	Version ofVersion.OperatorVersion `json:"version,omitempty"`
}

var _ fwapi.PlatformObject = (*Trainer)(nil)

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:validation:XValidation:rule="self.metadata.name == 'default-trainer'",message="Trainer name must be default-trainer"
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`,description="Ready"
// +kubebuilder:printcolumn:name="Reason",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].reason`,description="Reason"
// +kubebuilder:printcolumn:name="Version",type=string,JSONPath=`.status.release.version`,description="Module Version"

// Trainer is the Schema for the trainers API.
type Trainer struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   TrainerSpec   `json:"spec,omitempty"`
	Status TrainerStatus `json:"status,omitempty"`
}

// TrainerSpec defines the desired state of Trainer.
type TrainerSpec struct {
	TrainerCommonSpec `json:",inline"`
}

type TrainerCommonSpec struct{}

// TrainerStatus defines the observed state of Trainer.
type TrainerStatus struct {
	fwapi.Status                  `json:",inline"`
	common.ComponentReleaseStatus `json:",inline"`

	// Release reports the operator version and platform.
	Release Release `json:"release,omitempty"`
}

func (c *Trainer) GetStatus() *fwapi.Status {
	return &c.Status.Status
}

func (c *Trainer) GetConditions() []fwapi.Condition {
	return c.Status.GetConditions()
}

func (c *Trainer) SetConditions(conditions []fwapi.Condition) {
	c.Status.SetConditions(conditions)
}

func (c *Trainer) GetReleaseStatus() *common.ComponentReleaseStatus {
	return &c.Status.ComponentReleaseStatus
}

func (c *Trainer) SetReleaseStatus(status common.ComponentReleaseStatus) {
	c.Status.ComponentReleaseStatus = status
}

// +kubebuilder:object:root=true

// TrainerList contains a list of Trainer.
type TrainerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Trainer `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Trainer{}, &TrainerList{})
}
