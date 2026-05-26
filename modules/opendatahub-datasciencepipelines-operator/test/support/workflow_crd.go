package support

import (
	"context"
	"fmt"
	"time"

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

		if err := waitForWorkflowCRDState(ctx, cli, crdName, true); err != nil {
			return err
		}

		return nil
	} else if err != nil {
		return fmt.Errorf("loading CRD %s for restore: %w", crdName, err)
	}

	if state.Original == nil {
		// E2E and integration cleanup scripts remove test-managed workflow CRDs
		// after the suite finishes. Keeping them between subtests avoids races
		// where a slow CRD delete overlaps the next setup path.
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

func waitForWorkflowCRDState(
	ctx context.Context,
	cli client.Client,
	crdName string,
	shouldExist bool,
) error {
	waitCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		current := &apiextensionsv1.CustomResourceDefinition{}
		err := cli.Get(waitCtx, client.ObjectKey{Name: crdName}, current)

		if shouldExist {
			if err == nil {
				return nil
			}
		} else if k8serr.IsNotFound(err) {
			return nil
		}

		select {
		case <-waitCtx.Done():
			if shouldExist {
				return fmt.Errorf("waiting for CRD %s to exist: %w", crdName, waitCtx.Err())
			}

			return fmt.Errorf("waiting for CRD %s to be deleted: %w", crdName, waitCtx.Err())
		case <-ticker.C:
		}
	}
}
