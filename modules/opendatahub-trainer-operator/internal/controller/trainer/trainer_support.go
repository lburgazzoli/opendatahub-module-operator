package trainer

import (
	"context"
	"fmt"

	k8serr "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-trainer-operator/pkg/resources/gvk"
	"github.com/opendatahub-io/opendatahub-operator/v2/pkg/cluster"
	"github.com/opendatahub-io/opendatahub-operator/v2/pkg/controller/precondition"
	"github.com/opendatahub-io/opendatahub-operator/v2/pkg/controller/status"
	"github.com/opendatahub-io/opendatahub-operator/v2/pkg/controller/types"
)

const (
	LegacyComponentName = componentName

	jobSetOperatorCRName  = "cluster"
	jobSetOperatorCRDName = "jobsetoperators.operator.openshift.io"
	jobSetCRDName         = "jobsets.jobset.x-k8s.io"
)

var (
	imageParamMap = map[string]string{
		"odh-kubeflow-trainer-controller-image":           "RELATED_IMAGE_ODH_TRAINER_IMAGE",
		"odh-training-cuda128-torch29-py312-image":        "RELATED_IMAGE_ODH_TRAINING_CUDA128_TORCH29_PY312_IMAGE",
		"odh-training-rocm64-torch29-py312-image":         "RELATED_IMAGE_ODH_TRAINING_ROCM64_TORCH29_PY312_IMAGE",
		"odh-th06-cuda130-torch210-py312-image":           "RELATED_IMAGE_ODH_TH06_CUDA130_TORCH210_PY312_IMAGE",
		"odh-th06-rocm64-torch291-py312-image":            "RELATED_IMAGE_ODH_TH06_ROCM64_TORCH291_PY312_IMAGE",
		"odh-th06-cpu-torch210-py312-image":               "RELATED_IMAGE_ODH_TH06_CPU_TORCH210_PY312_IMAGE",
		"odh-training-universal-workbench-image-cuda-3-4": "RELATED_IMAGE_ODH_TH06_CUDA130_TORCH210_PY312_IMAGE",
		"odh-training-universal-workbench-image-rocm-3-4": "RELATED_IMAGE_ODH_TH06_ROCM64_TORCH291_PY312_IMAGE",
		"odh-training-universal-workbench-image-cpu-3-4":  "RELATED_IMAGE_ODH_TH06_CPU_TORCH210_PY312_IMAGE",
	}

	conditionTypes = []string{
		status.ConditionDeploymentsAvailable,
		status.ConditionDependenciesAvailable,
	}
)

func manifestPath(basePath string) types.ManifestInfo {
	return types.ManifestInfo{
		Path:       basePath,
		ContextDir: componentName,
		SourcePath: "rhoai",
	}
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

func (m *Module) checkPreConditions(ctx context.Context, rr *types.ReconciliationRequest) (precondition.CheckResult, error) {
	jobSetOperatorCRDExists, err := cluster.HasCRD(ctx, rr.Client, gvk.JobSetOperatorV1)
	switch {
	case err != nil:
		return precondition.CheckResult{}, fmt.Errorf("checking JobSet operator CRD: %w", err)
	case !jobSetOperatorCRDExists:
		return precondition.CheckResult{
			Pass:    false,
			Message: status.JobSetOperatorNotInstalledMessage,
		}, nil
	}

	switch err := getSingletonUnstructured(ctx, m.singletonReader(rr), gvk.JobSetOperatorV1, "jobsetoperators"); {
	case k8serr.IsNotFound(err):
		return precondition.CheckResult{
			Pass:    false,
			Message: status.JobSetOperatorCRNotFoundMessage,
		}, nil
	case err != nil:
		return precondition.CheckResult{}, fmt.Errorf("checking JobSetOperator CR: %w", err)
	}

	return precondition.CheckResult{Pass: true}, nil
}

// checkJobSetCRD verifies that the JobSet workload CRD exists before the
// trainer manifests are rendered and applied.
func (m *Module) checkJobSetCRD(ctx context.Context, rr *types.ReconciliationRequest) (precondition.CheckResult, error) {
	jobSetCRDExists, err := cluster.HasCRD(ctx, rr.Client, gvk.JobSetv1alpha2)
	switch {
	case err != nil:
		return precondition.CheckResult{}, fmt.Errorf("checking JobSet CRD: %w", err)
	case !jobSetCRDExists:
		return precondition.CheckResult{
			Pass:    false,
			Message: status.JobSetCRDMissingMessage,
		}, nil
	}

	return precondition.CheckResult{Pass: true}, nil
}
