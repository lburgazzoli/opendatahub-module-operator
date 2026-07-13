package helm

import (
	"context"
	"fmt"
	"time"

	"helm.sh/helm/v4/pkg/action"
	"helm.sh/helm/v4/pkg/helmpath"
	"helm.sh/helm/v4/pkg/kube"
	"k8s.io/client-go/rest"
)

const defaultTimeout = 5 * time.Minute

type Client struct {
	restConfig *rest.Config
	opts       ClientOptions
}

func New(restConfig *rest.Config, opts ...ClientOption) (*Client, error) {
	if restConfig == nil {
		return nil, fmt.Errorf("rest config is nil")
	}

	options := ClientOptions{
		RepositoryCache: helmpath.CachePath("repository"),
		Timeout:         defaultTimeout,
		HelmDriver:      "secret",
	}
	for _, opt := range opts {
		if opt != nil {
			opt.ApplyTo(&options)
		}
	}

	return &Client{
		restConfig: rest.CopyConfig(restConfig),
		opts:       options,
	}, nil
}

func (c *Client) Install(ctx context.Context, opts ...InstallOption) error {
	options := InstallOptions{}
	for _, opt := range opts {
		if opt != nil {
			opt.ApplyTo(&options)
		}
	}

	if options.Chart == "" {
		return fmt.Errorf("chart is empty")
	}
	if options.ReleaseName == "" {
		return fmt.Errorf("release name is empty")
	}
	if options.Namespace == "" {
		return fmt.Errorf("namespace is empty")
	}
	values, err := options.GetValues()
	if err != nil {
		return err
	}

	cfg, err := c.newActionConfig(options.Namespace)
	if err != nil {
		return err
	}

	chrt, err := loadChart(ctx, options, c.opts.RepositoryCache)
	if err != nil {
		return err
	}

	install := action.NewInstall(cfg)
	install.ReleaseName = options.ReleaseName
	install.Namespace = options.Namespace
	install.CreateNamespace = true
	install.SkipCRDs = options.SkipCRDs
	install.Timeout = c.opts.Timeout
	install.WaitStrategy = kube.StatusWatcherStrategy

	_, err = install.RunWithContext(ctx, chrt, values)
	if err != nil {
		return fmt.Errorf("installing helm release %q: %w", options.ReleaseName, err)
	}

	return nil
}

func (c *Client) Uninstall(opts ...UninstallOption) error {
	options := UninstallOptions{}
	for _, opt := range opts {
		if opt != nil {
			opt.ApplyTo(&options)
		}
	}
	if options.ReleaseName == "" {
		return fmt.Errorf("release name is empty")
	}
	if options.Namespace == "" {
		return fmt.Errorf("namespace is empty")
	}

	cfg, err := c.newActionConfig(options.Namespace)
	if err != nil {
		return err
	}

	uninstall := action.NewUninstall(cfg)
	uninstall.IgnoreNotFound = true
	uninstall.Timeout = c.opts.Timeout
	uninstall.WaitStrategy = kube.StatusWatcherStrategy

	if _, err := uninstall.Run(options.ReleaseName); err != nil {
		return fmt.Errorf("uninstalling helm release %q: %w", options.ReleaseName, err)
	}

	return nil
}

func (c *Client) newActionConfig(namespace string) (*action.Configuration, error) {
	cfg := action.NewConfiguration()
	getter := &restClientGetter{
		restConfig: rest.CopyConfig(c.restConfig),
	}
	if err := cfg.Init(getter, namespace, c.opts.HelmDriver); err != nil {
		return nil, fmt.Errorf("initializing helm action config: %w", err)
	}

	return cfg, nil
}
