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
	operatorv1 "github.com/openshift/api/operator/v1"
	ofVersion "github.com/operator-framework/api/pkg/lib/version"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	DataSciencePipelinesComponentName = "datasciencepipelines"
	DataSciencePipelinesInstanceName  = "default-" + DataSciencePipelinesComponentName
	DataSciencePipelinesKind          = "DataSciencePipelines"
	DataSciencePipelinesResource      = "datasciencepipelines"
	DataSciencePipelinesCRDName       = DataSciencePipelinesResource + "." + GroupName
	// AIPipelinesKind is the user-facing v2 alias in the monolith API.
	AIPipelinesKind = "AIPipelines"
)

type Platform string

// Release reports the operator version and platform.
type Release struct {
	Name    Platform                  `json:"name,omitempty"`
	Version ofVersion.OperatorVersion `json:"version,omitempty"`
}

var _ fwapi.PlatformObject = (*DataSciencePipelines)(nil)

type DataSciencePipelinesSpec struct {
	DataSciencePipelinesCommonSpec `json:",inline"`
}

type ArgoWorkflowsControllersSpec struct {
	// Set to "Managed" to let the module manage the bundled Argo Workflows
	// controllers, or "Removed" to leave them unmanaged.
	// +kubebuilder:validation:Enum=Managed;Removed
	// +kubebuilder:default=Managed
	ManagementState operatorv1.ManagementState `json:"managementState,omitempty"`
}

type DataSciencePipelinesCommonSpec struct {
	ArgoWorkflowsControllers *ArgoWorkflowsControllersSpec `json:"argoWorkflowsControllers,omitempty"`
}

type DataSciencePipelinesCommonStatus struct {
	common.ComponentReleaseStatus `json:",inline"`
}

type DataSciencePipelinesStatus struct {
	fwapi.Status                     `json:",inline"`
	DataSciencePipelinesCommonStatus `json:",inline"`

	// Release reports the operator version and platform.
	Release Release `json:"release,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:validation:XValidation:rule="self.metadata.name == 'default-datasciencepipelines'",message="DataSciencePipelines name must be default-datasciencepipelines"
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`,description="Ready"
// +kubebuilder:printcolumn:name="Reason",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].reason`,description="Reason"
// +kubebuilder:printcolumn:name="Version",type=string,JSONPath=`.status.release.version`,description="Module Version"
type DataSciencePipelines struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DataSciencePipelinesSpec   `json:"spec,omitempty"`
	Status DataSciencePipelinesStatus `json:"status,omitempty"`
}

func (c *DataSciencePipelines) GetStatus() *fwapi.Status {
	return &c.Status.Status
}

func (c *DataSciencePipelines) GetConditions() []fwapi.Condition {
	return c.Status.GetConditions()
}

func (c *DataSciencePipelines) SetConditions(conditions []fwapi.Condition) {
	c.Status.SetConditions(conditions)
}

func (c *DataSciencePipelines) GetReleaseStatus() *common.ComponentReleaseStatus {
	return &c.Status.ComponentReleaseStatus
}

func (c *DataSciencePipelines) SetReleaseStatus(status common.ComponentReleaseStatus) {
	c.Status.ComponentReleaseStatus = status
}

// +kubebuilder:object:root=true
type DataSciencePipelinesList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DataSciencePipelines `json:"items"`
}

func init() {
	SchemeBuilder.Register(&DataSciencePipelines{}, &DataSciencePipelinesList{})
}
