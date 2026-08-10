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

	infrav1 "github.com/opendatahub-io/opendatahub-operator/v2/api/infrastructure/v1alpha1"
)

// ---------------------------------------------------------------------------
// convertIntOrString
// ---------------------------------------------------------------------------

func TestConvertIntOrString_Int(t *testing.T) {
	g := NewWithT(t)
	q, err := convertIntOrString(intstr.FromInt32(2))
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(q.String()).To(Equal("2"))
}

func TestConvertIntOrString_String(t *testing.T) {
	g := NewWithT(t)
	q, err := convertIntOrString(intstr.FromString("500m"))
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(q.String()).To(Equal("500m"))
}

func TestConvertIntOrString_InvalidString(t *testing.T) {
	g := NewWithT(t)
	_, err := convertIntOrString(intstr.FromString("not-a-quantity"))
	g.Expect(err).To(HaveOccurred())
}

// ---------------------------------------------------------------------------
// validateNotebookContainers
// ---------------------------------------------------------------------------

func newNotebook(name string, containerNames ...string) *unstructured.Unstructured {
	nb := &unstructured.Unstructured{Object: map[string]any{}}
	nb.SetName(name)
	containers := make([]any, len(containerNames))
	for i, n := range containerNames {
		containers[i] = map[string]any{"name": n}
	}
	_ = unstructured.SetNestedSlice(nb.Object, containers,
		notebookWorkloadConfig.containersPath...)
	return nb
}

func TestValidateNotebookContainers_SingleContainer(t *testing.T) {
	g := NewWithT(t)
	nb := newNotebook("my-notebook", "any-name")
	g.Expect(validateNotebookContainers(nb)).To(Succeed())
}

func TestValidateNotebookContainers_MultipleMatchingName(t *testing.T) {
	g := NewWithT(t)
	nb := newNotebook("my-notebook", "my-notebook", "sidecar")
	g.Expect(validateNotebookContainers(nb)).To(Succeed())
}

func TestValidateNotebookContainers_MultipleNoMatch(t *testing.T) {
	g := NewWithT(t)
	nb := newNotebook("my-notebook", "other", "sidecar")
	err := validateNotebookContainers(nb)
	g.Expect(err).To(HaveOccurred())
	var noMatch *noMatchingContainerError
	g.Expect(err).To(BeAssignableToTypeOf(noMatch))
}

// ---------------------------------------------------------------------------
// applyIdentifiers
// ---------------------------------------------------------------------------

func TestApplyIdentifiers_SetsRequestsAndLimits(t *testing.T) {
	g := NewWithT(t)
	container := map[string]any{"name": "main"}
	identifiers := []infrav1.HardwareIdentifier{
		{Identifier: "cpu", DefaultCount: intstr.FromString("2")},
		{Identifier: "nvidia.com/gpu", DefaultCount: intstr.FromInt32(1)},
	}
	g.Expect(applyIdentifiers(container, identifiers)).To(Succeed())

	resMap := container["resources"].(map[string]any)
	requests := resMap["requests"].(map[string]any)
	limits := resMap["limits"].(map[string]any)
	g.Expect(requests["cpu"]).To(Equal("2"))
	g.Expect(limits["cpu"]).To(Equal("2"))
	g.Expect(requests["nvidia.com/gpu"]).To(Equal("1"))
	g.Expect(limits["nvidia.com/gpu"]).To(Equal("1"))
}

func TestApplyIdentifiers_SkipsExistingRequests(t *testing.T) {
	g := NewWithT(t)
	container := map[string]any{
		"name": "main",
		"resources": map[string]any{
			"requests": map[string]any{"cpu": "500m"},
		},
	}
	identifiers := []infrav1.HardwareIdentifier{
		{Identifier: "cpu", DefaultCount: intstr.FromString("2")},
	}
	g.Expect(applyIdentifiers(container, identifiers)).To(Succeed())

	resMap := container["resources"].(map[string]any)
	requests := resMap["requests"].(map[string]any)
	g.Expect(requests["cpu"]).To(Equal("500m"), "existing value must be preserved")
}

// ---------------------------------------------------------------------------
// notebookMainIndices
// ---------------------------------------------------------------------------

func TestNotebookMainIndices_SingleContainer(t *testing.T) {
	g := NewWithT(t)
	nb := newNotebook("my-notebook", "any")
	containers, _, _ := unstructured.NestedSlice(nb.Object, notebookWorkloadConfig.containersPath...)
	g.Expect(notebookMainIndices(nb, containers)).To(Equal([]int{0}))
}

func TestNotebookMainIndices_MatchesByName(t *testing.T) {
	g := NewWithT(t)
	nb := newNotebook("my-notebook", "other", "my-notebook")
	containers, _, _ := unstructured.NestedSlice(nb.Object, notebookWorkloadConfig.containersPath...)
	g.Expect(notebookMainIndices(nb, containers)).To(Equal([]int{1}))
}

func TestNotebookMainIndices_NoMatch(t *testing.T) {
	g := NewWithT(t)
	nb := newNotebook("my-notebook", "c1", "c2")
	containers, _, _ := unstructured.NestedSlice(nb.Object, notebookWorkloadConfig.containersPath...)
	g.Expect(notebookMainIndices(nb, containers)).To(BeEmpty())
}

// ---------------------------------------------------------------------------
// applyHWPToNotebook — scheduling
// ---------------------------------------------------------------------------

func TestApplyHWPToNotebook_SetsKueueLabel(t *testing.T) {
	g := NewWithT(t)
	nb := newNotebook("my-notebook", "nb")
	nb.SetLabels(map[string]string{})

	hwp := &infrav1.HardwareProfile{
		Spec: infrav1.HardwareProfileSpec{
			SchedulingSpec: &infrav1.SchedulingSpec{
				Kueue: &infrav1.KueueSchedulingSpec{LocalQueueName: "my-queue"},
			},
		},
	}

	warnings, err := applyHWPToNotebook(context.Background(), nb, hwp, false)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(warnings).To(BeEmpty())
	g.Expect(nb.GetLabels()["kueue.x-k8s.io/queue-name"]).To(Equal("my-queue"))
}

func TestApplyHWPToNotebook_WarnOnKueueOverwrite(t *testing.T) {
	g := NewWithT(t)
	nb := newNotebook("my-notebook", "nb")
	nb.SetLabels(map[string]string{"kueue.x-k8s.io/queue-name": "old-queue"})

	hwp := &infrav1.HardwareProfile{
		Spec: infrav1.HardwareProfileSpec{
			SchedulingSpec: &infrav1.SchedulingSpec{
				Kueue: &infrav1.KueueSchedulingSpec{LocalQueueName: "new-queue"},
			},
		},
	}

	warnings, err := applyHWPToNotebook(context.Background(), nb, hwp, false)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(warnings).To(HaveLen(1))
	g.Expect(warnings[0]).To(ContainSubstring("overwritten"))
}

func TestApplyHWPToNotebook_ClearsSchedulingOnProfileChange(t *testing.T) {
	g := NewWithT(t)
	nb := newNotebook("my-notebook", "nb")
	nb.SetLabels(map[string]string{"kueue.x-k8s.io/queue-name": "old"})
	_ = unstructured.SetNestedStringMap(nb.Object,
		map[string]string{"gpu": "true"}, notebookWorkloadConfig.nodeSelectorPath...)

	hwp := &infrav1.HardwareProfile{Spec: infrav1.HardwareProfileSpec{}}
	_, err := applyHWPToNotebook(context.Background(), nb, hwp, true)
	g.Expect(err).NotTo(HaveOccurred())

	g.Expect(nb.GetLabels()).NotTo(HaveKey("kueue.x-k8s.io/queue-name"))
	_, found, _ := unstructured.NestedStringMap(nb.Object, notebookWorkloadConfig.nodeSelectorPath...)
	g.Expect(found).To(BeFalse())
}

// ---------------------------------------------------------------------------
// mergeNodeSelector
// ---------------------------------------------------------------------------

func TestMergeNodeSelector_AddsNew(t *testing.T) {
	g := NewWithT(t)
	nb := newNotebook("nb")
	merged, warnings, err := mergeNodeSelector(nb, map[string]string{"zone": "us-east"}, "hwp")
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(warnings).To(BeEmpty())
	g.Expect(merged["zone"]).To(Equal("us-east"))
}

func TestMergeNodeSelector_WarnOnOverwrite(t *testing.T) {
	g := NewWithT(t)
	nb := newNotebook("nb")
	_ = unstructured.SetNestedStringMap(nb.Object, map[string]string{"zone": "us-west"}, notebookWorkloadConfig.nodeSelectorPath...)
	_, warnings, err := mergeNodeSelector(nb, map[string]string{"zone": "us-east"}, "hwp")
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(warnings).To(HaveLen(1))
}

// ---------------------------------------------------------------------------
// tolerationKey
// ---------------------------------------------------------------------------

func TestTolerationKey_Uniqueness(t *testing.T) {
	g := NewWithT(t)
	t1 := tolerationKey(map[string]any{"key": "k", "operator": "Equal", "value": "v", "effect": "NoSchedule"})
	t2 := tolerationKey(map[string]any{"key": "k", "operator": "Equal", "value": "v2", "effect": "NoSchedule"})
	g.Expect(t1).NotTo(Equal(t2))
}
