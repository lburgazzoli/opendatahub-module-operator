package common

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/test/support"
	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/test/support/addons"
	supportlogger "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/test/support/logger"
	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/test/support/reaper"
)

// Instance exposes the minimum cluster primitives needed by tests without
// coupling callers to a specific backend implementation.
type Instance interface {
	Config() *rest.Config
	Client() client.Client
	Setup(ctx context.Context) error
	Stop(ctx context.Context) error
}

type Base struct {
	cfg     *rest.Config
	cli     client.Client
	testCfg *support.Config

	operatorLogs *supportlogger.Handler
}

func NewBase(
	cfg *rest.Config,
	cli client.Client,
	testCfg *support.Config,
) Base {
	var copiedTestCfg *support.Config
	if testCfg != nil {
		cfgCopy := *testCfg
		copiedTestCfg = &cfgCopy
	}

	return Base{
		cfg:     cfg,
		cli:     cli,
		testCfg: copiedTestCfg,
	}
}

func (b *Base) Config() *rest.Config {
	if b == nil || b.cfg == nil {
		return nil
	}

	return rest.CopyConfig(b.cfg)
}

func (b *Base) Client() client.Client {
	if b == nil {
		return nil
	}

	return b.cli
}

func (b *Base) SetUp(ctx context.Context) error {
	if b == nil || b.cli == nil {
		return fmt.Errorf("client is nil")
	}
	if b.testCfg == nil {
		return fmt.Errorf("test config is nil")
	}
	testCfg := b.testCfg

	r, err := reaper.New(b.cli)
	if err != nil {
		return fmt.Errorf("creating reaper: %w", err)
	}
	if err := r.Run(ctx); err != nil {
		return fmt.Errorf("cleaning fixtures: %w", err)
	}

	if err := addons.EnsureCertManager(ctx, b.cfg, b.cli); err != nil {
		return fmt.Errorf("ensuring cert-manager: %w", err)
	}

	if testCfg.Operator.Install {
		if err := b.InstallOperator(ctx); err != nil {
			return err
		}
	}

	return nil
}

func (b *Base) TearDown(ctx context.Context) error {
	if b == nil {
		return nil
	}

	return b.UninstallOperator(ctx)
}

func NewClient(
	cfg *rest.Config,
	scheme *runtime.Scheme,
) (client.Client, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config is nil")
	}
	if scheme == nil {
		return nil, fmt.Errorf("scheme is nil")
	}

	cli, err := client.New(cfg, client.Options{
		Scheme: scheme,
	})
	if err != nil {
		return nil, fmt.Errorf("creating client: %w", err)
	}

	return cli, nil
}
