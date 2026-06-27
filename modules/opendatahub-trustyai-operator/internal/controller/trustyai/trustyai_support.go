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

package trustyai

import (
	"fmt"

	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-trustyai-operator/assets"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/kustomize/kyaml/filesys"

	moduleconfig "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-trustyai-operator/pkg/config"
	modulegvk "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-trustyai-operator/pkg/resources/gvk"
	common "github.com/opendatahub-io/odh-platform-utilities/api/common"
	kfs "github.com/opendatahub-io/odh-platform-utilities/framework/render/kustomize/fs"
	fwresources "github.com/opendatahub-io/odh-platform-utilities/framework/resources"
)

const (
	InferenceServicesCRDName = "inferenceservices.serving.kserve.io"
	KserveModuleCRDName      = "kserves.components.platform.opendatahub.io"
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

// kserveUnstructured returns an unstructured object typed as the Kserve CR GVK,
// used for watching Kserve CR lifecycle events without importing the kserve module types.
func kserveUnstructured() *unstructured.Unstructured {
	return fwresources.GvkToUnstructured(modulegvk.Kserve)
}

// createdOrDeletedNamed returns a predicate that fires only on create/delete events for the named object.
func createdOrDeletedNamed(name string) predicate.Funcs {
	return predicate.Funcs{
		CreateFunc:  func(e event.CreateEvent) bool { return e.Object.GetName() == name },
		UpdateFunc:  func(e event.UpdateEvent) bool { return false },
		DeleteFunc:  func(e event.DeleteEvent) bool { return e.Object.GetName() == name },
		GenericFunc: func(e event.GenericEvent) bool { return false },
	}
}

func lookupPlatformRelease(status *common.ComponentReleaseStatus) (common.ComponentRelease, bool) {
	for _, r := range status.Releases {
		if r.Name == moduleconfig.ReleasePlatform {
			return r, true
		}
	}
	return common.ComponentRelease{}, false
}
