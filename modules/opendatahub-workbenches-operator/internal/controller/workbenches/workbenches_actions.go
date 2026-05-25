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
	"sort"

	corev1 "k8s.io/api/core/v1"

	"github.com/opendatahub-io/opendatahub-operator/v2/pkg/cluster"
	odhtypes "github.com/opendatahub-io/opendatahub-operator/v2/pkg/controller/types"
	odhdeploy "github.com/opendatahub-io/opendatahub-operator/v2/pkg/deploy"
	"github.com/opendatahub-io/opendatahub-operator/v2/pkg/metadata/labels"

	localapi "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-workbenches-operator/api/components/v1alpha1"
	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-workbenches-operator/pkg/version"
)

// initialize assigns the pre-computed manifest infos for this reconcile cycle.
func (m *Module) initialize(_ context.Context, rr *odhtypes.ReconciliationRequest) error {
	rr.Manifests = m.manifestInfos
	return nil
}

// configureDependencies creates the workbench namespace with the owned-namespace label.
func (m *Module) configureDependencies(_ context.Context, rr *odhtypes.ReconciliationRequest) error {
	workbench, ok := rr.Instance.(*localapi.Workbenches)
	if !ok {
		return fmt.Errorf("resource instance %v is not a Workbenches", rr.Instance)
	}

	ns := &corev1.Namespace{}
	ns.Labels = map[string]string{
		labels.ODH.OwnedNamespace: "true",
	}

	switch {
	case workbench.Spec.WorkbenchNamespace != "":
		ns.Name = workbench.Spec.WorkbenchNamespace
	case rr.Release.Name == cluster.SelfManagedRhoai || rr.Release.Name == cluster.ManagedRhoai:
		ns.Name = cluster.DefaultNotebooksNamespaceRHOAI
	default:
		ns.Name = cluster.DefaultNotebooksNamespaceODH
	}

	if err := rr.AddResources(ns); err != nil {
		return fmt.Errorf("failed to create namespace for workbenches: %w", err)
	}

	return nil
}

// reportStatus populates the module status with version, platform, source information,
// and workbench-specific fields (WorkbenchNamespace).
func (m *Module) reportStatus(_ context.Context, rr *odhtypes.ReconciliationRequest) error {
	obj, ok := rr.Instance.(*localapi.Workbenches)
	if !ok {
		return fmt.Errorf("instance is not a Workbenches")
	}

	obj.Status.WorkbenchNamespace = obj.Spec.WorkbenchNamespace

	obj.Status.Module = localapi.ModuleStatus{
		Version:     m.version,
		BuildSource: version.Repo + "@" + version.Branch + "/" + version.Commit,
		Platform: localapi.PlatformStatus{
			Name:    m.cfg.PlatformType,
			Version: m.platformVersion,
		},
	}

	var sources []localapi.SourceStatus
	for _, manifest := range rr.Manifests {
		sources = append(sources, localapi.SourceStatus{
			Path:     manifest.String(),
			Renderer: localapi.SourceRendererKustomize,
		})
	}

	sort.Slice(sources, func(i int, j int) bool {
		if sources[i].Path == sources[j].Path {
			return sources[i].Renderer < sources[j].Renderer
		}

		return sources[i].Path < sources[j].Path
	})

	obj.Status.Module.Sources = sources

	return nil
}

// setKustomizedParams writes gateway URL, section title, and mlflow-enabled into params.env.
func (m *Module) setKustomizedParams(ctx context.Context, rr *odhtypes.ReconciliationRequest) error {
	extraParamsMap, err := ComputeKustomizeVariable(ctx, rr.Client, rr.Release.Name)
	if err != nil {
		return fmt.Errorf("computing kustomize variables: %w", err)
	}

	paramsPath := path.Join(rr.ManifestsBasePath, notebookControllerContextDir, notebookControllerManifestSourcePath)
	if err := odhdeploy.ApplyParams(paramsPath, "params.env", nil, extraParamsMap); err != nil {
		return fmt.Errorf("applying params.env from %s: %w", paramsPath, err)
	}
	return nil
}
