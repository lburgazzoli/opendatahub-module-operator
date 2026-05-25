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

package workbenches

import (
	"context"
	"testing"

	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// ---------------------------------------------------------------------------
// getContainerSizes
// ---------------------------------------------------------------------------

func odhConfigWithNotebookSizes(sizes []any) *unstructured.Unstructured {
	cfg := &unstructured.Unstructured{Object: map[string]any{}}
	_ = unstructured.SetNestedSlice(cfg.Object, sizes, "spec", "notebookSizes")
	return cfg
}

func TestGetContainerSizes_ParsesCorrectly(t *testing.T) {
	g := NewWithT(t)

	sizes := []any{
		map[string]any{
			"name": "Small",
			"resources": map[string]any{
				"requests": map[string]any{"cpu": "1", "memory": "2Gi"},
				"limits":   map[string]any{"cpu": "2", "memory": "4Gi"},
			},
		},
	}
	cfg := odhConfigWithNotebookSizes(sizes)
	result, err := getContainerSizes(cfg, "notebookSizes")
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(result).To(HaveLen(1))
	g.Expect(result[0].Name).To(Equal("Small"))
	g.Expect(result[0].Resources.Requests.Cpu).To(Equal("1"))
	g.Expect(result[0].Resources.Limits.Memory).To(Equal("4Gi"))
}

func TestGetContainerSizes_MissingSpec(t *testing.T) {
	g := NewWithT(t)
	cfg := &unstructured.Unstructured{Object: map[string]any{}}
	_, err := getContainerSizes(cfg, "notebookSizes")
	g.Expect(err).To(HaveOccurred())
}

func TestGetContainerSizes_MissingSizeType(t *testing.T) {
	g := NewWithT(t)
	cfg := &unstructured.Unstructured{Object: map[string]any{"spec": map[string]any{}}}
	result, err := getContainerSizes(cfg, "notebookSizes")
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(result).To(BeEmpty())
}

// ---------------------------------------------------------------------------
// containerSizeExists
// ---------------------------------------------------------------------------

func TestContainerSizeExists_Found(t *testing.T) {
	g := NewWithT(t)
	sizes := []ContainerSize{{Name: "Small"}, {Name: "Medium"}}
	g.Expect(containerSizeExists(sizes, "Small")).To(BeTrue())
}

func TestContainerSizeExists_NotFound(t *testing.T) {
	g := NewWithT(t)
	sizes := []ContainerSize{{Name: "Small"}}
	g.Expect(containerSizeExists(sizes, "Large")).To(BeFalse())
}

// ---------------------------------------------------------------------------
// getNotebooksOnlyToleration
// ---------------------------------------------------------------------------

func TestGetNotebooksOnlyToleration_Enabled(t *testing.T) {
	g := NewWithT(t)
	cfg := &unstructured.Unstructured{Object: map[string]any{
		"spec": map[string]any{
			"notebookController": map[string]any{
				"enabled": true,
				"notebookTolerationSettings": map[string]any{
					"enabled":  true,
					"key":      "notebooks-only",
					"operator": "Exists",
					"effect":   "NoSchedule",
				},
			},
		},
	}}
	tols, err := getNotebooksOnlyToleration(cfg)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(tols).To(HaveLen(1))
	g.Expect(tols[0].Key).To(Equal("notebooks-only"))
	g.Expect(string(tols[0].Effect)).To(Equal("NoSchedule"))
}

func TestGetNotebooksOnlyToleration_Disabled(t *testing.T) {
	g := NewWithT(t)
	cfg := &unstructured.Unstructured{Object: map[string]any{
		"spec": map[string]any{
			"notebookController": map[string]any{
				"enabled": false,
			},
		},
	}}
	tols, err := getNotebooksOnlyToleration(cfg)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(tols).To(BeNil())
}

// ---------------------------------------------------------------------------
// generateHWPFromContainerSize
// ---------------------------------------------------------------------------

func TestGenerateHWPFromContainerSize_NameFormat(t *testing.T) {
	g := NewWithT(t)
	size := ContainerSize{Name: "Large GPU"}
	size.Resources.Requests.Cpu = "4"
	size.Resources.Requests.Memory = "8Gi"
	size.Resources.Limits.Cpu = "8"
	size.Resources.Limits.Memory = "16Gi"

	hwp := generateHWPFromContainerSize(context.Background(), size, "notebooks", nil, "opendatahub")
	g.Expect(hwp.GetName()).To(Equal("containersize-large-gpu-notebooks"))
	g.Expect(hwp.GetNamespace()).To(Equal("opendatahub"))
	g.Expect(hwp.Spec.Identifiers).To(HaveLen(2))
	g.Expect(hwp.Spec.Identifiers[0].Identifier).To(Equal("cpu"))
	g.Expect(hwp.Spec.Identifiers[0].MinCount).To(Equal(intstr.FromString("4")))
	g.Expect(hwp.Spec.SchedulingSpec).To(BeNil())
}

// ---------------------------------------------------------------------------
// createHWPAnnotations
// ---------------------------------------------------------------------------

func TestCreateHWPAnnotations_VisibilityIsWorkbench(t *testing.T) {
	g := NewWithT(t)
	ann := createHWPAnnotations("notebooks", "My Profile", "desc", false)
	g.Expect(ann[upgradeAnnotationHWPVisibility]).To(Equal(upgradeFeatureVisibilityWorkbench))
	g.Expect(ann[upgradeAnnotationHWPDisplayName]).To(Equal("My Profile"))
	g.Expect(ann[upgradeAnnotationHWPDisabled]).To(Equal("false"))
}

func TestCreateHWPAnnotations_Disabled(t *testing.T) {
	g := NewWithT(t)
	ann := createHWPAnnotations("notebooks", "Off Profile", "", true)
	g.Expect(ann[upgradeAnnotationHWPDisabled]).To(Equal("true"))
}

// ---------------------------------------------------------------------------
// findCpuMemoryMinMaxFromSizes
// ---------------------------------------------------------------------------

func TestFindCpuMemoryMinMaxFromSizes_EmptyFallsToDefault(t *testing.T) {
	g := NewWithT(t)
	result, err := findCpuMemoryMinMaxFromSizes(nil)
	g.Expect(err).NotTo(HaveOccurred())
	// Empty input returns upgradeDefaultResourceLimits values.
	g.Expect(result["minCpu"]).To(Equal(upgradeDefaultResourceLimits["minCpu"]))
	g.Expect(result["minMemory"]).To(Equal(upgradeDefaultResourceLimits["minMemory"]))
}

func TestFindCpuMemoryMinMaxFromSizes_ExtractsFromFirstSize(t *testing.T) {
	g := NewWithT(t)
	size := ContainerSize{Name: "S"}
	size.Resources.Requests.Cpu = "500m"
	size.Resources.Requests.Memory = "1Gi"
	size.Resources.Limits.Cpu = "1"
	size.Resources.Limits.Memory = "2Gi"

	result, err := findCpuMemoryMinMaxFromSizes([]ContainerSize{size})
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(result["minCpu"]).To(Equal("500m"))
	g.Expect(result["minMemory"]).To(Equal("1Gi"))
}
