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
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/yaml"

	"github.com/opendatahub-io/operator-actions-framework/cluster/gvk"
)

const (
	yamlFieldNamespace          = "namespace:"
	yamlFieldServiceAccountName = "serviceAccountName:"
	yamlFieldSubjects           = "subjects:"
	yamlFieldName               = "name:"
	yamlFieldImage              = "image:"
	yamlFieldImagePullPolicy    = "imagePullPolicy:"
	yamlFieldReplicas           = "replicas:"
	yamlFieldResources          = "resources:"
	yamlFieldData               = "data:"
	yamlFieldMetadata           = "metadata:"

	tplReleaseNamespace   = "namespace: {{ .Release.Namespace }}"
	tplServiceAccountName = `{{ default (include "chart.fullname" .) .Values.serviceAccount.name }}`

	annotationCertManagerInjectCAFrom = "cert-manager.io/inject-ca-from"
)

func renderGroup(
	resourceGVK schema.GroupVersionKind,
	resources []unstructured.Unstructured,
) (string, error) {
	var parts []string

	for i := range resources {
		transformed, err := transformResource(resourceGVK, &resources[i])
		if err != nil {
			return "", fmt.Errorf("transforming %s/%s: %w", resourceGVK.Kind, resources[i].GetName(), err)
		}

		parts = append(parts, transformed)
	}

	return strings.Join(parts, "\n---\n"), nil
}

var stripLabelKeys = []string{
	"app.kubernetes.io/managed-by",
}

func stripLabels(obj *unstructured.Unstructured) {
	labels := obj.GetLabels()
	if len(labels) == 0 {
		return
	}

	for _, key := range stripLabelKeys {
		delete(labels, key)
	}

	if len(labels) == 0 {
		obj.SetLabels(nil)
	} else {
		obj.SetLabels(labels)
	}
}

func transformResource(
	resourceGVK schema.GroupVersionKind,
	obj *unstructured.Unstructured,
) (string, error) {
	stripLabels(obj)

	switch resourceGVK {
	case gvk.Deployment:
		return transformDeployment(obj)
	case gvk.ServiceAccount:
		return transformServiceAccount(obj)
	case gvk.ConfigMap:
		return transformConfigMap(obj)
	case gvk.ClusterRoleBinding, gvk.RoleBinding:
		return transformRoleBinding(obj)
	case gvk.MutatingWebhookConfiguration, gvk.ValidatingWebhookConfiguration:
		return transformWebhook(obj)
	case gvk.CertManagerCertificate:
		return transformCertificate(obj)
	default:
		return transformGeneric(obj)
	}
}

func transformDeployment(obj *unstructured.Unstructured) (string, error) {
	raw, err := marshalResource(obj)
	if err != nil {
		return "", err
	}

	raw = replaceNamespace(raw)
	raw = replaceImageField(raw)
	raw = replaceReplicas(raw)
	raw = replaceResources(raw)
	raw = replaceServiceAccountName(raw)
	raw = addImagePullSecrets(raw)

	return raw, nil
}

func transformServiceAccount(obj *unstructured.Unstructured) (string, error) {
	raw, err := marshalResource(obj)
	if err != nil {
		return "", err
	}

	raw = replaceNamespace(raw)
	raw = replaceServiceAccountMetadata(raw, obj.GetName())

	return raw, nil
}

func transformConfigMap(obj *unstructured.Unstructured) (string, error) {
	raw, err := marshalResource(obj)
	if err != nil {
		return "", err
	}

	raw = replaceNamespace(raw)
	raw = injectConfigMapValues(raw)

	return raw, nil
}

func transformRoleBinding(obj *unstructured.Unstructured) (string, error) {
	raw, err := marshalResource(obj)
	if err != nil {
		return "", err
	}

	raw = replaceNamespace(raw)
	raw = replaceSubjectsNamespace(raw)
	raw = replaceSubjectsServiceAccount(raw)

	return raw, nil
}

func transformWebhook(obj *unstructured.Unstructured) (string, error) {
	raw, err := marshalResource(obj)
	if err != nil {
		return "", err
	}

	raw = replaceWebhookNamespace(raw)

	return raw, nil
}

func transformCertificate(obj *unstructured.Unstructured) (string, error) {
	raw, err := marshalResource(obj)
	if err != nil {
		return "", err
	}

	raw = replaceNamespace(raw)
	raw = replaceCertificateDNSNames(raw)

	return raw, nil
}

func replaceCertificateDNSNames(raw string) string {
	lines := strings.Split(raw, "\n")
	inDNSNames := false

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)

		if trimmed == "dnsNames:" {
			inDNSNames = true
			continue
		}

		if inDNSNames && !strings.HasPrefix(trimmed, "-") && trimmed != "" {
			inDNSNames = false
		}

		if inDNSNames && strings.HasPrefix(trimmed, "- ") && strings.Contains(trimmed, ".svc") {
			entry := strings.TrimPrefix(trimmed, "- ")
			parts := strings.SplitN(entry, ".", 3)
			if len(parts) >= 3 {
				indent := line[:len(line)-len(strings.TrimLeft(line, " "))]
				suffix := parts[2]
				lines[i] = indent + "- " + parts[0] + ".{{ .Release.Namespace }}." + suffix
			}
		}
	}

	return strings.Join(lines, "\n")
}

func transformGeneric(obj *unstructured.Unstructured) (string, error) {
	raw, err := marshalResource(obj)
	if err != nil {
		return "", err
	}

	if obj.GetNamespace() != "" {
		raw = replaceNamespace(raw)
	}

	return raw, nil
}

func marshalResource(obj *unstructured.Unstructured) (string, error) {
	data, err := yaml.Marshal(obj.Object)
	if err != nil {
		return "", fmt.Errorf("marshaling resource: %w", err)
	}

	return strings.TrimSpace(string(data)), nil
}

func replaceNamespace(raw string) string {
	lines := strings.Split(raw, "\n")

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, yamlFieldNamespace) && !strings.Contains(trimmed, "{{") {
			indent := line[:len(line)-len(strings.TrimLeft(line, " "))]
			lines[i] = indent + tplReleaseNamespace
		}
	}

	return strings.Join(lines, "\n")
}

func replaceImageField(raw string) string {
	lines := strings.Split(raw, "\n")
	var result []string

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		switch {
		case strings.HasPrefix(trimmed, yamlFieldImage) && !strings.Contains(trimmed, "{{"):
			indent := line[:len(line)-len(strings.TrimLeft(line, " "))]
			result = append(result, indent+`image: "{{ include "chart.imageRef" . }}"`)
			result = append(result, indent+"imagePullPolicy: Always")
		case strings.HasPrefix(trimmed, yamlFieldImagePullPolicy):
		default:
			result = append(result, line)
		}
	}

	return strings.Join(result, "\n")
}

func replaceReplicas(raw string) string {
	lines := strings.Split(raw, "\n")

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, yamlFieldReplicas) && !strings.Contains(trimmed, "{{") {
			indent := line[:len(line)-len(strings.TrimLeft(line, " "))]
			lines[i] = indent + "replicas: {{ .Values.replicas }}"
		}
	}

	return strings.Join(lines, "\n")
}

func replaceResources(raw string) string {
	lines := strings.Split(raw, "\n")
	var result []string

	i := 0
	for i < len(lines) {
		trimmed := strings.TrimSpace(lines[i])
		if trimmed == yamlFieldResources {
			indent := lines[i][:len(lines[i])-len(strings.TrimLeft(lines[i], " "))]
			result = append(result, indent+"resources:")
			result = append(result, indent+"  {{- toYaml .Values.resources | nindent "+fmt.Sprintf("%d", len(indent)+2)+" }}")

			i++
			baseIndent := len(indent) + 2
			for i < len(lines) {
				lineIndent := len(lines[i]) - len(strings.TrimLeft(lines[i], " "))
				if strings.TrimSpace(lines[i]) == "" || lineIndent >= baseIndent {
					i++
				} else {
					break
				}
			}

			continue
		}

		result = append(result, lines[i])
		i++
	}

	return strings.Join(result, "\n")
}

func replaceServiceAccountName(raw string) string {
	lines := strings.Split(raw, "\n")

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, yamlFieldServiceAccountName) && !strings.Contains(trimmed, "{{") {
			indent := line[:len(line)-len(strings.TrimLeft(line, " "))]
			lines[i] = indent + yamlFieldServiceAccountName + " " + tplServiceAccountName
		}
	}

	return strings.Join(lines, "\n")
}

func addImagePullSecrets(raw string) string {
	lines := strings.Split(raw, "\n")
	var result []string

	for _, line := range lines {
		result = append(result, line)
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, yamlFieldServiceAccountName) {
			indent := line[:len(line)-len(strings.TrimLeft(line, " "))]
			result = append(result,
				indent+"{{- with .Values.imagePullSecret }}",
				indent+"imagePullSecrets:",
				indent+"  - name: {{ . }}",
				indent+"{{- end }}",
			)
		}
	}

	return strings.Join(result, "\n")
}

func replaceServiceAccountMetadata(raw string, originalName string) string {
	lines := strings.Split(raw, "\n")
	var result []string

	inMetadata := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		if trimmed == yamlFieldMetadata {
			inMetadata = true
		} else if !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "\t") && trimmed != "" {
			inMetadata = false
		}

		if inMetadata && strings.HasPrefix(trimmed, yamlFieldName) && strings.Contains(trimmed, originalName) {
			indent := line[:len(line)-len(strings.TrimLeft(line, " "))]
			result = append(result, indent+yamlFieldName+" "+tplServiceAccountName)
			result = append(result,
				indent+"{{- with .Values.serviceAccount.annotations }}",
				indent+"annotations:",
				indent+"  {{- toYaml . | nindent "+fmt.Sprintf("%d", len(indent)+2)+" }}",
				indent+"{{- end }}",
			)

			continue
		}

		result = append(result, line)
	}

	return strings.Join(result, "\n")
}

func replaceSubjectsNamespace(raw string) string {
	lines := strings.Split(raw, "\n")
	inSubjects := false

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)

		if trimmed == yamlFieldSubjects {
			inSubjects = true
			continue
		}

		if inSubjects && !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "\t") &&
			!strings.HasPrefix(trimmed, "-") && trimmed != "" {
			inSubjects = false
		}

		if inSubjects && strings.HasPrefix(trimmed, yamlFieldNamespace) && !strings.Contains(trimmed, "{{") {
			indent := line[:len(line)-len(strings.TrimLeft(line, " "))]
			lines[i] = indent + tplReleaseNamespace
		}
	}

	return strings.Join(lines, "\n")
}

func replaceSubjectsServiceAccount(raw string) string {
	lines := strings.Split(raw, "\n")
	inSubjects := false

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)

		if trimmed == yamlFieldSubjects {
			inSubjects = true
			continue
		}

		if inSubjects && !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "\t") &&
			!strings.HasPrefix(trimmed, "-") && trimmed != "" {
			inSubjects = false
		}

		if inSubjects && strings.HasPrefix(trimmed, yamlFieldName) && !strings.Contains(trimmed, "{{") {
			indent := line[:len(line)-len(strings.TrimLeft(line, " "))]
			lines[i] = indent + yamlFieldName + " " + tplServiceAccountName
		}
	}

	return strings.Join(lines, "\n")
}

func replaceWebhookNamespace(raw string) string {
	raw = replaceNamespace(raw)

	lines := strings.Split(raw, "\n")

	for i, line := range lines {
		if strings.Contains(line, annotationCertManagerInjectCAFrom) {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				value := strings.TrimSpace(parts[1])
				valueParts := strings.SplitN(value, "/", 2)
				if len(valueParts) == 2 {
					indent := line[:len(line)-len(strings.TrimLeft(line, " "))]
					lines[i] = indent + strings.TrimSpace(parts[0]) + ": {{ .Release.Namespace }}/" + valueParts[1]
				}
			}
		}
	}

	return strings.Join(lines, "\n")
}

func injectConfigMapValues(raw string) string {
	lines := strings.Split(raw, "\n")
	var result []string

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, yamlFieldData) {
			indent := line[:len(line)-len(strings.TrimLeft(line, " "))]
			result = append(result, indent+yamlFieldData)
			result = append(result,
				indent+"  {{- with .Values.imagePullSecret }}",
				indent+"  imagePullSecret: {{ . }}",
				indent+"  {{- end }}",
				indent+"  {{- range $key, $val := .Values.config }}",
				indent+"  {{ $key }}: {{ $val | quote }}",
				indent+"  {{- end }}",
			)

			continue
		}

		result = append(result, line)
	}

	return strings.Join(result, "\n")
}
