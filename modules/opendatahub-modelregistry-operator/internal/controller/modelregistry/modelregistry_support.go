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

package modelregistry

import (
	"errors"
	"fmt"

	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-modelregistry-operator/pkg/resources/gvk"
	odhtypes "github.com/opendatahub-io/odh-platform-utilities/framework/controller/types"
)

func upstreamControllerSubject(rr *odhtypes.ReconciliationRequest) (rbacv1.Subject, error) {
	var fallback *rbacv1.Subject

	for i := range rr.Resources {
		resource := &rr.Resources[i]

		switch resource.GroupVersionKind() {
		case gvk.Deployment:
			serviceAccountName, found, err := unstructured.NestedString(
				resource.Object,
				"spec",
				"template",
				"spec",
				"serviceAccountName",
			)
			if err != nil {
				return rbacv1.Subject{}, fmt.Errorf(
					"reading serviceAccountName from deployment %s/%s: %w",
					resource.GetNamespace(),
					resource.GetName(),
					err,
				)
			}
			if found && serviceAccountName != "" {
				return rbacv1.Subject{
					Kind:      gvk.ServiceAccount.Kind,
					Name:      serviceAccountName,
					Namespace: resource.GetNamespace(),
				}, nil
			}
		case gvk.ServiceAccount:
			subject := rbacv1.Subject{
				Kind:      gvk.ServiceAccount.Kind,
				Name:      resource.GetName(),
				Namespace: resource.GetNamespace(),
			}
			if fallback == nil {
				fallback = &subject
			}
		}
	}

	if fallback != nil {
		return *fallback, nil
	}

	return rbacv1.Subject{}, errors.New("no rendered deployment or service account found")
}
