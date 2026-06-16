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
	"context"
	"errors"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ofVersion "github.com/operator-framework/api/pkg/lib/version"

	componentApi "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-modelregistry-operator/api/components/v1alpha1"
	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-modelregistry-operator/pkg/resources/gvk"
	fwdeploy "github.com/opendatahub-io/odh-platform-utilities/framework/controller/actions/deploy"
	odhtypes "github.com/opendatahub-io/odh-platform-utilities/framework/controller/types"
	fwparams "github.com/opendatahub-io/odh-platform-utilities/framework/utils/params"
)

const (
	openShiftAPIServerReaderRoleName        = "model-registry-operator-apiserver-reader"
	openShiftAPIServerReaderRoleBindingName = "model-registry-operator-apiserver-reader-binding"
)

// initialize sets the per-reconcile manifest list.
func (m *Module) initialize(_ context.Context, rr *odhtypes.ReconciliationRequest) error {
	rr.Manifests = []odhtypes.ManifestInfo{
		m.manifestInfo,
		m.extraManifest,
	}
	return nil
}

// customizeManifests computes kustomize variables (gateway, namespace) and writes them to params.env.
func (m *Module) customizeManifests(_ context.Context, rr *odhtypes.ReconciliationRequest) error {
	mr, ok := rr.Instance.(*componentApi.ModelRegistry)
	if !ok {
		return fmt.Errorf("resource instance %v is not a ModelRegistry", rr.Instance)
	}

	extraParams, err := m.computeKustomizeVariables(mr)
	if err != nil {
		return fmt.Errorf("failed to compute kustomize variables: %w", err)
	}

	extraParams["REGISTRIES_NAMESPACE"] = mr.Spec.RegistriesNamespace

	if err := fwparams.Apply(
		rr.Manifests[0].String(),
		"params.env",
		fwparams.Values(extraParams),
	); err != nil {
		return fmt.Errorf("failed to update params on path %s: %w", rr.Manifests[0].String(), err)
	}

	return nil
}

// configureDependencies ensures the registries namespace exists and adds
// supplemental OpenShift RBAC for the upstream controller.
func (m *Module) configureDependencies(_ context.Context, rr *odhtypes.ReconciliationRequest) error {
	mr, ok := rr.Instance.(*componentApi.ModelRegistry)
	if !ok {
		return fmt.Errorf("resource instance %v is not a ModelRegistry", rr.Instance)
	}

	subject, err := upstreamControllerSubject(rr)
	if err != nil {
		return fmt.Errorf("failed to resolve upstream controller service account: %w", err)
	}

	if err := rr.AddResources(
		&corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: mr.Spec.RegistriesNamespace,
				Annotations: map[string]string{
					fwdeploy.DefaultManagedByAnnotation: "false",
				},
			},
		},
		// Temporary: grant the rendered upstream controller access to
		// config.openshift.io/apiservers until the fetched upstream RBAC carries
		// this permission directly.
		&rbacv1.ClusterRole{
			ObjectMeta: metav1.ObjectMeta{
				Name: openShiftAPIServerReaderRoleName,
			},
			Rules: []rbacv1.PolicyRule{{
				APIGroups: []string{"config.openshift.io"},
				Resources: []string{"apiservers"},
				Verbs:     []string{"get", "list", "watch"},
			}},
		},
		&rbacv1.ClusterRoleBinding{
			ObjectMeta: metav1.ObjectMeta{
				Name: openShiftAPIServerReaderRoleBindingName,
			},
			RoleRef: rbacv1.RoleRef{
				APIGroup: gvk.ClusterRole.Group,
				Kind:     gvk.ClusterRole.Kind,
				Name:     openShiftAPIServerReaderRoleName,
			},
			Subjects: []rbacv1.Subject{
				subject,
			},
		},
	); err != nil {
		return fmt.Errorf(
			"failed to add namespace and OpenShift RBAC dependencies for %s: %w",
			mr.Spec.RegistriesNamespace,
			err,
		)
	}

	return nil
}

// updateStatus copies the registries namespace from spec to status.
func (m *Module) updateStatus(_ context.Context, rr *odhtypes.ReconciliationRequest) error {
	mr, ok := rr.Instance.(*componentApi.ModelRegistry)
	if !ok {
		return errors.New("instance is not of type *ModelRegistry")
	}

	mr.Status.RegistriesNamespace = mr.Spec.RegistriesNamespace

	return nil
}

// reportStatus populates the release status and platform.
func (m *Module) reportStatus(_ context.Context, rr *odhtypes.ReconciliationRequest) error {
	obj, ok := rr.Instance.(*componentApi.ModelRegistry)
	if !ok {
		return fmt.Errorf("instance is not a ModelRegistry")
	}

	obj.Status.Release = componentApi.Release{
		Name:    componentApi.Platform(rr.Release.Name),
		Version: ofVersion.OperatorVersion{Version: rr.Release.Version},
	}

	return nil
}
