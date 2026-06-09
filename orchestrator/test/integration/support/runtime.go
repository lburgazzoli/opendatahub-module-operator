//go:build integration

/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package support

import (
	"context"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/onsi/gomega"

	k8sm "github.com/lburgazzoli/gomega-matchers/pkg/matchers/k8s"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	configApi "github.com/lburgazzoli/opendatahub-module-operator/orchestrator/api/config/v1alpha1"
	"github.com/lburgazzoli/opendatahub-module-operator/orchestrator/internal/controller/platform"
	"github.com/lburgazzoli/opendatahub-module-operator/orchestrator/internal/controller/platformoperator"
	"github.com/lburgazzoli/opendatahub-module-operator/orchestrator/pkg/config"
	"github.com/lburgazzoli/opendatahub-module-operator/orchestrator/pkg/module"
	testsupport "github.com/lburgazzoli/opendatahub-module-operator/orchestrator/test/support"
)

const (
	TestNamespace = "test"
	Timeout       = 30 * time.Second
	Interval      = 250 * time.Millisecond
)

var (
	kubeConfig *rest.Config
	testConfig *config.Config
	baseConfig config.Config

	testScheme = runtime.NewScheme()

	cleanupModules  []*module.Module
	selectedModules []*module.Module
)

type Suite struct {
	Modules        []*module.Module
	CleanupModules []*module.Module
	Config         *config.Config
	Client         client.Client
	K              *k8sm.Resources
}

type RunConfig struct {
	Modules        []*module.Module
	CleanupModules []*module.Module
}

type MainSuite struct {
	cancel context.CancelFunc
}

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(testScheme))
	utilruntime.Must(apiextensionsv1.AddToScheme(testScheme))
	utilruntime.Must(configApi.AddToScheme(testScheme))
}

func Setup(cfg RunConfig) (*MainSuite, error) {
	ctx, cancel := context.WithCancel(context.Background())

	ctrl.SetLogger(zap.New(zap.UseDevMode(true)))

	var err error
	var chartPath string
	kubeConfig, chartPath, err = setupEnv()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("setting up environment: %w", err)
	}

	selectedModules, err = normalizeModules(cfg.Modules, chartPath)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("normalizing test modules: %w", err)
	}

	if len(cfg.CleanupModules) == 0 {
		cleanupModules, err = normalizeModules(cfg.Modules, chartPath)
	} else {
		cleanupModules, err = normalizeModules(cfg.CleanupModules, chartPath)
	}
	if err != nil {
		cancel()
		return nil, fmt.Errorf("normalizing cleanup modules: %w", err)
	}

	if err := loadTestConfig(); err != nil {
		cancel()
		return nil, fmt.Errorf("loading test config: %w", err)
	}

	suite, err := newSuite()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("creating cleanup client: %w", err)
	}

	if err := suite.cleanupBeforeRun(ctx); err != nil {
		cancel()
		return nil, fmt.Errorf("cleaning cluster state before run: %w", err)
	}

	if err := setupManager(ctx, kubeConfig, selectedModules); err != nil {
		cancel()
		return nil, fmt.Errorf("setting up manager: %w", err)
	}

	gomega.SetDefaultEventuallyTimeout(Timeout)
	gomega.SetDefaultEventuallyPollingInterval(Interval)

	return &MainSuite{cancel: cancel}, nil
}

func (suite *MainSuite) Run(m *testing.M) int {
	return m.Run()
}

func (suite *MainSuite) TearDown() error {
	defer suite.cancel()

	cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), cleanupWaitTimeout)
	defer cleanupCancel()

	cleanupSuite, err := newSuite()
	if err != nil {
		return fmt.Errorf("creating teardown cleanup client: %w", err)
	}

	if err := cleanupSuite.cleanupBeforeRun(cleanupCtx); err != nil {
		return fmt.Errorf("cleaning cluster state during teardown: %w", err)
	}

	return nil
}

func Run(m *testing.M, cfg RunConfig) int {
	suite, err := Setup(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to setup integration suite: %v\n", err)
		return 1
	}

	code := suite.Run(m)

	if err := suite.TearDown(); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to teardown integration suite: %v\n", err)
		if code == 0 {
			code = 1
		}
	}

	return code
}

func NewSuite(t *testing.T) *Suite {
	t.Helper()

	suite, err := newSuite()
	if err != nil {
		t.Fatalf("creating test suite: %v", err)
	}

	return suite
}

func newSuite() (*Suite, error) {
	cli, err := newClient(kubeConfig)
	if err != nil {
		return nil, err
	}

	return &Suite{
		Modules:        selectedModules,
		CleanupModules: cleanupModules,
		Config:         testConfig,
		Client:         cli,
		K:              k8sm.NewResources(cli, testScheme),
	}, nil
}

func newClient(kc *rest.Config) (client.Client, error) {
	httpClient, err := rest.HTTPClientFor(kc)
	if err != nil {
		return nil, fmt.Errorf("creating HTTP client: %w", err)
	}

	mapper, err := apiutil.NewDynamicRESTMapper(kc, httpClient)
	if err != nil {
		return nil, fmt.Errorf("creating REST mapper: %w", err)
	}

	cli, err := client.New(kc, client.Options{Scheme: testScheme, Mapper: mapper})
	if err != nil {
		return nil, fmt.Errorf("creating client: %w", err)
	}

	return cli, nil
}

func setupEnv() (*rest.Config, string, error) {
	root, err := testsupport.ProjectRoot()
	if err != nil {
		return nil, "", fmt.Errorf("finding project root: %w", err)
	}

	chartsPath := filepath.Join(root, "orchestrator", "test", "support", "testdata", "charts")
	cfg, err := ctrl.GetConfig()
	if err != nil {
		return nil, "", fmt.Errorf("getting existing cluster config: %w", err)
	}

	chartPath := filepath.Join(chartsPath, "test-module")

	return cfg, chartPath, nil
}

func setupManager(ctx context.Context, kc *rest.Config, modules []*module.Module) error {
	registry := module.NewModuleRegistry(testConfig.Namespace(), testConfig.ChartsPath)

	for _, mod := range modules {
		registry.Register(mod)
	}

	registry.ComputeRunlevels()

	ctrlMgr, err := ctrl.NewManager(kc, ctrl.Options{
		Scheme:         testScheme,
		Metrics:        metricsserver.Options{BindAddress: "0"},
		LeaderElection: false,
	})
	if err != nil {
		return fmt.Errorf("creating manager: %w", err)
	}

	if err := platform.NewReconciler(ctx, ctrlMgr, registry, testConfig); err != nil {
		return fmt.Errorf("creating platform reconciler: %w", err)
	}

	if err := platformoperator.NewModuleReconciler(ctx, ctrlMgr, registry, testConfig); err != nil {
		return fmt.Errorf("creating module reconciler: %w", err)
	}

	go func() {
		if err := ctrlMgr.Start(ctx); err != nil {
			fmt.Fprintf(os.Stderr, "Manager exited with error: %v\n", err)
		}
	}()

	if !ctrlMgr.GetCache().WaitForCacheSync(ctx) {
		return fmt.Errorf("waiting for manager cache sync")
	}

	return nil
}

func loadTestConfig() error {
	cfg, err := config.LoadFromFS(nil)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	cfg.Distribution.Name = "Standalone"

	testConfig = cfg
	baseConfig = *cfg

	return nil
}

func normalizeModules(modules []*module.Module, chartPath string) ([]*module.Module, error) {
	normalized := make([]*module.Module, 0, len(modules))
	for i, mod := range modules {
		if mod == nil {
			return nil, fmt.Errorf("test module at index %d is nil", i)
		}
		normalized = append(normalized, normalizeModule(mod, chartPath))
	}

	return normalized, nil
}

func normalizeModule(mod *module.Module, chartPath string) *module.Module {
	cloned := &module.Module{
		Name:              mod.Name,
		GVK:               mod.GVK,
		Namespace:         mod.Namespace,
		Runlevel:          mod.Runlevel,
		ChartPath:         mod.ChartPath,
		Timeout:           mod.Timeout,
		ConfigHashRollout: mod.ConfigHashRollout,
		Config:            mod.Config,
		Ext:               mod.Ext,
	}
	if len(mod.AdminAcks) > 0 {
		cloned.AdminAcks = append([]module.AdminAck(nil), mod.AdminAcks...)
	}
	if len(mod.Values) > 0 {
		cloned.Values = make(map[string]any, len(mod.Values))
		maps.Copy(cloned.Values, mod.Values)
	}
	if cloned.ChartPath == "" {
		cloned.ChartPath = chartPath
	}

	return cloned
}
