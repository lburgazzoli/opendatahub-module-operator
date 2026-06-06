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
	PlatformInstanceName = "default-platform"
	PlatformKind         = "Platform"
)

var _ common.PlatformObject = (*Platform)(nil)

// PlatformSpec defines the desired state of Platform.
type PlatformSpec struct {
	// Modules lists the enabled module names (e.g. "ray", "spark").
	// Each entry must match a registered module in the orchestrator.
	Modules []string `json:"modules,omitempty"`
}

// PlatformStatus defines the observed state of Platform.
type PlatformStatus struct {
	common.Status `json:",inline"`

	// Version is the current platform version.
	Version string `json:"version,omitempty"`

	// Mode is the current operational mode.
	// +kubebuilder:validation:Enum=upgrade;reconcile
	Mode OperationalMode `json:"mode,omitempty"`

	// CurrentRunlevel is the runlevel being processed during upgrade mode.
	CurrentRunlevel *int `json:"currentRunlevel,omitempty"`

	// Modules is a per-module status summary.
	Modules []ModuleStatusSummary `json:"modules,omitempty"`
}

// OperationalMode represents the orchestrator's current mode.
// +kubebuilder:validation:Enum="";upgrade;reconcile
type OperationalMode string

const (
	ModeUnknown   OperationalMode = ""
	ModeUpgrade   OperationalMode = "upgrade"
	ModeReconcile OperationalMode = "reconcile"
)

// OperationalState holds the current orchestration mode and runlevel.
type OperationalState struct {
	Mode     OperationalMode `json:"mode,omitempty"`
	Runlevel int             `json:"runlevel,omitempty"`
}

// ModuleStatusSummary holds a per-module status summary.
type ModuleStatusSummary struct {
	Name     string `json:"name"`
	Runlevel int    `json:"runlevel"`
	Version  string `json:"version,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:validation:XValidation:rule="self.metadata.name == 'default-platform'",message="Platform name must be default-platform"
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`,description="Ready"
// +kubebuilder:printcolumn:name="Mode",type=string,JSONPath=`.status.mode`,description="Operational Mode"
// +kubebuilder:printcolumn:name="Runlevel",type=integer,JSONPath=`.status.currentRunlevel`,description="Current Runlevel"
// +kubebuilder:printcolumn:name="Version",type=string,JSONPath=`.status.version`,description="Platform Version"

// Platform is the Schema for the platforms API.
type Platform struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PlatformSpec   `json:"spec,omitempty"`
	Status PlatformStatus `json:"status,omitempty"`
}

func (c *Platform) GetStatus() *common.Status {
	return &c.Status.Status
}

func (c *Platform) GetConditions() []common.Condition {
	return c.Status.GetConditions()
}

func (c *Platform) SetConditions(conditions []common.Condition) {
	c.Status.SetConditions(conditions)
}

func (c *Platform) GetReleaseStatus() *[]common.ComponentRelease {
	return nil
}

func (c *Platform) SetReleaseStatus(_ []common.ComponentRelease) {
}

// +kubebuilder:object:root=true

// PlatformList contains a list of Platform.
type PlatformList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Platform `json:"items"`
}
