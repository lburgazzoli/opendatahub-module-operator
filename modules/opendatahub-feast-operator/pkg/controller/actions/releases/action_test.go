package releases

import (
	"os"
	"path/filepath"
	"testing"

	componentApi "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-feast-operator/api/components/v1alpha1"
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
  - name: feastoperator
    version: 1.2.3
    repoUrl: https://example.com/feastoperator
  - name: ignored
    version: " "
    repoUrl: https://example.com/ignored
`), 0o600)
	g.Expect(err).To(Succeed())

	obj := &componentApi.FeastOperator{}
	rr := &types.ReconciliationRequest{Instance: obj}

	action := NewAction(WithMetadataFilePath(func(_ *types.ReconciliationRequest) string {
		return metadataPath
	}))

	g.Expect(action(t.Context(), rr)).To(Succeed())
	g.Expect(obj.GetReleaseStatus()).To(Equal(&common.ComponentReleaseStatus{
		Releases: []common.ComponentRelease{{
			Name:    "feastoperator",
			Version: "1.2.3",
		}},
	}))
}

func TestActionMissingMetadataIsNonFatal(t *testing.T) {
	g := NewWithT(t)

	obj := &componentApi.FeastOperator{}
	rr := &types.ReconciliationRequest{Instance: obj}

	action := NewAction(WithMetadataFilePath(func(_ *types.ReconciliationRequest) string {
		return filepath.Join(t.TempDir(), "missing.yaml")
	}))

	g.Expect(action(t.Context(), rr)).To(Succeed())
	g.Expect(obj.GetReleaseStatus()).To(Equal(&common.ComponentReleaseStatus{}))
}

func TestActionUsesProvidedStatus(t *testing.T) {
	g := NewWithT(t)

	obj := &componentApi.FeastOperator{}
	rr := &types.ReconciliationRequest{Instance: obj}

	expected := common.ComponentReleaseStatus{
		Releases: []common.ComponentRelease{{
			Name:    "cached",
			Version: "9.9.9",
			RepoURL: "https://example.com/cached",
		}},
	}
	action := NewAction(WithReleaseStatus(expected))

	g.Expect(action(t.Context(), rr)).To(Succeed())
	g.Expect(obj.GetReleaseStatus()).To(Equal(&expected))
}
