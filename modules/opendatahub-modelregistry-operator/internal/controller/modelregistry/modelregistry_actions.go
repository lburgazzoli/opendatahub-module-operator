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
	"fmt"
	"path"
	"slices"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	componentApi "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-modelregistry-operator/api/components/v1alpha1"
	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-modelregistry-operator/assets"
	fwdeploy "github.com/opendatahub-io/odh-platform-utilities/framework/controller/actions/deploy"
	odhtypes "github.com/opendatahub-io/odh-platform-utilities/framework/controller/types"
	kparams "github.com/opendatahub-io/odh-platform-utilities/framework/render/kustomize/params"
)

const (
	openShiftConfigGrantsTemplatePath = "manifests/ext/openshift-config-grants.yaml.tmpl"
)

// stageManifests sets the per-reconcile manifest list.
func (m *Module) stageManifests(_ context.Context, rr *odhtypes.ReconciliationRequest) error {
	rr.Manifests = []odhtypes.ManifestInfo{
		m.manifestInfo,
		m.extraManifest,
	}
	rr.Templates = []odhtypes.TemplateInfo{{
		FS:   assets.Manifests,
		Path: openShiftConfigGrantsTemplatePath,
	}}
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

	if err := kparams.Apply(
		m.kustomizeFS,
		path.Join(rr.Manifests[0].String(), "params.env"),
		kparams.Values(extraParams),
	); err != nil {
		return fmt.Errorf("failed to update params on path %s: %w", rr.Manifests[0].String(), err)
	}

	return nil
}

// configureDependencies ensures the registries namespace exists.
func (m *Module) configureDependencies(_ context.Context, rr *odhtypes.ReconciliationRequest) error {
	mr, ok := rr.Instance.(*componentApi.ModelRegistry)
	if !ok {
		return fmt.Errorf("resource instance %v is not a ModelRegistry", rr.Instance)
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
	); err != nil {
		return fmt.Errorf("failed to add namespace dependency for %s: %w", mr.Spec.RegistriesNamespace, err)
	}

	return nil
}

// reportStatus refreshes status fields from the cached metadata and spec.
func (m *Module) reportStatus(_ context.Context, rr *odhtypes.ReconciliationRequest) error {
	obj, ok := rr.Instance.(*componentApi.ModelRegistry)
	if !ok {
		return fmt.Errorf("instance is not a ModelRegistry")
	}

	obj.Status.RegistriesNamespace = obj.Spec.RegistriesNamespace
	obj.Status.Releases = slices.Clone(m.releases)

	return nil
}
