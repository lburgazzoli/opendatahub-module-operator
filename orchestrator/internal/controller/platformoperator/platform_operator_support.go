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

package platformoperator

import (
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	configApi "github.com/lburgazzoli/opendatahub-module-operator/orchestrator/api/config/v1alpha1"
)

// FlattenValues flattens nested maps into dot-separated keys.
// e.g. {"a": {"b": "c"}} → {"a.b": "c"}
func FlattenValues(m map[string]any) map[string]string {
	result := make(map[string]string)
	flattenRecursive("", m, result)
	return result
}

func flattenRecursive(prefix string, m map[string]any, result map[string]string) {
	for k, v := range m {
		key := k
		if prefix != "" {
			key = prefix + "." + k
		}

		switch val := v.(type) {
		case map[string]any:
			flattenRecursive(key, val, result)
		default:
			result[key] = fmt.Sprintf("%v", val)
		}
	}
}

func ToResourceRefs(resources []unstructured.Unstructured) []configApi.ResourceRef {
	refs := make([]configApi.ResourceRef, 0, len(resources))
	for i := range resources {
		refs = append(refs, configApi.ResourceRef{
			APIVersion: resources[i].GetAPIVersion(),
			Kind:       resources[i].GetKind(),
			Namespace:  resources[i].GetNamespace(),
			Name:       resources[i].GetName(),
		})
	}
	return refs
}
