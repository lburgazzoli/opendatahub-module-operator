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

	"sigs.k8s.io/kustomize/kyaml/filesys"

	componentApi "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-modelregistry-operator/api/components/v1alpha1"
	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-modelregistry-operator/assets"
	moduleconfig "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-modelregistry-operator/pkg/config"
	common "github.com/opendatahub-io/odh-platform-utilities/api/common"
	fwreleases "github.com/opendatahub-io/odh-platform-utilities/framework/controller/actions/status/releases"
	kfs "github.com/opendatahub-io/odh-platform-utilities/framework/render/kustomize/fs"
)

func newKustomizeFS() (filesys.FileSystem, error) {
	baseKustomizeFS, err := kfs.NewFromIOFS(assets.Manifests, "")
	if err != nil {
		return nil, fmt.Errorf("creating base render filesystem: %w", err)
	}

	kustomizeFS, err := kfs.NewUnionFs(baseKustomizeFS)
	if err != nil {
		return nil, fmt.Errorf("creating render filesystem: %w", err)
	}

	return kustomizeFS, nil
}

func (m *Module) loadReleases() ([]common.ComponentRelease, error) {
	releases, err := fwreleases.ReadComponentReleases(assets.Manifests, componentMetadataPath)
	if err != nil {
		return nil, fmt.Errorf("read component metadata: %w", err)
	}

	return fwreleases.NormalizeComponentReleases(append(releases, m.cfg.ComponentRelease())), nil
}

// computeRuntimeParams returns the runtime params written to each resolved params file.
func (m *Module) computeRuntimeParams(mr *componentApi.ModelRegistry) (map[string]string, error) {
	var domain string
	if mr.Spec.Gateway != nil {
		domain = mr.Spec.Gateway.Domain
	}

	if domain == "" {
		return nil, errors.New(
			"gateway domain is missing for ModelRegistry; set spec.gateway.domain to the cluster ingress domain")
	}

	return map[string]string{
		"GATEWAY_DOMAIN":       domain,
		"GATEWAY_NAME":         defaultGatewayName,
		"GATEWAY_NAMESPACE":    gatewayNamespace,
		"HTTPROUTE_NAMESPACE":  m.cfg.ApplicationsNamespace,
		"REGISTRIES_NAMESPACE": mr.Spec.RegistriesNamespace,
	}, nil
}

func lookupPlatformRelease(status *common.ComponentReleaseStatus) (common.ComponentRelease, bool) {
	for _, r := range status.Releases {
		if r.Name == moduleconfig.ReleasePlatform {
			return r, true
		}
	}
	return common.ComponentRelease{}, false
}
