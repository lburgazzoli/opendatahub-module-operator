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

	ConditionReady    = "Ready"
	ConditionUpToDate = "UpToDate"

	ReasonAdminAckRequired = "AdminAckRequired"
)

var _ common.PlatformObject = (*Platform)(nil)

// PlatformSpec defines the desired state of Platform.
type PlatformSpec struct {
	// Modules lists the enabled module names (e.g. "ray", "spark").
	Modules []string `json:"modules,omitempty"`
}

// PlatformStatus defines the observed state of Platform.
type PlatformStatus struct {
	common.Status `json:",inline"`

	// Distribution reports the current converged distribution and the desired
	// target distribution.
	Distribution DistributionStatus `json:"distribution,omitempty"`

	// Runlevel is the current runlevel being processed.
	Runlevel int `json:"runlevel,omitempty"`

	// Modules is a per-module status summary, sorted by runlevel then name.
	Modules []ModuleStatusSummary `json:"modules,omitempty"`
}

// Distribution identifies a platform distribution name/version pair.
type Distribution struct {
	// Name is the distribution name (e.g. "OpenDataHub", "RHODS").
	Name string `json:"name,omitempty"`

	// Version is the distribution version.
	Version string `json:"version,omitempty"`
}

// DistributionStatus tracks the current and target distribution state.
type DistributionStatus struct {
	Current Distribution `json:"current,omitempty"`
	Target  Distribution `json:"target,omitempty"`
}

// ModuleStatusSummary holds a per-module status summary.
type ModuleStatusSummary struct {
	Name         string       `json:"name"`
	Runlevel     int          `json:"runlevel"`
	Distribution Distribution `json:"distribution,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:validation:XValidation:rule="self.metadata.name == 'default-platform'",message="Platform name must be default-platform"
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`,description="Ready"
// +kubebuilder:printcolumn:name="Runlevel",type=integer,JSONPath=`.status.runlevel`,description="Current Runlevel"
// +kubebuilder:printcolumn:name="Current",type=string,JSONPath=`.status.distribution.current.version`,description="Current Distribution Version"
// +kubebuilder:printcolumn:name="Target",type=string,JSONPath=`.status.distribution.target.version`,description="Target Distribution Version"

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
