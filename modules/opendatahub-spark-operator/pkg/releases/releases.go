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

package releases

import (
	"sort"

	"github.com/blang/semver/v4"
	common "github.com/opendatahub-io/odh-platform-utilities/api/common"
)

const Platform = "platform"

// Upsert inserts or updates a release by name and keeps the list sorted by name.
func Upsert(status *common.ComponentReleaseStatus, release common.ComponentRelease) {
	for i := range status.Releases {
		if status.Releases[i].Name == release.Name {
			status.Releases[i] = release
			return // position unchanged, no re-sort needed
		}
	}
	status.Releases = append(status.Releases, release)
	sort.Slice(status.Releases, func(i, j int) bool {
		return status.Releases[i].Name < status.Releases[j].Name
	})
}

// Get returns the release with the given name, or (zero, false) if absent.
func Get(status *common.ComponentReleaseStatus, name string) (common.ComponentRelease, bool) {
	for _, r := range status.Releases {
		if r.Name == name {
			return r, true
		}
	}
	return common.ComponentRelease{}, false
}

// ParseVersion parses a semver version string, returning zero semver for empty input.
// Returns an error if the string is non-empty but not a valid semver.
func ParseVersion(version string) (semver.Version, error) {
	if version == "" {
		return semver.Version{}, nil
	}
	return semver.ParseTolerant(version)
}
