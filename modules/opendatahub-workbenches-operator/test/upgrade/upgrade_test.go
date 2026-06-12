//go:build upgrade

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

package upgrade

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	. "github.com/onsi/gomega"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	k8sm "github.com/lburgazzoli/gomega-matchers/pkg/matchers/k8s"

	componentsv1alpha1 "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-workbenches-operator/api/components/v1alpha1"
	moduleconfig "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-workbenches-operator/pkg/config"
	workbenchesmanager "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-workbenches-operator/pkg/manager"
	module "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-workbenches-operator/pkg/module"
	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-workbenches-operator/test/support"
)

const (
	timeout  = 90 * time.Second
	interval = 2 * time.Second

	appliedUpgradeVersion = "0.1.0"
	desiredUpgradeVersion = "0.2.0"

	moduleCRDName = "workbenches.components.platform.opendatahub.io"
)

var (
	ctx             context.Context
	cancel          context.CancelFunc
	kubeConfig      *rest.Config
	moduleCfg       *moduleconfig.Config
	directClient    client.Client
	k8sClient       client.Client
	k               *k8sm.Matcher
	directK         *k8sm.Matcher
	operatorCfgData map[string]string
	testScheme      = workbenchesmanager.NewScheme()
)

func TestMain(m *testing.M) {
	os.Exit(runTestMain(m))
}

func runTestMain(m *testing.M) int {
	ctx, cancel = context.WithCancel(context.Background())
	defer cancel()
	var err error

	ctrl.SetLogger(zap.New(zap.UseDevMode(true)))

	kubeConfig, err = config.GetConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to get kubeconfig: %v\n", err)
		return 1
	}

	directClient, err = client.New(kubeConfig, client.Options{Scheme: testScheme})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create client: %v\n", err)
		return 1
	}

	operatorNamespace := support.OperatorNamespace()
	if err := support.EnsureNamespace(ctx, directClient, operatorNamespace); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create namespace: %v\n", err)
		return 1
	}

	moduleCRD := &apiextensionsv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: moduleCRDName},
	}
	if err := directClient.Get(ctx, client.ObjectKeyFromObject(moduleCRD), moduleCRD); err != nil {
		fmt.Fprintf(os.Stderr, "Expected CRD %s to be installed before running upgrade tests: %v\n", moduleCRDName, err)
		return 1
	}

	_ = directClient.DeleteAllOf(ctx, &componentsv1alpha1.Workbenches{})
	_ = directClient.DeleteAllOf(ctx, &appsv1.Deployment{}, client.InNamespace(operatorNamespace))
	_ = directClient.DeleteAllOf(ctx, &corev1.Service{}, client.InNamespace(operatorNamespace))

	operatorCfgData = support.MustReadConfigMapData(
		support.MustProjectFile("config", "manager", "configmap.yaml"))

	module.Version = desiredUpgradeVersion

	moduleCfg = &moduleconfig.Config{
		PlatformName:          operatorCfgData[moduleconfig.KeyPlatformName],
		PlatformVersion:       desiredUpgradeVersion,
		ApplicationsNamespace: operatorNamespace,
		ManifestsPath:         support.MustProjectFile("config", "manifests"),
		Controller: moduleconfig.ControllerConfig{
			Metrics:        moduleconfig.MetricsConfig{BindAddress: "0"},
			Health:         moduleconfig.HealthConfig{BindAddress: "0"},
			LeaderElection: moduleconfig.LeaderElectionConfig{Enabled: false},
			Webhook:        moduleconfig.WebhookConfig{Enabled: false},
		},
	}

	directK = k8sm.New(directClient, testScheme)

	return m.Run()
}

func startManager(t *testing.T) {
	t.Helper()
	if k8sClient != nil {
		return
	}

	mgr, err := workbenchesmanager.New(
		ctx,
		kubeConfig,
		moduleCfg,
		func(opts *ctrl.Options) {
			opts.Cache.DefaultTransform = nil
			opts.Cache.ReaderFailOnMissingInformer = false
		},
	)
	NewWithT(t).Expect(err).NotTo(HaveOccurred())

	go func() {
		if err := mgr.Start(ctx); err != nil {
			fmt.Fprintf(os.Stderr, "Manager exited with error: %v\n", err)
		}
	}()

	if !mgr.GetCache().WaitForCacheSync(ctx) {
		t.Fatal("failed to sync manager cache")
	}

	k8sClient = mgr.GetClient()
	k = k8sm.New(k8sClient, testScheme)
}

type workbenchesUpgradeTest struct {
	module            *componentsv1alpha1.Workbenches
	moduleCRD         *apiextensionsv1.CustomResourceDefinition
	operatorNamespace string
}

func TestWorkbenchesUpgrade(t *testing.T) {
	suite := &workbenchesUpgradeTest{
		module: &componentsv1alpha1.Workbenches{
			ObjectMeta: metav1.ObjectMeta{
				Name: componentsv1alpha1.WorkbenchesInstanceName,
			},
			Spec: componentsv1alpha1.WorkbenchesSpec{
				WorkbenchesCommonSpec: componentsv1alpha1.WorkbenchesCommonSpec{
					WorkbenchNamespace: "opendatahub",
				},
			},
		},
		moduleCRD: &apiextensionsv1.CustomResourceDefinition{
			ObjectMeta: metav1.ObjectMeta{Name: moduleCRDName},
		},
		operatorNamespace: support.OperatorNamespace(),
	}
	migration := newMigrationTests(suite)

	t.Run("migration", migration.Execute)
}

func waitForDeleted(t *testing.T, obj client.Object) {
	t.Helper()

	g := NewWithT(t)
	g.Eventually(directK.Gone(obj)).WithContext(ctx).WithTimeout(timeout).WithPolling(interval).Should(BeTrue())
}

func waitForSingletonDeleted(t *testing.T, obj client.Object) {
	t.Helper()

	waitForDeleted(t, obj)
	obj.SetResourceVersion("")
	obj.SetUID("")
}
