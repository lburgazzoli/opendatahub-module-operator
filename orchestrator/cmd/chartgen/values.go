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

	"github.com/opendatahub-io/operator-actions-framework/cluster/gvk"
)

const (
	defaultImageRepository = "ttl.sh/opendatahub-orchestrator"
	defaultImageTag        = "1h"
	defaultLimitsCPU       = "500m"
	defaultLimitsMemory    = "128Mi"
	defaultRequestsCPU     = "10m"
	defaultRequestsMemory  = "64Mi"

	resourceKeyLimits   = "limits"
	resourceKeyRequests = "requests"
	resourceKeyCPU      = "cpu"
	resourceKeyMemory   = "memory"
)

type Values struct {
	Image ImageSpec `json:"image"`

	Replicas int32 `json:"replicas" jsonschema:"default=1,minimum=1"`

	Resources ResourceSpec `json:"resources"`

	LeaderElect bool `json:"leaderElect" jsonschema:"default=true"`

	ServiceAccount ServiceAccountSpec `json:"serviceAccount"`

	ImagePullSecret string `json:"imagePullSecret,omitempty"`

	Config map[string]string `json:"config,omitempty"`
}

type ImageSpec struct {
	Repository string `json:"repository"`
	Tag        string `json:"tag"`
	FullRef    string `json:"fullRef,omitempty"`
}

type ResourceSpec struct {
	Limits   ResourceList `json:"limits,omitempty"`
	Requests ResourceList `json:"requests,omitempty"`
}

type ResourceList struct {
	CPU    string `json:"cpu,omitempty"`
	Memory string `json:"memory,omitempty"`
}

type ServiceAccountSpec struct {
	Name string `json:"name,omitempty"`

	Annotations map[string]string `json:"annotations,omitempty"`
}

func DefaultValues() Values {
	return Values{
		Image: ImageSpec{
			Repository: defaultImageRepository,
			Tag:        defaultImageTag,
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
		LeaderElect: true,
		Config: map[string]string{
			"charts-path":                        "/charts",
			"distribution.name":                  "OpenDataHub",
			"distribution.version":               "unknown",
			"controller.metrics.bind-address":    ":8080",
			"controller.health.bind-address":     ":8081",
			"controller.leader-election.enabled": "true",
			"controller.leader-election.id":      "opendatahub-orchestrator-lock",
			"controller.zap.level":               "info",
			"controller.zap.dev-mode":            "false",
			"controller.zap.encoder":             "",
			"controller.pprof.enabled":           "false",
		},
	}
}

func ExtractDefaults(resources []unstructured.Unstructured) Values {
	values := DefaultValues()

	for _, r := range resources {
		if r.GroupVersionKind() != gvk.Deployment {
			continue
		}

		containers, found, _ := unstructured.NestedSlice(r.Object, "spec", "template", "spec", "containers")
		if found && len(containers) > 0 {
			c, ok := containers[0].(map[string]any)
			if ok {
				if img, exists := c["image"].(string); exists {
					parts := splitImageTag(img)
					values.Image.Repository = parts[0]
					values.Image.Tag = parts[1]
				}

				if res, exists := c["resources"].(map[string]any); exists {
					values.Resources = extractResources(res)
				}
			}
		}

		replicas, found, _ := unstructured.NestedInt64(r.Object, "spec", "replicas")
		if found {
			values.Replicas = int32(replicas)
		}

		break
	}

	return values
}

func splitImageTag(image string) [2]string {
	if idx := len(image) - 1; idx > 0 {
		for i := idx; i >= 0; i-- {
			if image[i] == ':' {
				return [2]string{image[:i], image[i+1:]}
			}
			if image[i] == '/' {
				break
			}
		}
	}

	return [2]string{image, defaultImageTag}
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

func WriteValuesYAML(v Values, path string) error {
	data, err := yaml.Marshal(v)
	if err != nil {
		return fmt.Errorf("marshaling values: %w", err)
	}

	return os.WriteFile(path, data, 0o644)
}

func WriteValuesSchema(path string) error {
	reflector := &jsonschema.Reflector{}
	schema := reflector.Reflect(&Values{})

	data, err := json.MarshalIndent(schema, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling schema: %w", err)
	}

	return os.WriteFile(path, data, 0o644)
}
