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
	"net/http"
	"strings"

	admissionv1 "k8s.io/api/admission/v1"
	authorizationv1 "k8s.io/api/authorization/v1"
	corev1 "k8s.io/api/core/v1"
	k8serr "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/opendatahub-io/opendatahub-operator/v2/pkg/metadata/annotations"
	"github.com/opendatahub-io/opendatahub-operator/v2/pkg/resources"
)

// +kubebuilder:webhook:path=/platform-connection-notebook,mutating=true,failurePolicy=fail,groups=kubeflow.org,resources=notebooks,verbs=create;update,versions=v1,name=connection-notebook.opendatahub.io,sideEffects=None,admissionReviewVersions=v1

// handleNotebookConnection validates opendatahub.io/connections annotation and injects
// referenced secrets into the notebook container's envFrom.
func (m *Module) handleNotebookConnection(ctx context.Context, req admission.Request) admission.Response {
	log := logf.FromContext(ctx)

	if m.decoder == nil {
		return admission.Errored(http.StatusInternalServerError, errors.New("webhook decoder not initialized"))
	}

	nb := &unstructured.Unstructured{}
	if err := m.decoder.Decode(req, nb); err != nil {
		return admission.Errored(http.StatusBadRequest, fmt.Errorf("failed to decode object: %w", err))
	}

	if !nb.GetDeletionTimestamp().IsZero() {
		return admission.Allowed("object marked for deletion, skipping")
	}

	switch req.Operation {
	case admissionv1.Create, admissionv1.Update:
		resp, shouldInject, secretRefs := m.validateConnectionAnnotation(ctx, nb, &req)
		if !resp.Allowed {
			return resp
		}
		if !shouldInject || secretRefs == nil {
			return admission.Allowed("connection annotation valid, no injection needed")
		}

		injected, obj, err := performConnectionInjection(nb, secretRefs)
		if err != nil {
			log.Error(err, "failed to perform connection injection")
			return admission.Errored(http.StatusInternalServerError, err)
		}
		if injected {
			raw, err := json.Marshal(obj)
			if err != nil {
				return admission.Errored(http.StatusInternalServerError, err)
			}
			return admission.PatchResponseFromRaw(req.Object.Raw, raw)
		}
	}

	return admission.Allowed(fmt.Sprintf("operation %s allowed", req.Operation))
}

func (m *Module) validateConnectionAnnotation(
	ctx context.Context,
	nb *unstructured.Unstructured,
	req *admission.Request,
) (admission.Response, bool, []notebookSecretRef) {
	annotationValue := resources.GetAnnotation(nb, annotations.Connection)
	if req.Operation == admissionv1.Create && annotationValue == "" {
		return admission.Allowed("no connection annotation"), false, nil
	}

	secretRefs, err := parseConnectionsAnnotation(annotationValue)
	if err != nil {
		return admission.Denied(fmt.Sprintf("invalid connections annotation: %v", err)), false, nil
	}

	existErrors, permErrors, err := m.checkSecretsAndPermissions(ctx, req, secretRefs)
	if err != nil {
		return admission.Errored(http.StatusInternalServerError, err), false, nil
	}
	if len(existErrors) > 0 {
		return admission.Denied(fmt.Sprintf("connection secrets not found: %s", strings.Join(existErrors, ", "))), false, nil
	}
	if len(permErrors) > 0 {
		return admission.Denied(fmt.Sprintf("user lacks permission for secrets: %s", strings.Join(permErrors, ", "))), false, nil
	}

	refs, err := m.buildSecretRefs(ctx, req, secretRefs)
	if err != nil {
		return admission.Errored(http.StatusInternalServerError, err), false, nil
	}
	return admission.Allowed("permissions validated"), true, refs
}

type notebookSecretRef struct {
	secret corev1.SecretReference
	action string // "create" or "delete"
}

func parseConnectionsAnnotation(value string) ([]corev1.SecretReference, error) {
	if strings.TrimSpace(value) == "" {
		return nil, nil
	}
	var refs []corev1.SecretReference
	for part := range strings.SplitSeq(value, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		parts := strings.Split(part, "/")
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid secret reference %q: expected namespace/name", part)
		}
		ns, name := strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
		if ns == "" || name == "" {
			return nil, fmt.Errorf("invalid secret reference %q: namespace and name required", part)
		}
		refs = append(refs, corev1.SecretReference{Namespace: ns, Name: name})
	}
	return refs, nil
}

func (m *Module) checkSecretsAndPermissions(ctx context.Context, req *admission.Request, refs []corev1.SecretReference) ([]string, []string, error) {
	var existErrs, permErrs []string
	for _, ref := range refs {
		if ref.Namespace != req.Namespace {
			existErrs = append(existErrs, ref.Namespace+"/"+ref.Name)
			continue
		}
		switch err := m.apiReader.Get(ctx, client.ObjectKey{Namespace: ref.Namespace, Name: ref.Name}, &corev1.Secret{}); {
		case err == nil:
		case k8serr.IsNotFound(err):
			existErrs = append(existErrs, ref.Namespace+"/"+ref.Name)
			continue
		default:
			return nil, nil, fmt.Errorf("checking secret: %w", err)
		}
		sar := &authorizationv1.SubjectAccessReview{
			Spec: authorizationv1.SubjectAccessReviewSpec{
				User:   req.UserInfo.Username,
				Groups: req.UserInfo.Groups,
				ResourceAttributes: &authorizationv1.ResourceAttributes{
					Namespace: ref.Namespace, Verb: "get",
					Group: "", Version: "v1", Resource: "secrets", Name: ref.Name,
				},
			},
		}
		if err := m.webhookClient.Create(ctx, sar); err != nil {
			return nil, nil, fmt.Errorf("creating SubjectAccessReview: %w", err)
		}
		if !sar.Status.Allowed {
			permErrs = append(permErrs, ref.Namespace+"/"+ref.Name)
		}
	}
	return existErrs, permErrs, nil
}

func (m *Module) buildSecretRefs(_ context.Context, req *admission.Request, refs []corev1.SecretReference) ([]notebookSecretRef, error) {
	if req.Operation != admissionv1.Update {
		out := make([]notebookSecretRef, 0, len(refs))
		for _, r := range refs {
			out = append(out, notebookSecretRef{secret: r, action: "create"})
		}
		return out, nil
	}

	old := &unstructured.Unstructured{}
	if err := m.decoder.DecodeRaw(req.OldObject, old); err != nil {
		return nil, fmt.Errorf("decoding old object: %w", err)
	}
	oldRefs, err := parseConnectionsAnnotation(resources.GetAnnotation(old, annotations.Connection))
	if err != nil {
		return nil, err
	}

	actions := determineSecretActions(oldRefs, refs)
	allRefs := append(refs, oldRefs...)
	var out []notebookSecretRef
	for _, r := range allRefs {
		out = append(out, notebookSecretRef{secret: r, action: actions[r.Namespace+"/"+r.Name]})
	}
	return out, nil
}

func determineSecretActions(old, cur []corev1.SecretReference) map[string]string {
	actions := map[string]string{}
	oldMap := map[string]bool{}
	curMap := map[string]bool{}
	for _, r := range old {
		oldMap[r.Namespace+"/"+r.Name] = true
	}
	for _, r := range cur {
		k := r.Namespace + "/" + r.Name
		curMap[k] = true
		if !oldMap[k] {
			actions[k] = "create"
		}
	}
	for _, r := range old {
		k := r.Namespace + "/" + r.Name
		if !curMap[k] {
			actions[k] = "delete"
		}
	}
	return actions
}

var notebookContainersPath = []string{"spec", "template", "spec", "containers"}

func performConnectionInjection(nb *unstructured.Unstructured, secretRefs []notebookSecretRef) (bool, *unstructured.Unstructured, error) {
	containers, found, err := unstructured.NestedSlice(nb.Object, notebookContainersPath...)
	if err != nil {
		return false, nil, fmt.Errorf("getting containers: %w", err)
	}
	if !found || len(containers) == 0 {
		return false, nil, errors.New("no containers found in notebook")
	}
	container, ok := containers[0].(map[string]any)
	if !ok {
		return false, nil, errors.New("first container is not a map")
	}

	envFrom, _ := container["envFrom"].([]any)
	for _, ref := range secretRefs {
		envFrom = handleConnectionSecret(ref, envFrom)
	}
	container["envFrom"] = envFrom
	containers[0] = container

	if err := unstructured.SetNestedSlice(nb.Object, containers, notebookContainersPath...); err != nil {
		return false, nil, fmt.Errorf("setting containers: %w", err)
	}
	return true, nb, nil
}

func handleConnectionSecret(ref notebookSecretRef, envFrom []any) []any {
	switch ref.action {
	case "create":
		return append(envFrom, map[string]any{"secretRef": map[string]any{"name": ref.secret.Name}})
	case "delete":
		for i, e := range envFrom {
			if m, ok := e.(map[string]any); ok {
				if sr, ok := m["secretRef"].(map[string]any); ok {
					if sr["name"] == ref.secret.Name {
						return append(envFrom[:i], envFrom[i+1:]...)
					}
				}
			}
		}
	}
	return envFrom
}
