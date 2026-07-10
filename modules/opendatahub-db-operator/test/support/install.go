package support

import (
	"context"
	"fmt"
	"time"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// InstallCRD installs this module's CRDs into the provided cluster client and
// waits for each one to become established before moving to the next.
func InstallCRD(ctx context.Context, cli client.Client) error {
	installCRDs := []string{
		"config/crd/bases/infrastructure.opendatahub.io_databaseclaims.yaml",
		"config/crd/bases/infrastructure.opendatahub.io_databaseproviders.yaml",
		"config/crd/bases/infrastructure.opendatahub.io_schemaclaims.yaml",
		"config/crd/bases/services.platform.opendatahub.io_databaseservices.yaml",
	}

	for _, manifest := range installCRDs {
		manifestPath, err := ProjectFile(manifest)
		if err != nil {
			return fmt.Errorf("resolving CRD manifest path %q: %w", manifest, err)
		}
		obj, err := ApplyManifestFromFile(ctx, cli, manifestPath)
		if err != nil {
			return fmt.Errorf("installing CRD manifest %q: %w", manifest, err)
		}
		if err := wait.PollUntilContextTimeout(
			ctx,
			500*time.Millisecond,
			60*time.Second,
			true,
			func(ctx context.Context) (bool, error) {
				return IsCRDEstablished(ctx, cli, obj.GetName())
			},
		); err != nil {
			return fmt.Errorf("waiting for CRD %q to become established: %w", obj.GetName(), err)
		}
	}

	return nil
}

// IsCRDEstablished reports whether the named CRD has reached the Established condition.
func IsCRDEstablished(
	ctx context.Context,
	cli client.Client,
	name string,
) (bool, error) {
	crd := &apiextensionsv1.CustomResourceDefinition{}
	if err := cli.Get(ctx, client.ObjectKey{Name: name}, crd); err != nil {
		if apierrors.IsNotFound(err) {
			return false, nil
		}

		return false, fmt.Errorf("getting CRD %q: %w", name, err)
	}

	for _, cond := range crd.Status.Conditions {
		if cond.Type == apiextensionsv1.Established && cond.Status == apiextensionsv1.ConditionTrue {
			return true, nil
		}
	}

	return false, nil
}
