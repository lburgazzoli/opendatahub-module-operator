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
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
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
	timeout  = 90 * time.Second
	interval = 2 * time.Second
)

var (
	componentsGV = schema.GroupVersion{
		Group:   "test.opendatahub.io",
		Version: "v1alpha1",
	}

	alphaGVK = componentsGV.WithKind("Alpha")
	betaGVK  = componentsGV.WithKind("Beta")
	gammaGVK = componentsGV.WithKind("Gamma")

	ctx       context.Context
	cancel    context.CancelFunc
	k8sClient client.Client
	k         *k8sm.Matcher

	testScheme = runtime.NewScheme()

	testModules []*module.Module
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(testScheme))
	utilruntime.Must(apiextensionsv1.AddToScheme(testScheme))
	utilruntime.Must(configApi.AddToScheme(testScheme))
}

func TestMain(m *testing.M) {
	os.Exit(runTestMain(m))
}

func runTestMain(m *testing.M) int {
	ctx, cancel = context.WithCancel(context.Background())
	defer cancel()

	ctrl.SetLogger(zap.New(zap.UseDevMode(true)))

	root, err := support.ProjectRoot()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to find project root: %v\n", err)
		return 1
	}

	chartsPath := filepath.Join(root, "orchestrator", "test", "integration", "testdata", "charts")
	crdPath := filepath.Join(root, "orchestrator", "config", "crd", "bases")

	env, err := k3senv.New(
		k3senv.WithScheme(testScheme),
		k3senv.WithManifests(crdPath),
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create k3s environment: %v\n", err)
		return 1
	}

	if err := env.Start(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to start k3s environment: %v\n", err)
		return 1
	}

	defer func() {
		_ = env.Stop(ctx)
	}()

	cfg, err := config.LoadFromFS(nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
		return 1
	}

	o := platform.NewOrchestrator(cfg)

	chartPath := filepath.Join(chartsPath, "test-module")

	testModules = []*module.Module{
		{
			GVK:       alphaGVK,
			Namespace: testNS + "-" + strings.ToLower(alphaGVK.Kind),
			ChartPath: chartPath,
			Runlevel:  1,
			Config: func(_ context.Context, _ client.Client) (map[string]any, error) {
				return map[string]any{
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
		},
		{
			GVK:       gammaGVK,
			Namespace: testNS + "-" + strings.ToLower(gammaGVK.Kind),
			ChartPath: chartPath,
			Runlevel:  2,
		},
	}

	for _, mod := range testModules {
		o.Register(mod)
	}

	o.ComputeRunlevels()
	o.SetState(configApi.OperationalState{Mode: configApi.ModeReconcile})

	ctrlMgr, err := ctrl.NewManager(env.Config(), ctrl.Options{
		Scheme:         testScheme,
		Metrics:        metricsserver.Options{BindAddress: "0"},
		LeaderElection: false,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create manager: %v\n", err)
		return 1
	}

	if err := platform.NewReconciler(ctx, ctrlMgr, o); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create platform reconciler: %v\n", err)
		return 1
	}

	if err := platformoperator.NewModuleReconciler(ctx, ctrlMgr, o, cfg); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create module reconciler: %v\n", err)
		return 1
	}

	go func() {
		if err := ctrlMgr.Start(ctx); err != nil {
			fmt.Fprintf(os.Stderr, "Manager exited with error: %v\n", err)
		}
	}()

	SetDefaultEventuallyTimeout(timeout)
	SetDefaultEventuallyPollingInterval(interval)

	k8sClient = ctrlMgr.GetClient()
	k = k8sm.New(k8sClient, testScheme)

	for _, mod := range testModules {
		ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: mod.Namespace}}
		if err := k8sClient.Create(ctx, ns); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to create namespace %s: %v\n", mod.Namespace, err)
			return 1
		}
	}

	return m.Run()
}

// orchestratorTest holds shared test fixtures.
type orchestratorTest struct {
	modules []*module.Module
	pos     []*configApi.PlatformOperator
}

func TestPlatformOperator(t *testing.T) {
	g := NewWithT(t)

	suite := &orchestratorTest{
		modules: testModules,
	}

	moduleNames := make([]string, 0, len(testModules))
	for _, mod := range testModules {
		moduleNames = append(moduleNames, mod.EffectiveName())
	}

	p := &configApi.Platform{
		ObjectMeta: metav1.ObjectMeta{Name: configApi.PlatformInstanceName},
		Spec: configApi.PlatformSpec{
			Modules: moduleNames,
		},
	}

	_ = k8sClient.Delete(ctx, p)
	waitForDeleted(t, p)
	p.ResourceVersion = ""

	g.Expect(k8sClient.Create(ctx, p)).To(Succeed())

	t.Cleanup(func() {
		_ = k8sClient.Delete(ctx, p)
	})

	for _, mod := range suite.modules {
		po := &configApi.PlatformOperator{
			ObjectMeta: metav1.ObjectMeta{Name: mod.EffectiveName()},
		}

		g.Eventually(func() error {
			return k8sClient.Get(ctx, client.ObjectKeyFromObject(po), po)
		}).WithContext(ctx).Should(Succeed())

		suite.pos = append(suite.pos, po)
	}

	foundation := &foundationTests{suite: suite}
	t.Run("foundation", foundation.Execute)
}

func waitForDeleted(t *testing.T, obj client.Object) {
	t.Helper()

	g := NewWithT(t)
	g.Eventually(func(g Gomega) {
		fresh := obj.DeepCopyObject().(client.Object)
		err := k8sClient.Get(ctx, client.ObjectKeyFromObject(obj), fresh)
		if err != nil {
			g.Expect(client.IgnoreNotFound(err)).To(Succeed())
		}
	}).WithContext(ctx).WithTimeout(timeout).WithPolling(interval).Should(Succeed())
}
