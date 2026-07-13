package external

import (
	"context"
	"fmt"

	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlconfig "sigs.k8s.io/controller-runtime/pkg/client/config"

	modulemanager "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/pkg/manager"
	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/test/support"
	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/test/support/cluster/common"
)

type Cluster struct {
	base common.Base
}

func New(testCfg *support.Config) (common.Instance, error) {
	cfg, err := ctrlconfig.GetConfig()
	if err != nil {
		return nil, fmt.Errorf("getting kubeconfig: %w", err)
	}

	return NewFromConfig(cfg, testCfg)
}

func NewFromConfig(
	cfg *rest.Config,
	testCfg *support.Config,
) (common.Instance, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config is nil")
	}

	copiedCfg := rest.CopyConfig(cfg)
	scheme, err := modulemanager.NewScheme()
	if err != nil {
		return nil, fmt.Errorf("building scheme: %w", err)
	}
	cli, err := common.NewClient(copiedCfg, scheme)
	if err != nil {
		return nil, err
	}

	return &Cluster{
		base: common.NewBase(copiedCfg, cli, testCfg),
	}, nil
}

func (c *Cluster) Config() *rest.Config {
	return c.base.Config()
}

func (c *Cluster) Client() client.Client {
	return c.base.Client()
}

func (c *Cluster) Setup(ctx context.Context) error {
	return c.base.SetUp(ctx)
}

func (c *Cluster) Stop(ctx context.Context) error {
	return c.base.TearDown(ctx)
}
