package cluster

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"

	modulemanager "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/pkg/manager"
)

const DefaultK3sImage = "rancher/k3s:v1.32.9-k3s1"

// TestCluster exposes the minimum cluster primitives needed by tests without
// coupling callers to a specific backend implementation.
type TestCluster interface {
	Config() *rest.Config
	Scheme() *runtime.Scheme
	Client() (client.Client, error)
	Stop(ctx context.Context) error
}

func newScheme() *runtime.Scheme {
	return modulemanager.NewScheme()
}

func newClient(
	cfg *rest.Config,
	scheme *runtime.Scheme,
) (client.Client, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config is nil")
	}
	if scheme == nil {
		return nil, fmt.Errorf("scheme is nil")
	}

	cli, err := client.New(cfg, client.Options{Scheme: scheme})
	if err != nil {
		return nil, fmt.Errorf("creating client: %w", err)
	}

	return cli, nil
}
