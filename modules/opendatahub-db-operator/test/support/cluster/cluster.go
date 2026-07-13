package cluster

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlconfig "sigs.k8s.io/controller-runtime/pkg/client/config"

	modulemanager "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/pkg/manager"
)

type Type string

const (
	TypeExternal Type = "external"
	TypeK3s      Type = "k3s"
)

// Instance exposes the minimum cluster primitives needed by tests without
// coupling callers to a specific backend implementation.
type Instance interface {
	Config() *rest.Config
	Client() client.Client
	Stop(ctx context.Context) error
}

func New(ctx context.Context, clusterType Type) (Instance, error) {
	switch clusterType {
	case TypeExternal:
		return newExternal()
	case TypeK3s:
		return newK3s(ctx)
	default:
		return nil, fmt.Errorf("unsupported cluster type %q", clusterType)
	}
}

type external struct {
	cfg *rest.Config
	cli client.Client
}

func newExternal() (Instance, error) {
	cfg, err := ctrlconfig.GetConfig()
	if err != nil {
		return nil, fmt.Errorf("getting kubeconfig: %w", err)
	}

	return newExternalFromConfig(cfg)
}

func newExternalFromConfig(cfg *rest.Config) (Instance, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config is nil")
	}

	copiedCfg := rest.CopyConfig(cfg)
	scheme, err := newScheme()
	if err != nil {
		return nil, err
	}
	cli, err := newClient(copiedCfg, scheme)
	if err != nil {
		return nil, err
	}

	return &external{
		cfg: copiedCfg,
		cli: cli,
	}, nil
}

func (e *external) Config() *rest.Config {
	if e == nil || e.cfg == nil {
		return nil
	}

	return rest.CopyConfig(e.cfg)
}

func (e *external) Client() client.Client {
	if e == nil {
		return nil
	}

	return e.cli
}

func (e *external) Stop(_ context.Context) error {
	return nil
}

func newScheme() (*runtime.Scheme, error) {
	scheme, err := modulemanager.NewScheme()
	if err != nil {
		return nil, fmt.Errorf("building scheme: %w", err)
	}

	return scheme, nil
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
