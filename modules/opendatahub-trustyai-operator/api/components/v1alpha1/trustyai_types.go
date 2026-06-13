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
	TrustyAIComponentName = "trustyai"
	TrustyAIInstanceName  = "default-trustyai"
	TrustyAIKind          = "TrustyAI"

	TrustyAIResource = "trustyais"
	TrustyAICRDName  = TrustyAIResource + "." + GroupName
)

// EvalPermission controls whether an evaluation capability is allowed.
// +kubebuilder:validation:Enum=Allow;Deny
type EvalPermission string

const (
	EvalPermissionAllow EvalPermission = "Allow"
	EvalPermissionDeny  EvalPermission = "Deny"
)

// TrustyAILMEvalSpec defines LMEval-specific evaluation configuration.
type TrustyAILMEvalSpec struct {
	// PermitCodeExecution controls whether LMEval jobs may execute arbitrary code.
	// +kubebuilder:default=Deny
	// +optional
	PermitCodeExecution EvalPermission `json:"permitCodeExecution,omitempty"`

	// PermitOnline controls whether LMEval jobs may access the internet.
	// +kubebuilder:default=Deny
	// +optional
	PermitOnline EvalPermission `json:"permitOnline,omitempty"`
}

// TrustyAIEvalSpec defines evaluation configuration for TrustyAI.
type TrustyAIEvalSpec struct {
	// LMEval holds configuration for model evaluations.
	// +optional
	LMEval TrustyAILMEvalSpec `json:"lmeval,omitempty"`
}

// TrustyAISpec defines the desired state of TrustyAI.
type TrustyAISpec struct {
	// MCPGuardrailsMode enables the mcp-guardrails overlay when set to true.
	// +optional
	MCPGuardrailsMode bool `json:"mcpGuardrailsMode,omitempty"`

	// Eval configures evaluation capabilities.
	// +optional
	Eval TrustyAIEvalSpec `json:"eval,omitempty"`
}

type Platform string

// Release reports the operator version and platform.
type Release struct {
	Name    Platform                  `json:"name,omitempty"`
	Version ofVersion.OperatorVersion `json:"version,omitempty"`
}

// TrustyAIStatus defines the observed state of TrustyAI.
type TrustyAIStatus struct {
	common.Status                 `json:",inline"`
	common.ComponentReleaseStatus `json:",inline"`

	// Release reports the operator version and platform.
	Release Release `json:"release,omitempty"`
}

// Compile-time interface assertion.
var _ common.PlatformObject = (*TrustyAI)(nil)

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:validation:XValidation:rule="self.metadata.name == 'default-trustyai'",message="TrustyAI name must be default-trustyai"
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`,description="Ready"
// +kubebuilder:printcolumn:name="Reason",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].reason`,description="Reason"
// +kubebuilder:printcolumn:name="Version",type=string,JSONPath=`.status.release.version`,description="Module Version"

// TrustyAI is the Schema for the trustyais API.
type TrustyAI struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   TrustyAISpec   `json:"spec,omitempty"`
	Status TrustyAIStatus `json:"status,omitempty"`
}

func (c *TrustyAI) GetStatus() *common.Status         { return &c.Status.Status }
func (c *TrustyAI) GetConditions() []common.Condition { return c.Status.GetConditions() }
func (c *TrustyAI) SetConditions(conditions []common.Condition) {
	c.Status.SetConditions(conditions)
}
func (c *TrustyAI) GetReleaseStatus() *common.ComponentReleaseStatus {
	return &c.Status.ComponentReleaseStatus
}
func (c *TrustyAI) SetReleaseStatus(status common.ComponentReleaseStatus) {
	c.Status.ComponentReleaseStatus = status
}

// +kubebuilder:object:root=true

// TrustyAIList contains a list of TrustyAI.
type TrustyAIList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []TrustyAI `json:"items"`
}

func init() {
	SchemeBuilder.Register(&TrustyAI{}, &TrustyAIList{})
}
