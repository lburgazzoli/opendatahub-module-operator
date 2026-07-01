package trainer

import (
	"context"
	"errors"
	"fmt"
	iofs "io/fs"
	"path"

	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-trainer-operator/assets"
	common "github.com/opendatahub-io/odh-platform-utilities/api/common"
	fwreleases "github.com/opendatahub-io/odh-platform-utilities/framework/controller/actions/status/releases"
	kfs "github.com/opendatahub-io/odh-platform-utilities/framework/render/kustomize/fs"
	k8serr "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/kustomize/kyaml/filesys"

	moduleconfig "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-trainer-operator/pkg/config"
	module "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-trainer-operator/pkg/module"
	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-trainer-operator/pkg/resources/gvk"
	fwcluster "github.com/opendatahub-io/odh-platform-utilities/framework/cluster"
	"github.com/opendatahub-io/odh-platform-utilities/framework/controller/actions/status/deployments"
	"github.com/opendatahub-io/odh-platform-utilities/framework/controller/types"
)

const (
	LegacyComponentName = componentName

	// JobSetOperator is defined by OpenShift as a singleton named "cluster".
	jobSetOperatorCRName  = "cluster"
	jobSetOperatorCRDName = "jobsetoperators.operator.openshift.io"
	jobSetCRDName         = "jobsets.jobset.x-k8s.io"
)

var (
	conditionTypes = []string{
		deployments.DefaultConditionType,
		module.ConditionDependenciesAvailable,
	}
)

type preconditionResult struct {
	Pass    bool
	Message string
}

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

func (m *Module) singletonReader(rr *types.ReconciliationRequest) client.Reader {
	if m.apiReader != nil {
		return m.apiReader
	}

	return rr.Client
}

// getSingletonUnstructured reads a cluster-scoped singleton via a Reader so the
// precondition can bypass the informer cache when the dynamic watch for the
// dependency GVK has not been registered yet.
func getSingletonUnstructured(
	ctx context.Context,
	reader client.Reader,
	resourceGVK schema.GroupVersionKind,
	resourceName string,
) error {
	list := &unstructured.UnstructuredList{}
	list.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   resourceGVK.Group,
		Version: resourceGVK.Version,
		Kind:    resourceGVK.Kind + "List",
	})

	if err := reader.List(ctx, list); err != nil {
		return err
	}

	switch len(list.Items) {
	case 1:
		return nil
	case 0:
		return k8serr.NewNotFound(
			schema.GroupResource{
				Group:    resourceGVK.Group,
				Resource: resourceName,
			},
			"",
		)
	default:
		return fmt.Errorf(
			"failed to get a valid %s instance, expected to find 1 instance, found %d",
			resourceGVK,
			len(list.Items),
		)
	}
}

func (m *Module) checkPreConditions(ctx context.Context, rr *types.ReconciliationRequest) (preconditionResult, error) {
	jobSetOperatorCRDExists, err := fwcluster.HasCRD(ctx, rr.Client, gvk.JobSetOperatorV1)
	switch {
	case err != nil:
		return preconditionResult{}, fmt.Errorf("checking JobSet operator CRD: %w", err)
	case !jobSetOperatorCRDExists:
		return preconditionResult{
			Pass:    false,
			Message: module.JobSetOperatorNotInstalledMessage,
		}, nil
	}

	switch err := getSingletonUnstructured(ctx, m.singletonReader(rr), gvk.JobSetOperatorV1, "jobsetoperators"); {
	case k8serr.IsNotFound(err):
		return preconditionResult{
			Pass:    false,
			Message: module.JobSetOperatorCRNotFoundMessage,
		}, nil
	case err != nil:
		return preconditionResult{}, fmt.Errorf("checking JobSetOperator CR: %w", err)
	}

	return preconditionResult{Pass: true}, nil
}

// checkJobSetCRD verifies that the JobSet workload CRD exists before the
// trainer manifests are rendered and applied.
func (m *Module) checkJobSetCRD(ctx context.Context, rr *types.ReconciliationRequest) (preconditionResult, error) {
	jobSetCRDExists, err := fwcluster.HasCRD(ctx, rr.Client, gvk.JobSetv1alpha2)
	switch {
	case err != nil:
		return preconditionResult{}, fmt.Errorf("checking JobSet CRD: %w", err)
	case !jobSetCRDExists:
		return preconditionResult{
			Pass:    false,
			Message: module.JobSetCRDMissingMessage,
		}, nil
	}

	return preconditionResult{Pass: true}, nil
}

func lookupPlatformRelease(status *common.ComponentReleaseStatus) (common.ComponentRelease, bool) {
	for _, r := range status.Releases {
		if r.Name == moduleconfig.ReleasePlatform {
			return r, true
		}
	}
	return common.ComponentRelease{}, false
}
