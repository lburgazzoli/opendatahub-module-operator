package releases

import (
	"os"
	"path/filepath"
	"testing"

	componentApi "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-ogx-operator/api/components/v1alpha1"
	common "github.com/opendatahub-io/odh-platform-utilities/api/common"
	"github.com/opendatahub-io/odh-platform-utilities/framework/controller/types"

	. "github.com/onsi/gomega"
)

func TestActionReadsMetadata(t *testing.T) {
	g := NewWithT(t)

	root := t.TempDir()
	metadataPath := filepath.Join(root, "component_metadata.yaml")
	err := os.WriteFile(metadataPath, []byte(`
releases:
  - name: ogx
    version: 1.2.3
  - name: ignored
    version: " "
`), 0o600)
	g.Expect(err).To(Succeed())

	obj := &componentApi.OGX{}
	rr := &types.ReconciliationRequest{Instance: obj}

	action := NewAction(WithMetadataFilePath(func(_ *types.ReconciliationRequest) string {
		return metadataPath
	}))

	g.Expect(action(t.Context(), rr)).To(Succeed())
	g.Expect(obj.GetReleaseStatus()).To(Equal(&common.ComponentReleaseStatus{
		Releases: []common.ComponentRelease{{
			Name:    "ogx",
			Version: "1.2.3",
		}},
	}))
}

func TestActionMissingMetadataIsNonFatal(t *testing.T) {
	g := NewWithT(t)

	obj := &componentApi.OGX{}
	rr := &types.ReconciliationRequest{Instance: obj}

	action := NewAction(WithMetadataFilePath(func(_ *types.ReconciliationRequest) string {
		return filepath.Join(t.TempDir(), "missing.yaml")
	}))

	g.Expect(action(t.Context(), rr)).To(Succeed())
	g.Expect(obj.GetReleaseStatus()).To(Equal(&common.ComponentReleaseStatus{}))
}
