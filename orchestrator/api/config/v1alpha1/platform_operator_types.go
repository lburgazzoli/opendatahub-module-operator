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

var _ common.PlatformObject = (*PlatformOperator)(nil)

// +kubebuilder:rbac:groups=config.opendatahub.io,resources=platformoperators,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=config.opendatahub.io,resources=platformoperators/status,verbs=get;update;patch

const (
	PlatformOperatorKind = "PlatformOperator"
)

// PlatformOperatorSpec is intentionally empty — this CR is managed
// exclusively by the orchestrator, not by users.
type PlatformOperatorSpec struct{}

// PlatformOperatorStatus tracks what the orchestrator deployed for this module.
type PlatformOperatorStatus struct {
	common.Status `json:",inline"`

	// Runlevel is the module's runlevel in the orchestration DAG.
	Runlevel int `json:"runlevel,omitempty"`

	// DeployedVersion is the module version reported by the module CR status.
	DeployedVersion string `json:"deployedVersion,omitempty"`

	// Chart describes the Helm chart used to render this module's resources.
	Chart ChartInfo `json:"chart,omitempty"`

	// Resources lists all resources deployed for this module.
	Resources []ResourceRef `json:"resources,omitempty"`

	// Config holds the config values merged into Values.config.
	Config map[string]string `json:"config,omitempty"`
}

// ChartInfo describes the Helm chart used to render module resources.
type ChartInfo struct {
	// Path is the location of the chart.
	Path string `json:"path"`

	// Name is the chart name from Chart.yaml.
	Name string `json:"name,omitempty"`

	// Version is the chart version from Chart.yaml.
	Version string `json:"version,omitempty"`

	// AppVersion is the chart appVersion from Chart.yaml.
	AppVersion string `json:"appVersion,omitempty"`
}

// ResourceRef identifies a deployed Kubernetes resource.
type ResourceRef struct {
	APIVersion string `json:"apiVersion"`
	Kind       string `json:"kind"`
	Namespace  string `json:"namespace,omitempty"`
	Name       string `json:"name"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster
// PlatformOperator is created per module (named after the module, e.g. "ray",
// "spark"). Tracks deployed resources for GC and drift detection.
// Managed exclusively by the orchestrator — not user-facing.
// Module resources use this CR as ownerRef so deleting it triggers
// Kubernetes garbage collection of all module resources.
type PlatformOperator struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PlatformOperatorSpec   `json:"spec,omitempty"`
	Status PlatformOperatorStatus `json:"status,omitempty"`
}

func (c *PlatformOperator) GetStatus() *common.Status {
	return &c.Status.Status
}

func (c *PlatformOperator) GetConditions() []common.Condition {
	return c.Status.GetConditions()
}

func (c *PlatformOperator) SetConditions(conditions []common.Condition) {
	c.Status.SetConditions(conditions)
}

func (c *PlatformOperator) GetReleaseStatus() *[]common.ComponentRelease {
	return nil
}

func (c *PlatformOperator) SetReleaseStatus(_ []common.ComponentRelease) {
}

// +kubebuilder:object:root=true

// PlatformOperatorList contains a list of PlatformOperator.
type PlatformOperatorList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []PlatformOperator `json:"items"`
}

func init() {
	SchemeBuilder.Register(&PlatformOperator{}, &PlatformOperatorList{})
}
