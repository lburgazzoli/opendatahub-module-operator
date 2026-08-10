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
	OGXComponentName = "ogx"
	OGXInstanceName  = "default-ogx"
	OGXKind          = "OGX"
	OGXResource      = "ogxs"
	OGXCRDName       = OGXResource + "." + GroupName
)

type Platform string

// Release reports the operator version and platform.
type Release struct {
	Name    Platform                  `json:"name,omitempty"`
	Version ofVersion.OperatorVersion `json:"version,omitempty"`
}

// Compile-time interface assertion.
var _ fwapi.PlatformObject = (*OGX)(nil)

// OGXSpec defines the desired state of OGX.
type OGXSpec struct{}

// OGXStatus defines the observed state of OGX.
type OGXStatus struct {
	fwapi.Status                  `json:",inline"`
	common.ComponentReleaseStatus `json:",inline"`

	// Release reports the operator version and platform.
	Release Release `json:"release,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster,path=ogxs
// +kubebuilder:validation:XValidation:rule="self.metadata.name == 'default-ogx'",message="OGX name must be default-ogx"
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`,description="Ready"
// +kubebuilder:printcolumn:name="Reason",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].reason`,description="Reason"
// +kubebuilder:printcolumn:name="Version",type=string,JSONPath=`.status.release.version`,description="Module Version"

// OGX is the Schema for the ogxs API.
type OGX struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   OGXSpec   `json:"spec,omitempty"`
	Status OGXStatus `json:"status,omitempty"`
}

func (c *OGX) GetStatus() *fwapi.Status {
	return &c.Status.Status
}

func (c *OGX) GetConditions() []fwapi.Condition {
	return c.Status.GetConditions()
}

func (c *OGX) SetConditions(conditions []fwapi.Condition) {
	c.Status.SetConditions(conditions)
}

func (c *OGX) GetReleaseStatus() *common.ComponentReleaseStatus {
	return &c.Status.ComponentReleaseStatus
}

func (c *OGX) SetReleaseStatus(status common.ComponentReleaseStatus) {
	c.Status.ComponentReleaseStatus = status
}

// +kubebuilder:object:root=true

// OGXList contains a list of OGX.
type OGXList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []OGX `json:"items"`
}

func init() {
	SchemeBuilder.Register(&OGX{}, &OGXList{})
}
