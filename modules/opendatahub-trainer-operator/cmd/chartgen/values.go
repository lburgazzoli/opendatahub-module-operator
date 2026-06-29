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

package chartgen

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/invopop/jsonschema"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/yaml"

	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-trainer-operator/pkg/resources/gvk"
)

const (
	defaultImageRef        = "controller:latest"
	defaultImagePullPolicy = "Always"
	defaultLimitsCPU       = "500m"
	defaultLimitsMemory    = "128Mi"
	defaultRequestsCPU     = "10m"
	defaultRequestsMemory  = "64Mi"

	resourceKeyLimits   = "limits"
	resourceKeyRequests = "requests"
	resourceKeyCPU      = "cpu"
	resourceKeyMemory   = "memory"
)

// Values defines the Helm chart values structure.
type Values struct {
	// Operator configures the operator Deployment itself.
	Operator OperatorSpec `json:"operator"`

	// ServiceAccount configures the chart-managed ServiceAccount.
	ServiceAccount ServiceAccountSpec `json:"serviceAccount"`

	// ImagePullSecrets configures pod image pull secrets.
	ImagePullSecrets []ImagePullSecretRef `json:"imagePullSecrets"`

	// Platform values are written explicitly into the controller ConfigMap.
	Platform PlatformSpec `json:"platform"`

	// Config provides additional controller configuration entries that are
	// merged into the controller ConfigMap.
	Config map[string]string `json:"config"`
}

// OperatorSpec configures the operator Deployment.
type OperatorSpec struct {
	// Image configures the container image for the operator.
	Image ImageSpec `json:"image"`

	// Replicas is the number of operator pod replicas.
	Replicas int32 `json:"replicas" jsonschema:"default=1,minimum=1"`

	// Resources configures CPU and memory requests/limits for the operator.
	Resources ResourceSpec `json:"resources"`
}

// ImageSpec describes a container image.
type ImageSpec struct {
	Ref        string `json:"ref"`
	PullPolicy string `json:"pullPolicy" jsonschema:"enum=Always,enum=IfNotPresent,enum=Never"`
}

// PlatformSpec contains explicit platform handshake values written to the ConfigMap.
type PlatformSpec struct {
	Type    string `json:"type" jsonschema:"default=OpenDataHub"`
	Version string `json:"version" jsonschema:"default="`
}

// ResourceSpec mirrors corev1.ResourceRequirements but with simpler
// serialization for Helm values.
type ResourceSpec struct {
	Limits   ResourceList `json:"limits,omitempty"`
	Requests ResourceList `json:"requests,omitempty"`
}

// ResourceList maps resource names to quantities.
type ResourceList struct {
	CPU    string `json:"cpu,omitempty"`
	Memory string `json:"memory,omitempty"`
}

// ServiceAccountSpec configures the operator's ServiceAccount.
type ServiceAccountSpec struct {
	// Name overrides the ServiceAccount name (defaults to release fullname).
	Name string `json:"name,omitempty"`

	// Annotations are additional annotations on the ServiceAccount.
	Annotations map[string]string `json:"annotations,omitempty"`
}

// ImagePullSecretRef matches the pod imagePullSecrets item shape.
type ImagePullSecretRef struct {
	Name string `json:"name"`
}

// DefaultValues returns a Values instance with sensible defaults.
func DefaultValues() Values {
	return Values{
		Operator: OperatorSpec{
			Image: ImageSpec{
				Ref:        defaultImageRef,
				PullPolicy: defaultImagePullPolicy,
			},
			Replicas: 1,
			Resources: ResourceSpec{
				Limits: ResourceList{
					CPU:    defaultLimitsCPU,
					Memory: defaultLimitsMemory,
				},
				Requests: ResourceList{
					CPU:    defaultRequestsCPU,
					Memory: defaultRequestsMemory,
				},
			},
		},
		ServiceAccount:   ServiceAccountSpec{},
		ImagePullSecrets: []ImagePullSecretRef{},
		Platform: PlatformSpec{
			Type:    "OpenDataHub",
			Version: "",
		},
		Config: map[string]string{},
	}
}

// ExtractDefaults extracts default values from the kustomize resources,
// primarily from the Deployment spec.
func ExtractDefaults(resources []unstructured.Unstructured) Values {
	values := DefaultValues()

	for _, r := range resources {
		if r.GroupVersionKind() != gvk.Deployment {
			continue
		}

		// Extract image
		containers, found, _ := unstructured.NestedSlice(r.Object, "spec", "template", "spec", "containers")
		if found && len(containers) > 0 {
			c, ok := containers[0].(map[string]any)
			if ok {
				if img, exists := c["image"].(string); exists {
					values.Operator.Image.Ref = img
				}

				if policy, exists := c["imagePullPolicy"].(string); exists {
					values.Operator.Image.PullPolicy = policy
				}

				if res, exists := c["resources"].(map[string]any); exists {
					values.Operator.Resources = extractResources(res)
				}
			}
		}

		// Extract replicas
		replicas, found, _ := unstructured.NestedInt64(r.Object, "spec", "replicas")
		if found {
			values.Operator.Replicas = int32(replicas)
		}

		break // Only process the first Deployment
	}

	return values
}

func extractResources(res map[string]any) ResourceSpec {
	spec := ResourceSpec{}

	if limits, ok := res[resourceKeyLimits].(map[string]any); ok {
		if cpu, ok := limits[resourceKeyCPU].(string); ok {
			spec.Limits.CPU = cpu
		} else if cpu, ok := limits[resourceKeyCPU]; ok {
			spec.Limits.CPU = resource.NewMilliQuantity(int64(cpu.(float64)*1000), resource.DecimalSI).String()
		}

		if mem, ok := limits[resourceKeyMemory].(string); ok {
			spec.Limits.Memory = mem
		}
	}

	if requests, ok := res[resourceKeyRequests].(map[string]any); ok {
		if cpu, ok := requests[resourceKeyCPU].(string); ok {
			spec.Requests.CPU = cpu
		} else if cpu, ok := requests[resourceKeyCPU]; ok {
			spec.Requests.CPU = resource.NewMilliQuantity(int64(cpu.(float64)*1000), resource.DecimalSI).String()
		}

		if mem, ok := requests[resourceKeyMemory].(string); ok {
			spec.Requests.Memory = mem
		}
	}

	return spec
}

// WriteValuesYAML writes the values to a YAML file.
func WriteValuesYAML(v Values, path string) error {
	data, err := yaml.Marshal(v)
	if err != nil {
		return fmt.Errorf("marshaling values: %w", err)
	}

	return os.WriteFile(path, data, 0o644)
}

// WriteValuesSchema generates a JSON Schema from the Values struct and
// writes it to the given path.
func WriteValuesSchema(path string) error {
	reflector := &jsonschema.Reflector{}
	schema := reflector.Reflect(&Values{})

	data, err := json.MarshalIndent(schema, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling schema: %w", err)
	}

	return os.WriteFile(path, data, 0o644)
}
