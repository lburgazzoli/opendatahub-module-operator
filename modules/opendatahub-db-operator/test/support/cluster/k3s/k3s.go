package k3s

import (
	"context"
	"errors"
	"fmt"

	"github.com/testcontainers/testcontainers-go"
	tck3s "github.com/testcontainers/testcontainers-go/modules/k3s"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"

	modulemanager "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/pkg/manager"
	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/test/support"
	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/test/support/cluster/common"
)

const defaultImage = "rancher/k3s:v1.36.2-k3s1"

type Option func(*options)

type options struct {
	Image       string
	Customizers []testcontainers.ContainerCustomizer
}

type Cluster struct {
	base      common.Base
	container *tck3s.K3sContainer
}

func New(
	ctx context.Context,
	testCfg *support.Config,
	opts ...Option,
) (common.Instance, error) {
	if ctx == nil {
		return nil, fmt.Errorf("context is nil")
	}

	options := defaultOptions()
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

	scheme, err := modulemanager.NewScheme()
	if err != nil {
		_ = testcontainers.TerminateContainer(container)
		return nil, fmt.Errorf("building scheme: %w", err)
	}
	cli, err := common.NewClient(cfg, scheme)
	if err != nil {
		_ = testcontainers.TerminateContainer(container)
		return nil, err
	}

	return &Cluster{
		base:      common.NewBase(cfg, cli, testCfg),
		container: container,
	}, nil
}

func WithImage(image string) Option {
	return func(opts *options) {
		if opts == nil || image == "" {
			return
		}

		opts.Image = image
	}
}

func WithContainerCustomizer(customizer testcontainers.ContainerCustomizer) Option {
	return func(opts *options) {
		if opts == nil || customizer == nil {
			return
		}

		opts.Customizers = append(opts.Customizers, customizer)
	}
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
	var errs []error

	if c == nil {
		return nil
	}

	if err := c.base.TearDown(ctx); err != nil {
		errs = append(errs, err)
	}
	if c.container == nil {
		return errors.Join(errs...)
	}
	if ctx == nil {
		ctx = context.Background()
	}

	if err := c.container.Terminate(ctx); err != nil {
		errs = append(errs, err)
	}

	return errors.Join(errs...)
}

func defaultOptions() options {
	return options{
		Image: defaultImage,
	}
}
