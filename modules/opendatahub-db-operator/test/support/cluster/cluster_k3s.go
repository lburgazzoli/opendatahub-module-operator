package cluster

import (
	"context"
	"fmt"

	"github.com/testcontainers/testcontainers-go"
	tck3s "github.com/testcontainers/testcontainers-go/modules/k3s"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const defaultK3sImage = "rancher/k3s:v1.36.2-k3s1"

type k3sOption func(*k3sOptions)

type k3sOptions struct {
	Image       string
	Customizers []testcontainers.ContainerCustomizer
}

type k3sInstance struct {
	cfg       *rest.Config
	cli       client.Client
	container *tck3s.K3sContainer
}

func newK3s(ctx context.Context, opts ...k3sOption) (Instance, error) {
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

	scheme, err := newScheme()
	if err != nil {
		_ = testcontainers.TerminateContainer(container)
		return nil, err
	}
	cli, err := newClient(cfg, scheme)
	if err != nil {
		_ = testcontainers.TerminateContainer(container)
		return nil, err
	}

	return &k3sInstance{
		cfg:       cfg,
		cli:       cli,
		container: container,
	}, nil
}

func withK3sImage(image string) k3sOption {
	return func(opts *k3sOptions) {
		if opts == nil || image == "" {
			return
		}

		opts.Image = image
	}
}

func withContainerCustomizer(customizer testcontainers.ContainerCustomizer) k3sOption {
	return func(opts *k3sOptions) {
		if opts == nil || customizer == nil {
			return
		}

		opts.Customizers = append(opts.Customizers, customizer)
	}
}

func (k *k3sInstance) Config() *rest.Config {
	if k == nil || k.cfg == nil {
		return nil
	}

	return rest.CopyConfig(k.cfg)
}

func (k *k3sInstance) Client() client.Client {
	if k == nil {
		return nil
	}

	return k.cli
}

func (k *k3sInstance) Stop(ctx context.Context) error {
	if k == nil || k.container == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}

	return k.container.Terminate(ctx)
}

func defaultK3sOptions() k3sOptions {
	return k3sOptions{
		Image: defaultK3sImage,
	}
}
