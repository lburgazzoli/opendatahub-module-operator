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
	// MyModuleComponentName is the component name used in labels and status.
	MyModuleComponentName = "mymodule"

	// MyModuleInstanceName is the singleton instance name enforced by the CEL rule below.
	MyModuleInstanceName = "default-mymodule"

	// MyModuleKind is the Kubernetes kind string.
	MyModuleKind = "MyModule"

	MyModuleResource = "mymodules"
	MyModuleCRDName  = MyModuleResource + "." + GroupName
)

type Platform string

// Release reports the operator version and platform.
type Release struct {
	Name    Platform                  `json:"name,omitempty"`
	Version ofVersion.OperatorVersion `json:"version,omitempty"`
}

// Compile-time interface assertions.
var (
	_ common.PlatformObject = (*MyModule)(nil)
)

// MyModuleSpec defines the desired state of MyModule.
type MyModuleSpec struct {
	// TODO: add module-specific configuration fields
}

// MyModuleStatus defines the observed state of MyModule.
type MyModuleStatus struct {
	common.Status                 `json:",inline"`
	common.ComponentReleaseStatus `json:",inline"`

	// Release reports the operator version and platform.
	Release Release `json:"release,omitempty"`

	// ConfigValues holds the parsed controller ConfigMap entries for observability.
	ConfigValues map[string]string `json:"configValues,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:validation:XValidation:rule="self.metadata.name == 'default-mymodule'",message="MyModule name must be default-mymodule"
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`,description="Ready"
// +kubebuilder:printcolumn:name="Reason",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].reason`,description="Reason"
// +kubebuilder:printcolumn:name="Version",type=string,JSONPath=`.status.release.version`,description="Module Version"

// MyModule is the Schema for the mymodules API.
type MyModule struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   MyModuleSpec   `json:"spec,omitempty"`
	Status MyModuleStatus `json:"status,omitempty"`
}

func (c *MyModule) GetStatus() *common.Status {
	return &c.Status.Status
}

func (c *MyModule) GetConditions() []common.Condition {
	return c.Status.GetConditions()
}

func (c *MyModule) SetConditions(conditions []common.Condition) {
	c.Status.SetConditions(conditions)
}

func (c *MyModule) GetReleaseStatus() *common.ComponentReleaseStatus {
	return &c.Status.ComponentReleaseStatus
}

func (c *MyModule) SetReleaseStatus(status common.ComponentReleaseStatus) {
	c.Status.ComponentReleaseStatus = status
}

// +kubebuilder:object:root=true

// MyModuleList contains a list of MyModule.
type MyModuleList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []MyModule `json:"items"`
}

func init() {
	SchemeBuilder.Register(&MyModule{}, &MyModuleList{})
}
