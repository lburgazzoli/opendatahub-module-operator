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
	scheme *runtime.Scheme
}

func NewExternal() (TestCluster, error) {
	cfg, err := ctrlconfig.GetConfig()
	if err != nil {
		return nil, fmt.Errorf("getting kubeconfig: %w", err)
	}

	return NewExternalFromConfig(cfg)
}

func NewExternalFromConfig(cfg *rest.Config) (TestCluster, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config is nil")
	}

	return &External{
		cfg:    rest.CopyConfig(cfg),
		scheme: newScheme(),
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

func (e *External) Client() (client.Client, error) {
	if e == nil {
		return nil, fmt.Errorf("external cluster is nil")
	}

	return newClient(e.cfg, e.scheme)
}

func (e *External) Stop(_ context.Context) error {
	return nil
}
