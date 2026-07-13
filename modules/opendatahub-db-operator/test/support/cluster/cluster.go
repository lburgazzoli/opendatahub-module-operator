package cluster

import (
	"context"
	"fmt"

	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/test/support"
	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/test/support/cluster/external"
	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/test/support/cluster/k3s"
)

const (
	TypeExternal = support.ClusterTypeExternal
	TypeK3s      = support.ClusterTypeK3s
)

type Instance interface {
	Config() *rest.Config
	Client() client.Client
	Setup(ctx context.Context) error
	Stop(ctx context.Context) error
}

func New(ctx context.Context, cfg *support.Config) (Instance, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config is nil")
	}

	switch cfg.Cluster.Type {
	case TypeExternal:
		return external.New(cfg)
	case TypeK3s:
		return k3s.New(ctx, cfg)
	default:
		return nil, fmt.Errorf("unsupported cluster type %q", cfg.Cluster.Type)
	}
}
