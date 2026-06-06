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

package integration

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	. "github.com/onsi/gomega"

	k8sm "github.com/lburgazzoli/gomega-matchers/pkg/matchers/k8s"
	k3senv "github.com/lburgazzoli/k3s-envtest/pkg/k3senv"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
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
	"github.com/lburgazzoli/opendatahub-module-operator/orchestrator/test/support"
)

const (
	testNS   = "test"
	timeout  = 30 * time.Second
	interval = 250 * time.Millisecond
)

var (
	componentsGV = schema.GroupVersion{
		Group:   "test.opendatahub.io",
		Version: "v1alpha1",
	}

	alphaGVK = componentsGV.WithKind("Alpha")
	betaGVK  = componentsGV.WithKind("Beta")
	gammaGVK = componentsGV.WithKind("Gamma")

	ctx            context.Context
	cancel         context.CancelFunc
	kubeConfig     *rest.Config
	testOrchestrator *platform.Orchestrator

	testScheme = runtime.NewScheme()

	testModules []*module.Module
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(testScheme))
	utilruntime.Must(apiextensionsv1.AddToScheme(testScheme))
	utilruntime.Must(configApi.AddToScheme(testScheme))
}

func TestMain(m *testing.M) {
	ctx, cancel = context.WithCancel(context.Background())
	defer cancel()

	ctrl.SetLogger(zap.New(zap.UseDevMode(true)))

	var (
		cleanup func()
		err     error
	)
	kubeConfig, cleanup, err = setupEnv()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to setup environment: %v\n", err)
		os.Exit(1)
	}
	defer cleanup()

	if err = setupManager(kubeConfig); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to setup manager: %v\n", err)
		os.Exit(1)
	}

	SetDefaultEventuallyTimeout(timeout)
	SetDefaultEventuallyPollingInterval(interval)

	os.Exit(
		m.Run(),
	)
}

func setupEnv() (*rest.Config, func(), error) {
	root, err := support.ProjectRoot()
	if err != nil {
		return nil, nil, fmt.Errorf("finding project root: %w", err)
	}

	chartsPath := filepath.Join(root, "orchestrator", "test", "integration", "testdata", "charts")
	crdPath := filepath.Join(root, "orchestrator", "config", "crd", "bases")

	env, err := k3senv.New(
		k3senv.WithScheme(testScheme),
		k3senv.WithManifests(crdPath),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("creating k3s environment: %w", err)
	}

	if err := env.Start(ctx); err != nil {
		return nil, nil, fmt.Errorf("starting k3s environment: %w", err)
	}

	chartPath := filepath.Join(chartsPath, "test-module")

	testModules = []*module.Module{
		{
			GVK:       alphaGVK,
			Namespace: testNS + "-" + strings.ToLower(alphaGVK.Kind),
			ChartPath: chartPath,
			Runlevel:  1,
			Config: func(_ context.Context, _ client.Client) (map[string]any, error) {
				return map[string]any{
					"module-name":   "alpha",
					"platform-name": "TestPlatform",
					"test-key":      "test-value",
				}, nil
			},
		},
		{
			GVK:       betaGVK,
			Namespace: testNS + "-" + strings.ToLower(betaGVK.Kind),
			ChartPath: chartPath,
			Runlevel:  2,
			Config: func(_ context.Context, _ client.Client) (map[string]any, error) {
				return map[string]any{
					"module-name": "beta",
				}, nil
			},
		},
		{
			GVK:       gammaGVK,
			Namespace: testNS + "-" + strings.ToLower(gammaGVK.Kind),
			ChartPath: chartPath,
			Runlevel:  2,
			Config: func(_ context.Context, _ client.Client) (map[string]any, error) {
				return map[string]any{
					"module-name": "gamma",
				}, nil
			},
		},
	}

	return env.Config(), func() { _ = env.Stop(ctx) }, nil
}

func setupManager(kubeConfig *rest.Config) error {
	cfg, err := config.LoadFromFS(nil)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	testOrchestrator = platform.NewOrchestrator(cfg)

	for _, mod := range testModules {
		testOrchestrator.Register(mod)
	}

	testOrchestrator.ComputeRunlevels()

	ctrlMgr, err := ctrl.NewManager(kubeConfig, ctrl.Options{
		Scheme:         testScheme,
		Metrics:        metricsserver.Options{BindAddress: "0"},
		LeaderElection: false,
	})
	if err != nil {
		return fmt.Errorf("creating manager: %w", err)
	}

	if err := platform.NewReconciler(ctx, ctrlMgr, testOrchestrator); err != nil {
		return fmt.Errorf("creating platform reconciler: %w", err)
	}

	if err := platformoperator.NewModuleReconciler(ctx, ctrlMgr, testOrchestrator, cfg); err != nil {
		return fmt.Errorf("creating module reconciler: %w", err)
	}

	go func() {
		if err := ctrlMgr.Start(ctx); err != nil {
			fmt.Fprintf(os.Stderr, "Manager exited with error: %v\n", err)
		}
	}()

	return nil
}

// orchestratorTest holds shared test fixtures.
type orchestratorTest struct {
	modules      []*module.Module
	orchestrator *platform.Orchestrator
	client       client.Client
	k            *k8sm.Resources
}

func TestPlatformOperator(t *testing.T) {
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

	suite := &orchestratorTest{
		modules:      testModules,
		orchestrator: testOrchestrator,
		client:       cli,
		k:            k8sm.NewResources(cli, testScheme),
	}

	foundation := &foundationTests{suite: suite}
	t.Run("foundation", foundation.Execute)

	runlevels := &runlevelTests{suite: suite}
	t.Run("runlevel", runlevels.Execute)
}
