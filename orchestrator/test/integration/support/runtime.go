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
	ctx        context.Context
	cancel     context.CancelFunc
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

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(testScheme))
	utilruntime.Must(apiextensionsv1.AddToScheme(testScheme))
	utilruntime.Must(configApi.AddToScheme(testScheme))
}

func Run(m *testing.M, cfg RunConfig) int {
	ctx, cancel = context.WithCancel(context.Background())
	defer cancel()

	ctrl.SetLogger(zap.New(zap.UseDevMode(true)))

	var err error
	var chartPath string
	kubeConfig, chartPath, err = setupEnv()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to setup environment: %v\n", err)
		return 1
	}

	selectedModules, err = normalizeModules(cfg.Modules, chartPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to normalize test modules: %v\n", err)
		return 1
	}

	if len(cfg.CleanupModules) == 0 {
		cleanupModules, err = normalizeModules(cfg.Modules, chartPath)
	} else {
		cleanupModules, err = normalizeModules(cfg.CleanupModules, chartPath)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to normalize cleanup modules: %v\n", err)
		return 1
	}

	if err := setupManager(kubeConfig, selectedModules); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to setup manager: %v\n", err)
		return 1
	}

	gomega.SetDefaultEventuallyTimeout(Timeout)
	gomega.SetDefaultEventuallyPollingInterval(Interval)

	return m.Run()
}

func NewSuite(t *testing.T) *Suite {
	t.Helper()

	httpClient, err := rest.HTTPClientFor(kubeConfig)
	if err != nil {
		t.Fatalf("creating HTTP client: %v", err)
	}

	mapper, err := apiutil.NewDynamicRESTMapper(kubeConfig, httpClient)
	if err != nil {
		t.Fatalf("creating REST mapper: %v", err)
	}

	cli, err := client.New(kubeConfig, client.Options{Scheme: testScheme, Mapper: mapper})
	if err != nil {
		t.Fatalf("creating test client: %v", err)
	}

	return &Suite{
		Modules:        selectedModules,
		CleanupModules: cleanupModules,
		Config:         testConfig,
		Client:         cli,
		K:              k8sm.NewResources(cli, testScheme),
	}
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

func setupManager(kc *rest.Config, modules []*module.Module) error {
	var err error

	testConfig, err = config.LoadFromFS(nil)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}
	baseConfig = *testConfig

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
