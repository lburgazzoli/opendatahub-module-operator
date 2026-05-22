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
	"github.com/opendatahub-io/opendatahub-operator/v2/api/common"
	"github.com/opendatahub-io/opendatahub-operator/v2/pkg/cluster"
	"github.com/opendatahub-io/opendatahub-operator/v2/pkg/controller/status"
	"github.com/opendatahub-io/opendatahub-operator/v2/pkg/controller/types"

	componentApi "github.com/lburgazzoli/opendatahub-module-operator/api/components/v1alpha1"
)

const (
	componentName = componentApi.MyModuleComponentName

	overlayRhoai = "overlays/rhoai"
	overlayODH   = "overlays/odh"
)

var (
	// ManifestsSourcePath maps platform variants to kustomize overlay paths.
	ManifestsSourcePath = map[common.Platform]string{
		cluster.SelfManagedRhoai: overlayRhoai,
		cluster.ManagedRhoai:     overlayRhoai,
		cluster.OpenDataHub:      overlayODH,
	}

	conditionTypes = []string{
		status.ConditionDeploymentsAvailable,
	}
)

func manifestPath(basePath string, p common.Platform) types.ManifestInfo {
	sourcePath, ok := ManifestsSourcePath[p]
	if !ok {
		// Default to ODH overlay for unknown/undetected platforms (e.g., local development).
		sourcePath = ManifestsSourcePath[cluster.OpenDataHub]
	}

	return types.ManifestInfo{
		Path:       basePath,
		ContextDir: componentName,
		SourcePath: sourcePath,
	}
}
