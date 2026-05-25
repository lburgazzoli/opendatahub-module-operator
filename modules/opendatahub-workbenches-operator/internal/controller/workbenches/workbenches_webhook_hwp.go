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
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"slices"
	"strconv"

	admissionv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
	k8serr "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	infrav1 "github.com/opendatahub-io/opendatahub-operator/v2/api/infrastructure/v1"
	"github.com/opendatahub-io/opendatahub-operator/v2/pkg/cluster"
	"github.com/opendatahub-io/opendatahub-operator/v2/pkg/resources"
	webhookutils "github.com/opendatahub-io/opendatahub-operator/v2/pkg/webhook"

	gvk "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-workbenches-operator/pkg/resources/gvk"
)

// +kubebuilder:webhook:path=/mutate-hardware-profile,mutating=true,failurePolicy=fail,groups=kubeflow.org,resources=notebooks,verbs=create;update,versions=v1,name=hardwareprofile-notebook-injector.opendatahub.io,sideEffects=None,admissionReviewVersions=v1

const (
	hwpNameAnnotation      = "opendatahub.io/hardware-profile-name"
	hwpNamespaceAnnotation = "opendatahub.io/hardware-profile-namespace"
)

var notebookWorkloadConfig = struct {
	containersPath   []string
	nodeSelectorPath []string
	tolerationsPath  []string
}{
	containersPath:   []string{"spec", "template", "spec", "containers"},
	nodeSelectorPath: []string{"spec", "template", "spec", "nodeSelector"},
	tolerationsPath:  []string{"spec", "template", "spec", "tolerations"},
}

// handleHardwareProfileNotebook is the hardware profile mutating webhook for Notebooks only.
func (m *Module) handleHardwareProfileNotebook(ctx context.Context, req admission.Request) admission.Response {
	if m.decoder == nil {
		return admission.Errored(http.StatusInternalServerError, errors.New("webhook decoder not initialized"))
	}

	expectedGVK := schema.GroupVersionKind{
		Group:   gvk.Notebook.Group,
		Version: gvk.Notebook.Version,
		Kind:    gvk.Notebook.Kind,
	}
	reqGVK := schema.GroupVersionKind{Group: req.Kind.Group, Version: req.Kind.Version, Kind: req.Kind.Kind}
	if !slices.Contains([]schema.GroupVersionKind{expectedGVK}, reqGVK) {
		return admission.Errored(http.StatusBadRequest, fmt.Errorf("unexpected kind: %s", req.Kind.Kind))
	}

	obj, err := webhookutils.DecodeUnstructured(m.decoder, req)
	if err != nil {
		return admission.Errored(http.StatusInternalServerError, err)
	}
	if !obj.GetDeletionTimestamp().IsZero() {
		return admission.Allowed("object marked for deletion")
	}

	switch req.Operation {
	case admissionv1.Create, admissionv1.Update:
		return m.injectHardwareProfile(ctx, &req, obj)
	default:
		return admission.Allowed(fmt.Sprintf("operation %s allowed", req.Operation))
	}
}

func (m *Module) injectHardwareProfile(ctx context.Context, req *admission.Request, obj *unstructured.Unstructured) admission.Response {
	log := logf.FromContext(ctx)

	profileName := resources.GetAnnotation(obj, hwpNameAnnotation)
	if profileName == "" {
		if req.Operation == admissionv1.Update {
			if resp := m.handleHWPRemoval(ctx, req, obj); resp != nil {
				return *resp
			}
		}
		return admission.Allowed("no hardware profile annotation")
	}

	profileNS := resources.GetAnnotation(obj, hwpNamespaceAnnotation)
	if profileNS == "" {
		profileNS = obj.GetNamespace()
	}
	if profileNS == "" {
		return admission.Errored(http.StatusBadRequest, errors.New("cannot determine hardware profile namespace"))
	}

	hwp := &infrav1.HardwareProfile{}
	switch err := m.apiReader.Get(ctx, types.NamespacedName{Name: profileName, Namespace: profileNS}, hwp); {
	case err == nil:
	case k8serr.IsNotFound(err):
		return admission.Errored(http.StatusBadRequest, fmt.Errorf("hardware profile %q not found in %q", profileName, profileNS))
	default:
		return admission.Errored(http.StatusInternalServerError, err)
	}

	if resources.GetAnnotation(obj, hwpNamespaceAnnotation) == "" {
		resources.SetAnnotation(obj, hwpNamespaceAnnotation, profileNS)
	}

	if err := validateNotebookContainers(obj); err != nil {
		var noMatch *noMatchingContainerError
		if errors.As(err, &noMatch) {
			m.emitContainerWarning(ctx, obj, noMatch, hwp.Name)
			warn := fmt.Sprintf("hardware profile %q not applied: %s", hwp.Name, noMatch.Error())
			raw, marshalErr := json.Marshal(obj)
			if marshalErr != nil {
				log.Error(marshalErr, "marshal failed after container validation")
				resp := admission.Allowed("admitted despite marshal failure")
				resp.Warnings = []string{warn}
				return resp
			}
			resp := admission.PatchResponseFromRaw(req.Object.Raw, raw)
			resp.Warnings = []string{warn}
			return resp
		}
	}

	profileChanged := detectProfileChange(req, profileName, profileNS)
	warnings, err := applyHWPToNotebook(ctx, obj, hwp, profileChanged)
	if err != nil {
		log.Error(err, "failed to apply hardware profile")
		return admission.Errored(http.StatusInternalServerError, err)
	}

	raw, err := json.Marshal(obj)
	if err != nil {
		return admission.Errored(http.StatusInternalServerError, err)
	}
	resp := admission.PatchResponseFromRaw(req.Object.Raw, raw)
	resp.Warnings = warnings
	return resp
}

type noMatchingContainerError struct {
	kind, name, namespace, expected string
}

func (e *noMatchingContainerError) Error() string {
	return fmt.Sprintf("no matching main container in %s %s/%s: expected %q", e.kind, e.namespace, e.name, e.expected)
}

func validateNotebookContainers(obj *unstructured.Unstructured) error {
	containers, found, err := unstructured.NestedSlice(obj.Object, notebookWorkloadConfig.containersPath...)
	if err != nil {
		return fmt.Errorf("accessing containers: %w", err)
	}
	if !found || len(containers) <= 1 {
		return nil
	}
	expected := obj.GetName()
	for _, c := range containers {
		if m, ok := c.(map[string]any); ok {
			if n, _ := m["name"].(string); n == expected {
				return nil
			}
		}
	}
	return &noMatchingContainerError{kind: obj.GetKind(), name: obj.GetName(), namespace: obj.GetNamespace(), expected: expected}
}

func (m *Module) emitContainerWarning(ctx context.Context, obj *unstructured.Unstructured, err *noMatchingContainerError, hwpName string) {
	log := logf.FromContext(ctx)
	if m.webhookClient == nil {
		return
	}
	event := &corev1.Event{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s.hwp-container-name-mismatch", obj.GetName()),
			Namespace: obj.GetNamespace(),
		},
		InvolvedObject: corev1.ObjectReference{
			Kind: obj.GetKind(), Namespace: obj.GetNamespace(), Name: obj.GetName(),
			UID: obj.GetUID(), APIVersion: obj.GetAPIVersion(),
		},
		Reason:         "ContainerNameMismatch",
		Message:        fmt.Sprintf("Hardware profile %q not applied: %s", hwpName, err.Error()),
		Source:         corev1.EventSource{Component: "workbenches-hwp-webhook"},
		FirstTimestamp: metav1.Now(),
		LastTimestamp:  metav1.Now(),
		Count:          1,
		Type:           corev1.EventTypeWarning,
	}
	if createErr := m.webhookClient.Create(ctx, event); createErr != nil {
		log.Info("failed to create container warning event (non-blocking)", "error", createErr)
	}
}

func detectProfileChange(req *admission.Request, newName, newNS string) bool {
	if req.Operation == admissionv1.Create || req.OldObject.Raw == nil {
		return false
	}
	old := &unstructured.Unstructured{}
	if err := json.Unmarshal(req.OldObject.Raw, old); err != nil {
		return true
	}
	oldName := resources.GetAnnotation(old, hwpNameAnnotation)
	if oldName == "" {
		return false
	}
	oldNS := resources.GetAnnotation(old, hwpNamespaceAnnotation)
	if oldNS == "" {
		oldNS = old.GetNamespace()
	}
	return oldName != newName || oldNS != newNS
}

func (m *Module) handleHWPRemoval(ctx context.Context, req *admission.Request, obj *unstructured.Unstructured) *admission.Response {
	log := logf.FromContext(ctx)
	if req.OldObject.Raw == nil {
		return nil
	}
	old := &unstructured.Unstructured{}
	if err := json.Unmarshal(req.OldObject.Raw, old); err != nil {
		return nil
	}
	oldName := resources.GetAnnotation(old, hwpNameAnnotation)
	if oldName == "" {
		return nil
	}
	log.V(1).Info("HWP annotation removed, cleaning up", "oldProfile", oldName)
	oldNS := resources.GetAnnotation(old, hwpNamespaceAnnotation)
	if oldNS == "" {
		oldNS = old.GetNamespace()
	}
	oldHWP := &infrav1.HardwareProfile{}
	if err := m.apiReader.Get(ctx, types.NamespacedName{Name: oldName, Namespace: oldNS}, oldHWP); err != nil {
		// HWP not found or error — best-effort: remove namespace annotation only.
		resources.RemoveAnnotation(obj, hwpNamespaceAnnotation)
		raw, _ := json.Marshal(obj)
		resp := admission.PatchResponseFromRaw(req.Object.Raw, raw)
		return &resp
	}
	removeHWPSettings(obj, oldHWP)
	resources.RemoveAnnotation(obj, hwpNamespaceAnnotation)
	raw, err := json.Marshal(obj)
	if err != nil {
		resp := admission.Errored(http.StatusInternalServerError, err)
		return &resp
	}
	resp := admission.PatchResponseFromRaw(req.Object.Raw, raw)
	return &resp
}

func removeHWPSettings(obj *unstructured.Unstructured, hwp *infrav1.HardwareProfile) {
	if hwp.Spec.SchedulingSpec == nil || hwp.Spec.SchedulingSpec.Node == nil {
		return
	}
	node := hwp.Spec.SchedulingSpec.Node
	if len(node.Tolerations) > 0 {
		hwpKeys := map[string]bool{}
		for _, t := range node.Tolerations {
			hwpKeys[tolerationKeyCoreV1(t)] = true
		}
		existing, _, _ := unstructured.NestedSlice(obj.Object, notebookWorkloadConfig.tolerationsPath...)
		remaining := make([]any, 0)
		for _, e := range existing {
			if m, ok := e.(map[string]any); ok && !hwpKeys[tolerationKey(m)] {
				remaining = append(remaining, e)
			}
		}
		if len(remaining) == 0 {
			unstructured.RemoveNestedField(obj.Object, notebookWorkloadConfig.tolerationsPath...)
		} else {
			_ = unstructured.SetNestedSlice(obj.Object, remaining, notebookWorkloadConfig.tolerationsPath...)
		}
	}
	if len(node.NodeSelector) > 0 {
		existing, _, _ := unstructured.NestedStringMap(obj.Object, notebookWorkloadConfig.nodeSelectorPath...)
		for k, v := range node.NodeSelector {
			if existing[k] == v {
				delete(existing, k)
			}
		}
		if len(existing) == 0 {
			unstructured.RemoveNestedField(obj.Object, notebookWorkloadConfig.nodeSelectorPath...)
		} else {
			_ = unstructured.SetNestedStringMap(obj.Object, existing, notebookWorkloadConfig.nodeSelectorPath...)
		}
	}
	if hwp.Spec.SchedulingSpec.Kueue != nil && hwp.Spec.SchedulingSpec.Kueue.LocalQueueName != "" {
		resources.RemoveLabel(obj, cluster.KueueQueueNameLabel)
	}
}

func applyHWPToNotebook(ctx context.Context, obj *unstructured.Unstructured, hwp *infrav1.HardwareProfile, profileChanged bool) ([]string, error) {
	var warnings []string

	if profileChanged {
		logf.FromContext(ctx).V(1).Info("clearing scheduling settings due to profile change")
		resources.RemoveLabel(obj, cluster.KueueQueueNameLabel)
		unstructured.RemoveNestedField(obj.Object, notebookWorkloadConfig.nodeSelectorPath...)
		unstructured.RemoveNestedField(obj.Object, notebookWorkloadConfig.tolerationsPath...)
	}

	if len(hwp.Spec.Identifiers) > 0 {
		if err := applyIdentifiersToNotebook(ctx, obj, hwp); err != nil {
			return nil, fmt.Errorf("applying resource requirements: %w", err)
		}
	}

	if hwp.Spec.SchedulingSpec != nil {
		if hwp.Spec.SchedulingSpec.Kueue != nil && hwp.Spec.SchedulingSpec.Kueue.LocalQueueName != "" {
			hwpVal := hwp.Spec.SchedulingSpec.Kueue.LocalQueueName
			if !profileChanged {
				if existing := resources.GetLabel(obj, cluster.KueueQueueNameLabel); existing != "" && existing != hwpVal {
					warnings = append(warnings, fmt.Sprintf("label %q overwritten by HardwareProfile %q", cluster.KueueQueueNameLabel, hwp.Name))
				}
			}
			resources.SetLabel(obj, cluster.KueueQueueNameLabel, hwpVal)
			return warnings, nil
		}
		if hwp.Spec.SchedulingSpec.Node != nil {
			nodeWarnings, err := applyNotebookNodeScheduling(obj, hwp.Spec.SchedulingSpec.Node, profileChanged, hwp.Name)
			if err != nil {
				return nil, err
			}
			warnings = append(warnings, nodeWarnings...)
		}
	}

	return warnings, nil
}

func applyIdentifiersToNotebook(ctx context.Context, obj *unstructured.Unstructured, hwp *infrav1.HardwareProfile) error {
	containers, found, err := unstructured.NestedSlice(obj.Object, notebookWorkloadConfig.containersPath...)
	if err != nil || !found || len(containers) == 0 {
		return nil
	}
	indices := notebookMainIndices(obj, containers)
	if len(indices) == 0 && len(containers) > 1 {
		logf.FromContext(ctx).Info("no main container found; skipping HWP resource injection",
			"workload", obj.GetName(), "namespace", obj.GetNamespace())
		return nil
	}
	applySet := map[int]bool{}
	for _, i := range indices {
		applySet[i] = true
	}
	for i, c := range containers {
		if !applySet[i] {
			continue
		}
		if err := applyIdentifiers(c, hwp.Spec.Identifiers); err != nil {
			return fmt.Errorf("applying identifiers to container %d: %w", i, err)
		}
	}
	return unstructured.SetNestedSlice(obj.Object, containers, notebookWorkloadConfig.containersPath...)
}

func notebookMainIndices(obj *unstructured.Unstructured, containers []any) []int {
	if len(containers) == 1 {
		return []int{0}
	}
	name := obj.GetName()
	for i, c := range containers {
		if m, ok := c.(map[string]any); ok {
			if n, _ := m["name"].(string); n == name {
				return []int{i}
			}
		}
	}
	return []int{}
}

func applyIdentifiers(container any, identifiers []infrav1.HardwareIdentifier) error {
	cm, ok := container.(map[string]any)
	if !ok {
		return errors.New("container is not a map")
	}
	resMap, err := webhookutils.GetOrCreateNestedMap(cm, "resources")
	if err != nil {
		return err
	}
	requests, err := webhookutils.GetOrCreateNestedMap(resMap, "requests")
	if err != nil {
		return err
	}
	limits, err := webhookutils.GetOrCreateNestedMap(resMap, "limits")
	if err != nil {
		return err
	}
	for _, id := range identifiers {
		if _, exists := requests[id.Identifier]; exists {
			continue
		}
		q, err := convertIntOrString(id.DefaultCount)
		if err != nil {
			return fmt.Errorf("converting quantity for %s: %w", id.Identifier, err)
		}
		requests[id.Identifier] = q.String()
		if _, exists := limits[id.Identifier]; !exists {
			limits[id.Identifier] = q.String()
		}
	}
	resMap["requests"] = requests
	resMap["limits"] = limits
	cm["resources"] = resMap
	return nil
}

func convertIntOrString(v intstr.IntOrString) (resource.Quantity, error) {
	switch v.Type {
	case intstr.Int:
		return *resource.NewQuantity(int64(v.IntVal), resource.DecimalSI), nil
	case intstr.String:
		return resource.ParseQuantity(v.StrVal)
	default:
		return resource.Quantity{}, fmt.Errorf("invalid IntOrString type: %v", v.Type)
	}
}

func applyNotebookNodeScheduling(obj *unstructured.Unstructured, node *infrav1.NodeSchedulingSpec, profileChanged bool, hwpName string) ([]string, error) {
	var warnings []string
	if len(node.NodeSelector) > 0 {
		if profileChanged {
			if err := unstructured.SetNestedStringMap(obj.Object, node.NodeSelector, notebookWorkloadConfig.nodeSelectorPath...); err != nil {
				return nil, fmt.Errorf("setting nodeSelector: %w", err)
			}
		} else {
			merged, w, err := mergeNodeSelector(obj, node.NodeSelector, hwpName)
			if err != nil {
				return nil, err
			}
			warnings = append(warnings, w...)
			if err := unstructured.SetNestedStringMap(obj.Object, merged, notebookWorkloadConfig.nodeSelectorPath...); err != nil {
				return nil, err
			}
		}
	}
	if len(node.Tolerations) > 0 {
		hwpTols := make([]any, len(node.Tolerations))
		for i, t := range node.Tolerations {
			u, err := resources.ToUnstructured(&t)
			if err != nil {
				return nil, fmt.Errorf("converting toleration: %w", err)
			}
			hwpTols[i] = u.Object
		}
		var merged []any
		if profileChanged {
			merged = hwpTols
		} else {
			merged = mergeTolerations(obj, hwpTols)
		}
		if err := unstructured.SetNestedSlice(obj.Object, merged, notebookWorkloadConfig.tolerationsPath...); err != nil {
			return nil, err
		}
	}
	return warnings, nil
}

func mergeNodeSelector(obj *unstructured.Unstructured, hwpNS map[string]string, hwpName string) (map[string]string, []string, error) {
	existing, _, err := unstructured.NestedStringMap(obj.Object, notebookWorkloadConfig.nodeSelectorPath...)
	if err != nil {
		return nil, nil, fmt.Errorf("getting existing nodeSelector: %w", err)
	}
	merged := make(map[string]string)
	maps.Copy(merged, existing)
	var warnings []string
	for k, v := range hwpNS {
		if ev, ok := existing[k]; ok && ev != v {
			warnings = append(warnings, fmt.Sprintf("nodeSelector %q overwritten by HardwareProfile %q", k, hwpName))
		}
		merged[k] = v
	}
	return merged, warnings, nil
}

func mergeTolerations(obj *unstructured.Unstructured, hwpTols []any) []any {
	existing, _, _ := unstructured.NestedSlice(obj.Object, notebookWorkloadConfig.tolerationsPath...)
	hwpKeys := map[string]bool{}
	for _, t := range hwpTols {
		if m, ok := t.(map[string]any); ok {
			hwpKeys[tolerationKey(m)] = true
		}
	}
	merged := append([]any{}, hwpTols...)
	for _, e := range existing {
		if m, ok := e.(map[string]any); ok && !hwpKeys[tolerationKey(m)] {
			merged = append(merged, e)
		}
	}
	return merged
}

func tolerationKey(t map[string]any) string {
	key, _ := t["key"].(string)
	op, _ := t["operator"].(string)
	val, _ := t["value"].(string)
	effect, _ := t["effect"].(string)
	ts := ""
	if v, ok := t["tolerationSeconds"]; ok {
		ts = fmt.Sprintf("%v", v)
	}
	return fmt.Sprintf("%s:%s:%s:%s:%s", key, op, val, effect, ts)
}

func tolerationKeyCoreV1(t corev1.Toleration) string {
	ts := ""
	if t.TolerationSeconds != nil {
		ts = strconv.FormatInt(*t.TolerationSeconds, 10)
	}
	return fmt.Sprintf("%s:%s:%s:%s:%s", t.Key, string(t.Operator), t.Value, string(t.Effect), ts)
}
