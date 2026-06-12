package releases

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	common "github.com/opendatahub-io/odh-platform-utilities/api/common"
	"github.com/opendatahub-io/odh-platform-utilities/framework/controller/actions"
	"github.com/opendatahub-io/odh-platform-utilities/framework/controller/types"
	"gopkg.in/yaml.v3"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

const ComponentMetadataFilename = "component_metadata.yaml"

type Action struct {
	metadataFilePathFn func(rr *types.ReconciliationRequest) string
	releaseStatus      *common.ComponentReleaseStatus
}

type ActionOpt func(*Action)

func WithMetadataFilePath(
	fn func(rr *types.ReconciliationRequest) string,
) ActionOpt {
	return func(a *Action) {
		a.metadataFilePathFn = fn
	}
}

func WithReleaseStatus(status common.ComponentReleaseStatus) ActionOpt {
	return func(a *Action) {
		copied := status
		a.releaseStatus = &copied
	}
}

func NewAction(opts ...ActionOpt) actions.Fn {
	action := Action{}
	for _, opt := range opts {
		opt(&action)
	}

	return action.run
}

func (a *Action) run(ctx context.Context, rr *types.ReconciliationRequest) error {
	if a.releaseStatus == nil {
		status, err := a.render(ctx, rr)
		if err != nil {
			return err
		}
		a.releaseStatus = &status
	}

	rr.Instance.SetReleaseStatus(*a.releaseStatus)

	return nil
}

func (a *Action) render(
	ctx context.Context,
	rr *types.ReconciliationRequest,
) (common.ComponentReleaseStatus, error) {
	log := logf.FromContext(ctx)

	metadataPath := a.metadataPath(rr)
	yamlData, err := os.ReadFile(metadataPath)
	if err != nil {
		if os.IsNotExist(err) {
			log.V(3).Info(
				"Metadata file not found, proceeding with empty releases",
				"metadataFilePath",
				metadataPath,
			)

			return common.ComponentReleaseStatus{}, nil
		}

		return common.ComponentReleaseStatus{}, fmt.Errorf(
			"error reading metadata file: %w",
			err,
		)
	}

	var status common.ComponentReleaseStatus
	if err := yaml.Unmarshal(yamlData, &status); err != nil {
		return common.ComponentReleaseStatus{}, fmt.Errorf(
			"error unmarshaling YAML: %w",
			err,
		)
	}

	filtered := make([]common.ComponentRelease, 0, len(status.Releases))
	for _, release := range status.Releases {
		version := strings.TrimSpace(release.Version)
		if version == "" {
			continue
		}

		filtered = append(filtered, common.ComponentRelease{
			Name:    release.Name,
			Version: version,
			RepoURL: release.RepoURL,
		})
	}

	status.Releases = filtered

	return status, nil
}

func (a *Action) metadataPath(rr *types.ReconciliationRequest) string {
	if a.metadataFilePathFn != nil {
		return a.metadataFilePathFn(rr)
	}

	kind := rr.Instance.GetObjectKind().GroupVersionKind().Kind
	if kind == "" {
		kind = reflect.TypeOf(rr.Instance).Elem().Name()
	}

	return filepath.Join(
		rr.ManifestsBasePath,
		strings.ToLower(kind),
		ComponentMetadataFilename,
	)
}
