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

package mymodule

import (
	"context"
	"fmt"

	componentApi "github.com/lburgazzoli/opendatahub-module-operator/api/components/v1alpha1"
	moduleconfig "github.com/lburgazzoli/opendatahub-module-operator/pkg/config"
	"github.com/lburgazzoli/opendatahub-module-operator/pkg/version"
	"github.com/opendatahub-io/opendatahub-operator/v2/api/common"
	"github.com/opendatahub-io/opendatahub-operator/v2/pkg/controller/actions"
	odhtypes "github.com/opendatahub-io/opendatahub-operator/v2/pkg/controller/types"
)

// newInitializeAction returns an action that sets up the manifest sources
// based on the configured platform type.
func newInitializeAction(platformType string) actions.Fn {
	return func(_ context.Context, rr *odhtypes.ReconciliationRequest) error {
		rr.Manifests = append(rr.Manifests,
			manifestPath(rr.ManifestsBasePath, common.Platform(platformType)))

		return nil
	}
}

// newReportStatusAction returns an action that populates the module status
// with version, platform, and source information.
func newReportStatusAction(cfg *moduleconfig.Config) actions.Fn {
	return func(_ context.Context, rr *odhtypes.ReconciliationRequest) error {
		obj, ok := rr.Instance.(*componentApi.MyModule)
		if !ok {
			return fmt.Errorf("instance is not a MyModule")
		}

		obj.Status.Module = componentApi.ModuleStatus{
			Version:     version.Version,
			BuildSource: version.Repo + "@" + version.Branch + "/" + version.Commit,
			Platform: componentApi.PlatformStatus{
				Name:    cfg.PlatformType,
				Version: cfg.PlatformVersion,
			},
		}

		var sources []componentApi.SourceStatus

		for _, m := range rr.Manifests {
			sources = append(sources, componentApi.SourceStatus{
				Path:     m.String(),
				Renderer: componentApi.SourceRendererKustomize,
			})
		}

		for _, t := range rr.Templates {
			sources = append(sources, componentApi.SourceStatus{
				Path:     t.Path,
				Renderer: componentApi.SourceRendererTemplate,
			})
		}

		for _, h := range rr.HelmCharts {
			sources = append(sources, componentApi.SourceStatus{
				Path:     h.Chart,
				Renderer: componentApi.SourceRendererHelm,
			})
		}

		obj.Status.Module.Sources = sources

		obj.Status.ConfigValues = map[string]string{
			moduleconfig.KeyPlatformType:    cfg.PlatformType,
			moduleconfig.KeyPlatformVersion: cfg.PlatformVersion,
		}

		return nil
	}
}
