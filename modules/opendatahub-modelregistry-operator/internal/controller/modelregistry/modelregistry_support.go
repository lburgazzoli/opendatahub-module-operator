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
	iofs "io/fs"
	"sort"

	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/kustomize/kyaml/filesys"

	componentApi "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-modelregistry-operator/api/components/v1alpha1"
	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-modelregistry-operator/assets"
	moduleconfig "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-modelregistry-operator/pkg/config"
	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-modelregistry-operator/pkg/resources/gvk"
	common "github.com/opendatahub-io/odh-platform-utilities/api/common"
	odhtypes "github.com/opendatahub-io/odh-platform-utilities/framework/controller/types"
	kfs "github.com/opendatahub-io/odh-platform-utilities/framework/render/kustomize/fs"
	"sigs.k8s.io/yaml"
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
	raw, err := iofs.ReadFile(assets.Manifests, componentMetadataPath)
	if err != nil {
		return nil, fmt.Errorf("read component metadata: %w", err)
	}

	var metadata struct {
		Releases []common.ComponentRelease `json:"releases"`
	}

	if err := yaml.Unmarshal(raw, &metadata); err != nil {
		return nil, fmt.Errorf("unmarshal component metadata: %w", err)
	}

	releases := append(metadata.Releases, m.cfg.ComponentRelease())

	sort.Slice(releases, func(i, j int) bool {
		return releases[i].Name < releases[j].Name
	})

	return releases, nil
}

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

// computeKustomizeVariables returns the gateway and routing kustomize variables from the CR spec.
func (m *Module) computeKustomizeVariables(mr *componentApi.ModelRegistry) (map[string]string, error) {
	var domain string
	if mr.Spec.Gateway != nil {
		domain = mr.Spec.Gateway.Domain
	}

	if domain == "" {
		return nil, errors.New(
			"gateway domain is missing for ModelRegistry; set spec.gateway.domain to the cluster ingress domain")
	}

	return map[string]string{
		"GATEWAY_DOMAIN":      domain,
		"GATEWAY_NAME":        defaultGatewayName,
		"GATEWAY_NAMESPACE":   gatewayNamespace,
		"HTTPROUTE_NAMESPACE": m.cfg.ApplicationsNamespace,
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
