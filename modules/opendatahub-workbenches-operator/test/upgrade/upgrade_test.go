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

	componentsv1alpha1 "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-workbenches-operator/api/components/v1alpha1"
	moduleconfig "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-workbenches-operator/pkg/config"
	workbenchesmanager "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-workbenches-operator/pkg/manager"
	module "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-workbenches-operator/pkg/module"
	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-workbenches-operator/test/support"
)

const (
	appliedUpgradeVersion = "0.1.0"
	desiredUpgradeVersion = "0.2.0"
)

type suite struct {
	directClient client.Client
	k8sClient    client.Client
}

var (
	testDirectClient client.Client
	testK8sClient    client.Client
	testKubeConfig   *rest.Config
	testModuleCfg    *moduleconfig.Config
	managerCtx       context.Context
	managerCancel    context.CancelFunc
)

func newSuite() suite {
	return suite{
		directClient: testDirectClient,
		k8sClient:    testK8sClient,
	}
}

func (s *suite) operatorNamespace() string {
	return support.OperatorNamespace()
}

func loadOperatorConfig(namespace string) (*moduleconfig.Config, error) {
	moduleCfg, err := moduleconfig.LoadFromFS(nil)
	if err != nil {
		return nil, fmt.Errorf("loading operator config: %w", err)
	}

	moduleCfg.PlatformVersion = desiredUpgradeVersion
	moduleCfg.ApplicationsNamespace = namespace
	moduleCfg.ManifestsPath = support.MustProjectFile("config", "manifests")
	moduleCfg.Controller.Metrics.BindAddress = "0"
	moduleCfg.Controller.Health.BindAddress = "0"
	moduleCfg.Controller.LeaderElection.Enabled = false
	moduleCfg.Controller.Pprof.BindAddress = "0"
	moduleCfg.Controller.Webhook.Enabled = false

	return moduleCfg, nil
}

func TestMain(m *testing.M) {
	os.Exit(runTestMain(m))
}

func runTestMain(m *testing.M) int {
	managerCtx, managerCancel = context.WithCancel(context.Background())
	defer managerCancel()

	gomegaCfg, err := support.LoadGomegaConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load test config: %v\n", err)
		return 1
	}

	SetDefaultEventuallyTimeout(gomegaCfg.EventuallyTimeout)
	SetDefaultEventuallyPollingInterval(gomegaCfg.EventuallyPollingInterval)
	SetDefaultConsistentlyPollingInterval(gomegaCfg.ConsistentlyPollingInterval)

	ctrl.SetLogger(zap.New(zap.UseDevMode(true)))

	testKubeConfig, err = config.GetConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to get kubeconfig: %v\n", err)
		return 1
	}

	testDirectClient, err = support.NewClient()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create client: %v\n", err)
		return 1
	}

	operatorNamespace := support.OperatorNamespace()
	if err := support.EnsureNamespace(managerCtx, testDirectClient, operatorNamespace); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create namespace: %v\n", err)
		return 1
	}

	moduleCRD := &apiextensionsv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: componentsv1alpha1.WorkbenchesCRDName},
	}
	if err := testDirectClient.Get(managerCtx, client.ObjectKeyFromObject(moduleCRD), moduleCRD); err != nil {
		fmt.Fprintf(
			os.Stderr,
			"Expected CRD %s to be installed before running upgrade tests: %v\n",
			componentsv1alpha1.WorkbenchesCRDName,
			err,
		)
		return 1
	}

	_ = testDirectClient.DeleteAllOf(managerCtx, &componentsv1alpha1.Workbenches{})
	_ = testDirectClient.DeleteAllOf(managerCtx, &appsv1.Deployment{}, client.InNamespace(operatorNamespace))
	_ = testDirectClient.DeleteAllOf(managerCtx, &corev1.Service{}, client.InNamespace(operatorNamespace))

	module.Version = desiredUpgradeVersion

	testModuleCfg, err = loadOperatorConfig(operatorNamespace)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load operator config: %v\n", err)
		return 1
	}

	logger, err := testModuleCfg.Controller.Zap.NewLogger()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to build zap logger: %v\n", err)
		return 1
	}

	ctrl.SetLogger(logger)

	return m.Run()
}

func startManager(t *testing.T, s suite) suite {
	t.Helper()
	if s.k8sClient != nil {
		return s
	}

	mgr, err := workbenchesmanager.New(
		managerCtx,
		testKubeConfig,
		testModuleCfg,
		func(opts *ctrl.Options) {
			opts.Cache.DefaultTransform = nil
			opts.Cache.ReaderFailOnMissingInformer = false
		},
	)
	NewWithT(t).Expect(err).NotTo(HaveOccurred())

	go func() {
		if err := mgr.Start(managerCtx); err != nil {
			fmt.Fprintf(os.Stderr, "Manager exited with error: %v\n", err)
		}
	}()

	if !mgr.GetCache().WaitForCacheSync(managerCtx) {
		t.Fatal("failed to sync manager cache")
	}

	testK8sClient = mgr.GetClient()
	return suite{
		directClient: s.directClient,
		k8sClient:    testK8sClient,
	}
}

func TestWorkbenchesUpgrade(t *testing.T) {
	t.Run("migration", (&migrationTests{
		s: newSuite(),
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
			ObjectMeta: metav1.ObjectMeta{Name: componentsv1alpha1.WorkbenchesCRDName},
		},
	}).Execute)
}
