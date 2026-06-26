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
	"fmt"
	"path"

	corev1 "k8s.io/api/core/v1"

	fwtypes "github.com/opendatahub-io/odh-platform-utilities/framework/controller/types"
	kparams "github.com/opendatahub-io/odh-platform-utilities/framework/render/kustomize/params"
	odhcluster "github.com/opendatahub-io/odh-platform-utilities/pkg/cluster"

	localapi "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-workbenches-operator/api/components/v1alpha1"
	module "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-workbenches-operator/pkg/module"
)

// initialize assigns the pre-computed manifest infos for this reconcile cycle.
func (m *Module) initialize(_ context.Context, rr *fwtypes.ReconciliationRequest) error {
	rr.Manifests = m.manifestInfos
	return nil
}

// customizeManifests writes gateway URL, section title, and mlflow-enabled into params.env.
func (m *Module) customizeManifests(ctx context.Context, rr *fwtypes.ReconciliationRequest) error {
	extraParamsMap, err := ComputeKustomizeVariable(ctx, rr.Client, localapi.Platform(rr.Release.Name))
	if err != nil {
		return fmt.Errorf("computing kustomize variables: %w", err)
	}

	paramsPath := path.Join(notebookControllerContextDir, notebookControllerManifestSourcePath)
	if err := kparams.Apply(
		m.renderFS,
		path.Join(paramsPath, "params.env"),
		kparams.Values(extraParamsMap),
	); err != nil {
		return fmt.Errorf("applying params.env from %s: %w", paramsPath, err)
	}
	return nil
}

// configureDependencies creates the workbench namespace with the owned-namespace label.
func (m *Module) configureDependencies(_ context.Context, rr *fwtypes.ReconciliationRequest) error {
	workbench, ok := rr.Instance.(*localapi.Workbenches)
	if !ok {
		return fmt.Errorf("resource instance %v is not a Workbenches", rr.Instance)
	}

	ns := &corev1.Namespace{}
	ns.Labels = map[string]string{
		module.OwnedNamespaceLabel: "true",
	}

	switch {
	case workbench.Spec.WorkbenchNamespace != "":
		ns.Name = workbench.Spec.WorkbenchNamespace
	case localapi.Platform(rr.Release.Name) == localapi.Platform(odhcluster.SelfManagedRhoai) ||
		localapi.Platform(rr.Release.Name) == localapi.Platform(odhcluster.ManagedRhoai):
		ns.Name = module.DefaultNotebooksNamespaceRHOAI
	default:
		ns.Name = module.DefaultNotebooksNamespaceODH
	}

	if err := rr.AddResources(ns); err != nil {
		return fmt.Errorf("failed to create namespace for workbenches: %w", err)
	}

	return nil
}

// reportStatus writes the platform version handshake entry into status.releases
// and populates workbench-specific fields (WorkbenchNamespace).
func (m *Module) reportStatus(_ context.Context, rr *fwtypes.ReconciliationRequest) error {
	obj, ok := rr.Instance.(*localapi.Workbenches)
	if !ok {
		return fmt.Errorf("instance is not a Workbenches")
	}

	obj.Status.WorkbenchNamespace = obj.Spec.WorkbenchNamespace

	UpsertRelease(obj.GetReleaseStatus(), m.cfg.ComponentRelease())

	return nil
}
