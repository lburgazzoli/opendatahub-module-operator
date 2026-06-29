package ogx

import (
	"errors"
	"fmt"
	iofs "io/fs"
	"path"

	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-ogx-operator/assets"
	moduleconfig "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-ogx-operator/pkg/config"
	common "github.com/opendatahub-io/odh-platform-utilities/api/common"
	fwreleases "github.com/opendatahub-io/odh-platform-utilities/framework/controller/actions/status/releases"
	kfs "github.com/opendatahub-io/odh-platform-utilities/framework/render/kustomize/fs"
	"sigs.k8s.io/kustomize/kyaml/filesys"
)

const componentMetadataFilename = "component_metadata.yaml"

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
	metadataPath := path.Join("manifests", componentName, componentMetadataFilename)

	releases, err := fwreleases.ReadComponentReleases(assets.Manifests, metadataPath)
	if err != nil {
		if errors.Is(err, iofs.ErrNotExist) {
			return fwreleases.NormalizeComponentReleases([]common.ComponentRelease{m.cfg.ComponentRelease()}), nil
		}
		return nil, fmt.Errorf("reading metadata file: %w", err)
	}

	return fwreleases.NormalizeComponentReleases(append(releases, m.cfg.ComponentRelease())), nil
}

func lookupPlatformRelease(status *common.ComponentReleaseStatus) (common.ComponentRelease, bool) {
	for _, r := range status.Releases {
		if r.Name == moduleconfig.ReleasePlatform {
			return r, true
		}
	}
	return common.ComponentRelease{}, false
}
