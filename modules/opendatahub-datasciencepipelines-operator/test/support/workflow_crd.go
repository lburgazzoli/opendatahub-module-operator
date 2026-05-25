package support

import (
	"context"
	"fmt"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	k8serr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type WorkflowCRDState struct {
	Original *apiextensionsv1.CustomResourceDefinition
}

func CaptureWorkflowCRDState(
	ctx context.Context,
	cli client.Client,
	crdName string,
) (WorkflowCRDState, error) {
	crd := &apiextensionsv1.CustomResourceDefinition{}
	err := cli.Get(ctx, client.ObjectKey{Name: crdName}, crd)
	if k8serr.IsNotFound(err) {
		return WorkflowCRDState{}, nil
	}
	if err != nil {
		return WorkflowCRDState{}, fmt.Errorf("getting CRD %s: %w", crdName, err)
	}

	return WorkflowCRDState{
		Original: sanitizeWorkflowCRD(crd),
	}, nil
}

func RestoreWorkflowCRDState(
	ctx context.Context,
	cli client.Client,
	crdName string,
	state WorkflowCRDState,
	testManagedLabel string,
	testManagedValue string,
) error {
	current := &apiextensionsv1.CustomResourceDefinition{}
	err := cli.Get(ctx, client.ObjectKey{Name: crdName}, current)
	if k8serr.IsNotFound(err) {
		if state.Original == nil {
			return nil
		}

		original := state.Original.DeepCopy()
		if err := cli.Create(ctx, original); err != nil {
			return fmt.Errorf("recreating original CRD %s: %w", crdName, err)
		}

		return nil
	} else if err != nil {
		return fmt.Errorf("loading CRD %s for restore: %w", crdName, err)
	}

	if state.Original == nil {
		if current.Labels[testManagedLabel] != testManagedValue {
			return nil
		}
		if err := cli.Delete(ctx, current); err != nil && !k8serr.IsNotFound(err) {
			return fmt.Errorf("deleting test-managed CRD %s: %w", crdName, err)
		}
		return nil
	}

	current.Labels = cloneLabels(state.Original.Labels)
	if err := cli.Update(ctx, current); err != nil {
		return fmt.Errorf("restoring CRD %s labels: %w", crdName, err)
	}

	return nil
}

func cloneLabels(labels map[string]string) map[string]string {
	if labels == nil {
		return nil
	}

	cloned := make(map[string]string, len(labels))
	for key, value := range labels {
		cloned[key] = value
	}

	return cloned
}

func sanitizeWorkflowCRD(crd *apiextensionsv1.CustomResourceDefinition) *apiextensionsv1.CustomResourceDefinition {
	sanitized := crd.DeepCopy()
	sanitized.ResourceVersion = ""
	sanitized.UID = ""
	sanitized.Generation = 0
	sanitized.CreationTimestamp = metav1.Time{}
	sanitized.DeletionTimestamp = nil
	sanitized.DeletionGracePeriodSeconds = nil
	sanitized.ManagedFields = nil

	return sanitized
}
