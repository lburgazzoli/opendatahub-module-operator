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

package releases_test

import (
	"testing"

	. "github.com/onsi/gomega"
	common "github.com/opendatahub-io/odh-platform-utilities/api/common"

	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-datasciencepipelines-operator/pkg/releases"
)

func TestUpsert_Insert(t *testing.T) {
	g := NewWithT(t)
	status := &common.ComponentReleaseStatus{}

	releases.Upsert(status, common.ComponentRelease{Name: "platform", Version: "1.0.0"})

	g.Expect(status.Releases).To(HaveLen(1))
	g.Expect(status.Releases[0]).To(Equal(common.ComponentRelease{Name: "platform", Version: "1.0.0"}))
}

func TestUpsert_Update(t *testing.T) {
	g := NewWithT(t)
	status := &common.ComponentReleaseStatus{
		Releases: []common.ComponentRelease{
			{Name: "platform", Version: "1.0.0"},
		},
	}

	releases.Upsert(status, common.ComponentRelease{Name: "platform", Version: "2.0.0"})

	g.Expect(status.Releases).To(HaveLen(1))
	g.Expect(status.Releases[0].Version).To(Equal("2.0.0"))
}

func TestUpsert_SortedOrder(t *testing.T) {
	g := NewWithT(t)
	status := &common.ComponentReleaseStatus{}

	releases.Upsert(status, common.ComponentRelease{Name: "datasciencepipelines", Version: "1.0.0"})
	releases.Upsert(status, common.ComponentRelease{Name: "platform", Version: "3.5.0"})

	g.Expect(status.Releases).To(HaveLen(2))
	g.Expect(status.Releases[0].Name).To(Equal("datasciencepipelines"))
	g.Expect(status.Releases[1].Name).To(Equal("platform"))
}

func TestGet_Found(t *testing.T) {
	g := NewWithT(t)
	status := &common.ComponentReleaseStatus{
		Releases: []common.ComponentRelease{
			{Name: "platform", Version: "3.5.0"},
		},
	}

	r, ok := releases.Get(status, "platform")
	g.Expect(ok).To(BeTrue())
	g.Expect(r.Version).To(Equal("3.5.0"))
}

func TestGet_NotFound(t *testing.T) {
	g := NewWithT(t)
	status := &common.ComponentReleaseStatus{}

	_, ok := releases.Get(status, "platform")
	g.Expect(ok).To(BeFalse())
}

func TestParseVersion_Empty(t *testing.T) {
	g := NewWithT(t)
	v, err := releases.ParseVersion("")
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(v.String()).To(Equal("0.0.0"))
}

func TestParseVersion_Valid(t *testing.T) {
	g := NewWithT(t)
	v, err := releases.ParseVersion("3.5.0")
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(v.String()).To(Equal("3.5.0"))
}

func TestParseVersion_Invalid(t *testing.T) {
	g := NewWithT(t)
	_, err := releases.ParseVersion("not-a-version")
	g.Expect(err).To(HaveOccurred())
}
