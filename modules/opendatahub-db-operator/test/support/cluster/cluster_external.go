package cluster

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlconfig "sigs.k8s.io/controller-runtime/pkg/client/config"
)

type External struct {
	cfg    *rest.Config
	cli    client.Client
	scheme *runtime.Scheme
}

func NewExternal() (Instance, error) {
	cfg, err := ctrlconfig.GetConfig()
	if err != nil {
		return nil, fmt.Errorf("getting kubeconfig: %w", err)
	}

	return NewExternalFromConfig(cfg)
}

func NewExternalFromConfig(cfg *rest.Config) (Instance, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config is nil")
	}

	copiedCfg := rest.CopyConfig(cfg)
	scheme := newScheme()
	cli, err := newClient(copiedCfg, scheme)
	if err != nil {
		return nil, err
	}

	return &External{
		cfg:    copiedCfg,
		cli:    cli,
		scheme: scheme,
	}, nil
}

func (e *External) Config() *rest.Config {
	if e == nil || e.cfg == nil {
		return nil
	}

	return rest.CopyConfig(e.cfg)
}

func (e *External) Scheme() *runtime.Scheme {
	if e == nil {
		return nil
	}

	return e.scheme
}

func (e *External) Client() client.Client {
	if e == nil {
		return nil
	}

	return e.cli
}

func (e *External) Stop(_ context.Context) error {
	return nil
}
