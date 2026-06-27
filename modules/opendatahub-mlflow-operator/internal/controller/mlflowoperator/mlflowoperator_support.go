package mlflowoperator

import (
	"fmt"

	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-mlflow-operator/assets"
	moduleconfig "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-mlflow-operator/pkg/config"
	common "github.com/opendatahub-io/odh-platform-utilities/api/common"
	kfs "github.com/opendatahub-io/odh-platform-utilities/framework/render/kustomize/fs"
	"sigs.k8s.io/kustomize/kyaml/filesys"
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

func lookupPlatformRelease(status *common.ComponentReleaseStatus) (common.ComponentRelease, bool) {
	for _, r := range status.Releases {
		if r.Name == moduleconfig.ReleasePlatform {
			return r, true
		}
	}
	return common.ComponentRelease{}, false
}
