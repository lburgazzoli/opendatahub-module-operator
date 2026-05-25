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
	"testing"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// ---------------------------------------------------------------------------
// parseConnectionsAnnotation
// ---------------------------------------------------------------------------

func TestParseConnectionsAnnotation_Empty(t *testing.T) {
	g := NewWithT(t)
	refs, err := parseConnectionsAnnotation("")
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(refs).To(BeNil())
}

func TestParseConnectionsAnnotation_Single(t *testing.T) {
	g := NewWithT(t)
	refs, err := parseConnectionsAnnotation("my-ns/my-secret")
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(refs).To(HaveLen(1))
	g.Expect(refs[0]).To(Equal(corev1.SecretReference{Namespace: "my-ns", Name: "my-secret"}))
}

func TestParseConnectionsAnnotation_Multiple(t *testing.T) {
	g := NewWithT(t)
	refs, err := parseConnectionsAnnotation("ns1/s1, ns2/s2")
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(refs).To(HaveLen(2))
	g.Expect(refs[0].Name).To(Equal("s1"))
	g.Expect(refs[1].Name).To(Equal("s2"))
}

func TestParseConnectionsAnnotation_InvalidFormat(t *testing.T) {
	g := NewWithT(t)
	_, err := parseConnectionsAnnotation("just-a-name")
	g.Expect(err).To(HaveOccurred())
}

func TestParseConnectionsAnnotation_EmptyParts(t *testing.T) {
	g := NewWithT(t)
	_, err := parseConnectionsAnnotation("/secret")
	g.Expect(err).To(HaveOccurred())
}

// ---------------------------------------------------------------------------
// determineSecretActions
// ---------------------------------------------------------------------------

func TestDetermineSecretActions_NewSecret(t *testing.T) {
	g := NewWithT(t)
	actions := determineSecretActions(nil, []corev1.SecretReference{{Namespace: "ns", Name: "s"}})
	g.Expect(actions["ns/s"]).To(Equal("create"))
}

func TestDetermineSecretActions_RemovedSecret(t *testing.T) {
	g := NewWithT(t)
	old := []corev1.SecretReference{{Namespace: "ns", Name: "s"}}
	actions := determineSecretActions(old, nil)
	g.Expect(actions["ns/s"]).To(Equal("delete"))
}

func TestDetermineSecretActions_Unchanged(t *testing.T) {
	g := NewWithT(t)
	ref := []corev1.SecretReference{{Namespace: "ns", Name: "s"}}
	actions := determineSecretActions(ref, ref)
	g.Expect(actions).To(BeEmpty())
}

// ---------------------------------------------------------------------------
// performConnectionInjection
// ---------------------------------------------------------------------------

func newNotebookWithContainer(name string) *unstructured.Unstructured {
	nb := &unstructured.Unstructured{Object: map[string]any{}}
	nb.SetName(name)
	_ = unstructured.SetNestedSlice(nb.Object, []any{
		map[string]any{"name": name},
	}, "spec", "template", "spec", "containers")
	return nb
}

func TestPerformConnectionInjection_InjectsSecret(t *testing.T) {
	g := NewWithT(t)

	nb := newNotebookWithContainer("my-notebook")
	refs := []notebookSecretRef{{secret: corev1.SecretReference{Namespace: "ns", Name: "conn"}, action: "create"}}

	injected, result, err := performConnectionInjection(nb, refs)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(injected).To(BeTrue())

	containers, _, _ := unstructured.NestedSlice(result.Object, notebookContainersPath...)
	container := containers[0].(map[string]any)
	envFrom := container["envFrom"].([]any)
	g.Expect(envFrom).To(HaveLen(1))
	entry := envFrom[0].(map[string]any)
	g.Expect(entry["secretRef"].(map[string]any)["name"]).To(Equal("conn"))
}

func TestPerformConnectionInjection_RemovesSecret(t *testing.T) {
	g := NewWithT(t)

	nb := newNotebookWithContainer("my-notebook")
	// Pre-populate envFrom with existing secret reference.
	_ = unstructured.SetNestedSlice(nb.Object, []any{
		map[string]any{"name": "my-notebook", "envFrom": []any{
			map[string]any{"secretRef": map[string]any{"name": "conn"}},
		}},
	}, "spec", "template", "spec", "containers")

	refs := []notebookSecretRef{{secret: corev1.SecretReference{Namespace: "ns", Name: "conn"}, action: "delete"}}
	injected, result, err := performConnectionInjection(nb, refs)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(injected).To(BeTrue())

	containers, _, _ := unstructured.NestedSlice(result.Object, notebookContainersPath...)
	container := containers[0].(map[string]any)
	envFrom, _ := container["envFrom"].([]any)
	g.Expect(envFrom).To(BeEmpty())
}

func TestPerformConnectionInjection_NoContainers(t *testing.T) {
	g := NewWithT(t)
	nb := &unstructured.Unstructured{Object: map[string]any{}}
	_, _, err := performConnectionInjection(nb, []notebookSecretRef{{action: "create"}})
	g.Expect(err).To(HaveOccurred())
}
