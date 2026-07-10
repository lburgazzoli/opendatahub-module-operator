package cluster

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"

	modulemanager "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/pkg/manager"
)

type Type string

const (
	TypeExternal Type = "external"
	TypeK3s      Type = "k3s"
)

// TestCluster exposes the minimum cluster primitives needed by tests without
// coupling callers to a specific backend implementation.
type TestCluster interface {
	Config() *rest.Config
	Scheme() *runtime.Scheme
	Client() (client.Client, error)
	Stop(ctx context.Context) error
}

func New(ctx context.Context, clusterType Type) (TestCluster, error) {
	switch clusterType {
	case TypeExternal:
		return NewExternal()
	case TypeK3s:
		return NewK3s(ctx)
	default:
		return nil, fmt.Errorf("unsupported cluster type %q", clusterType)
	}
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
