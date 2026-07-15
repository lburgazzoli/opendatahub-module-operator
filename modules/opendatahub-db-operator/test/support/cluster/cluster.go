package cluster

import (
	"context"
	"fmt"

	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/test/support"
	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/test/support/cluster/external"
	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/test/support/cluster/k3s"
	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/test/support/cluster/kind"
)

const (
	TypeExternal = support.ClusterTypeExternal
	TypeKind     = support.ClusterTypeKind
	TypeK3s      = support.ClusterTypeK3s
)

type Instance interface {
	Config() *rest.Config
	Client() client.Client
	Setup(ctx context.Context) error
	Stop(ctx context.Context) error
}

func New(ctx context.Context, cfg *support.Config, opts ...Option) (Instance, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config is nil")
	}

	clusterOpts := options{}
	for _, opt := range opts {
		if opt != nil {
			opt.applyOption(&clusterOpts)
		}
	}

	switch cfg.Cluster.Type {
	case TypeExternal:
		return external.New(cfg)
	case TypeKind:
		kindOpts := []kind.Option{}
		if clusterOpts.LogFn != nil {
			kindOpts = append(kindOpts, kind.WithLogFn(clusterOpts.LogFn))
		}
		if clusterOpts.Purge != nil {
			kindOpts = append(kindOpts, kind.WithPurge(*clusterOpts.Purge))
		}
		for _, createOption := range clusterOpts.CreateOptions {
			kindOpts = append(kindOpts, kind.WithCreateOption(createOption))
		}

		return kind.New(ctx, cfg, kindOpts...)
	case TypeK3s:
		return k3s.New(ctx, cfg)
	default:
		return nil, fmt.Errorf("unsupported cluster type %q", cfg.Cluster.Type)
	}
}
