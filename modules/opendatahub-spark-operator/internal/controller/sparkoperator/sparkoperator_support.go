package sparkoperator

import (
	"errors"
	"fmt"
	iofs "io/fs"
	"path"
	"sort"
	"strings"

	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-spark-operator/assets"
	moduleconfig "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-spark-operator/pkg/config"
	common "github.com/opendatahub-io/odh-platform-utilities/api/common"
	kfs "github.com/opendatahub-io/odh-platform-utilities/framework/render/kustomize/fs"
	"gopkg.in/yaml.v3"
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

	yamlData, err := iofs.ReadFile(assets.Manifests, metadataPath)
	if err != nil {
		if errors.Is(err, iofs.ErrNotExist) {
			return []common.ComponentRelease{m.cfg.ComponentRelease()}, nil
		}
		return nil, fmt.Errorf("reading metadata file: %w", err)
	}

	componentReleaseStatus := common.ComponentReleaseStatus{}
	if err := yaml.Unmarshal(yamlData, &componentReleaseStatus); err != nil {
		return nil, fmt.Errorf("unmarshaling metadata file: %w", err)
	}

	releases := make([]common.ComponentRelease, 0, len(componentReleaseStatus.Releases))
	for _, release := range componentReleaseStatus.Releases {
		version := strings.TrimSpace(release.Version)
		if version == "" {
			continue
		}

		releases = append(releases, common.ComponentRelease{
			Name:    release.Name,
			Version: version,
			RepoURL: release.RepoURL,
		})
	}

	releases = append(releases, m.cfg.ComponentRelease())
	sort.Slice(releases, func(i, j int) bool {
		return releases[i].Name < releases[j].Name
	})

	return releases, nil
}

func lookupPlatformRelease(status *common.ComponentReleaseStatus) (common.ComponentRelease, bool) {
	for _, r := range status.Releases {
		if r.Name == moduleconfig.ReleasePlatform {
			return r, true
		}
	}
	return common.ComponentRelease{}, false
}
