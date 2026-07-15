package kind

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/rs/xid"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"
	kindcluster "sigs.k8s.io/kind/pkg/cluster"

	modulemanager "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/pkg/manager"
	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/test/support"
	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/test/support/cluster/common"
)

const (
	clusterPrefix       = "odh-db-operator-"
	defaultLogsDir      = "odh-db-operator-kind-logs-"
	defaultWaitForReady = 3 * time.Minute
)

type Cluster struct {
	opts     options
	base     common.Base
	provider *kindcluster.Provider
	name     string
}

func New(
	ctx context.Context,
	cfg *support.Config,
	opts ...Option,
) (common.Instance, error) {
	tc := &Cluster{}
	tc.opts = options{
		NamePrefix: clusterPrefix,
		Purge:      new(true),
		CreateOptions: []kindcluster.CreateOption{
			kindcluster.CreateWithWaitForReady(defaultWaitForReady),
		},
		LogFn: func(format string, args ...any) {
			_, _ = os.Stderr.WriteString(fmt.Sprintf(format+"\n", args...))
		},
	}

	for _, opt := range opts {
		opt.applyOption(&tc.opts)
	}

	if err := tc.opts.Validate(); err != nil {
		return nil, err
	}

	if tc.opts.LogsDir == "" {
		logsDir, err := os.MkdirTemp("", defaultLogsDir)
		if err != nil {
			return nil, fmt.Errorf("creating temp log directory: %w", err)
		}

		tc.opts.LogsDir = logsDir
	}

	tc.provider = kindcluster.NewProvider(tc.opts.ProviderOptions...)
	tc.name = fmt.Sprintf("%s%s", tc.opts.NamePrefix, xid.New().String())

	if err := tc.setUpProvider(cfg); err != nil {
		return nil, errors.Join(
			err,
			tc.collectFailureLogs(),
			tc.tearDownProvider(tc.opts.LogFn),
		)
	}

	return tc, nil
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
	if err := c.tearDownProvider(nil); err != nil {
		errs = append(errs, err)
	}

	return errors.Join(errs...)
}

func (c *Cluster) setUpProvider(testCfg *support.Config) error {
	if c == nil {
		return fmt.Errorf("cluster is nil")
	}
	if c.provider == nil {
		return fmt.Errorf("provider is nil")
	}

	if c.opts.Purge != nil && *c.opts.Purge {
		purgeStaleClusters(c.provider, c.opts.NamePrefix, c.opts.LogFn)
	}

	if err := c.provider.Create(c.name, c.opts.CreateOptions...); err != nil {
		return fmt.Errorf("creating kind cluster: %w", err)
	}

	kubeCfg, err := c.provider.KubeConfig(c.name, false)
	if err != nil {
		return fmt.Errorf("reading kind kubeconfig: %w", err)
	}

	restCfg, err := clientcmd.RESTConfigFromKubeConfig([]byte(kubeCfg))
	if err != nil {
		return fmt.Errorf("creating rest config from kubeconfig: %w", err)
	}

	scheme, err := modulemanager.NewScheme()
	if err != nil {
		return fmt.Errorf("building scheme: %w", err)
	}

	cli, err := common.NewClient(restCfg, scheme)
	if err != nil {
		return err
	}

	c.base = common.NewBase(restCfg, cli, testCfg)

	return nil
}

func (c *Cluster) tearDownProvider(logFn func(format string, args ...any)) error {
	if c == nil || c.provider == nil || c.name == "" {
		return nil
	}

	if err := c.provider.Delete(c.name, ""); err != nil {
		wrappedErr := fmt.Errorf("deleting kind cluster %q: %w", c.name, err)
		if logFn != nil {
			logFn("%v", wrappedErr)
		}

		return wrappedErr
	}

	return nil
}

func (c *Cluster) collectFailureLogs() error {
	if c == nil || c.provider == nil || c.name == "" || c.opts.LogsDir == "" {
		return nil
	}

	logDir := filepath.Join(c.opts.LogsDir, c.name)
	if err := c.provider.CollectLogs(c.name, logDir); err != nil {
		return fmt.Errorf("failed to collect kind cluster logs in %s: %v", logDir, err)
	}

	c.opts.LogFn("kind cluster logs written to %s", logDir)

	return nil
}
