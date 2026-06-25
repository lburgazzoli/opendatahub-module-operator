package sparkoperator

import (
	"sort"

	common "github.com/opendatahub-io/odh-platform-utilities/api/common"
)

func UpsertRelease(status *common.ComponentReleaseStatus, release common.ComponentRelease) {
	for i := range status.Releases {
		if status.Releases[i].Name == release.Name {
			status.Releases[i] = release
			return
		}
	}
	status.Releases = append(status.Releases, release)
	sort.Slice(status.Releases, func(i, j int) bool {
		return status.Releases[i].Name < status.Releases[j].Name
	})
}

func GetRelease(status *common.ComponentReleaseStatus, name string) (common.ComponentRelease, bool) {
	for _, r := range status.Releases {
		if r.Name == name {
			return r, true
		}
	}
	return common.ComponentRelease{}, false
}
