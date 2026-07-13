package addons

import (
	"context"
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"

	support "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/test/support"
	supporthelm "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/test/support/helm"
)

const (
	certManagerNamespace = "cert-manager"
	certManagerRelease   = "cert-manager"
	certManagerChart     = "cert-manager"
	certManagerRepoURL   = "https://charts.jetstack.io"
	certManagerCRDName   = "certificates.cert-manager.io"
)

func EnsureCertManager(
	ctx context.Context,
	restCfg *rest.Config,
	cli client.Client,
) error {
	if cli == nil {
		return fmt.Errorf("client is nil")
	}
	if restCfg == nil {
		return fmt.Errorf("rest config is nil")
	}

	present, err := support.HasCRD(ctx, cli, certManagerCRDName)
	if err != nil {
		return err
	}
	if present {
		return nil
	}

	helmClient, err := supporthelm.New(restCfg)
	if err != nil {
		return fmt.Errorf("creating helm client: %w", err)
	}
	if err := helmClient.Install(ctx,
		supporthelm.WithChart(certManagerChart),
		supporthelm.WithChartRepoURL(certManagerRepoURL),
		supporthelm.WithReleaseName(certManagerRelease),
		supporthelm.WithNamespace(certManagerNamespace),
		supporthelm.WithValue("crds.enabled", "true"),
	); err != nil {
		return fmt.Errorf("installing cert-manager: %w", err)
	}

	if err := wait.PollUntilContextTimeout(
		ctx,
		time.Second,
		2*time.Minute,
		true,
		func(ctx context.Context) (bool, error) {
			return support.HasCRD(ctx, cli, certManagerCRDName)
		},
	); err != nil {
		return fmt.Errorf("waiting for cert-manager CRDs: %w", err)
	}

	return nil
}
