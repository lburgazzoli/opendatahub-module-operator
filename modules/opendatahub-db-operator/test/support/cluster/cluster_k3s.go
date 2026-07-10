package cluster

import (
	"context"
	"fmt"

	"github.com/testcontainers/testcontainers-go"
	tck3s "github.com/testcontainers/testcontainers-go/modules/k3s"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const DefaultK3sImage = "rancher/k3s:v1.36.2-k3s1"

type K3sOption func(*K3sOptions)

type K3sOptions struct {
	Image       string
	Customizers []testcontainers.ContainerCustomizer
}

type K3s struct {
	cfg       *rest.Config
	cli       client.Client
	container *tck3s.K3sContainer
	scheme    *runtime.Scheme
}

func NewK3s(ctx context.Context, opts ...K3sOption) (Instance, error) {
	if ctx == nil {
		return nil, fmt.Errorf("context is nil")
	}

	options := defaultK3sOptions()
	for _, opt := range opts {
		if opt != nil {
			opt(&options)
		}
	}
	if options.Image == "" {
		return nil, fmt.Errorf("image is empty")
	}

	container, err := tck3s.Run(ctx, options.Image, options.Customizers...)
	if err != nil {
		return nil, fmt.Errorf("starting k3s container: %w", err)
	}

	kubeConfigYAML, err := container.GetKubeConfig(ctx)
	if err != nil {
		_ = testcontainers.TerminateContainer(container)
		return nil, fmt.Errorf("reading kubeconfig from k3s container: %w", err)
	}

	cfg, err := clientcmd.RESTConfigFromKubeConfig(kubeConfigYAML)
	if err != nil {
		_ = testcontainers.TerminateContainer(container)
		return nil, fmt.Errorf("creating rest config from kubeconfig: %w", err)
	}

	scheme := newScheme()
	cli, err := newClient(cfg, scheme)
	if err != nil {
		_ = testcontainers.TerminateContainer(container)
		return nil, err
	}

	return &K3s{
		cfg:       cfg,
		cli:       cli,
		container: container,
		scheme:    scheme,
	}, nil
}

func WithK3sImage(image string) K3sOption {
	return func(opts *K3sOptions) {
		if opts == nil || image == "" {
			return
		}

		opts.Image = image
	}
}

func WithContainerCustomizer(customizer testcontainers.ContainerCustomizer) K3sOption {
	return func(opts *K3sOptions) {
		if opts == nil || customizer == nil {
			return
		}

		opts.Customizers = append(opts.Customizers, customizer)
	}
}

func (k *K3s) Config() *rest.Config {
	if k == nil || k.cfg == nil {
		return nil
	}

	return rest.CopyConfig(k.cfg)
}

func (k *K3s) Scheme() *runtime.Scheme {
	if k == nil {
		return nil
	}

	return k.scheme
}

func (k *K3s) Client() client.Client {
	if k == nil {
		return nil
	}

	return k.cli
}

func (k *K3s) Stop(ctx context.Context) error {
	if k == nil || k.container == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}

	return k.container.Terminate(ctx)
}

func defaultK3sOptions() K3sOptions {
	return K3sOptions{
		Image: DefaultK3sImage,
	}
}
