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
	"testing"

	. "github.com/onsi/gomega"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	componentsv1alpha1 "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-trainer-operator/api/components/v1alpha1"
	moduleconfig "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-trainer-operator/pkg/config"
	modulemanager "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-trainer-operator/pkg/manager"
	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-trainer-operator/test/support"
)

func loadOperatorConfig() (*moduleconfig.Config, error) {
	moduleCfg, err := moduleconfig.LoadFromFS(nil)
	if err != nil {
		return nil, fmt.Errorf("loading operator config: %w", err)
	}

	moduleCfg.ApplicationsNamespace = support.IntegrationTestNamespace()
	moduleCfg.ManifestsPath = support.MustProjectFile("config", "manifests")

	return moduleCfg, nil
}

func TestMain(m *testing.M) {
	os.Exit(runTestMain(m))
}

func runTestMain(m *testing.M) int {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	gomegaCfg, err := support.LoadGomegaConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load test config: %v\n", err)
		return 1
	}

	SetDefaultEventuallyTimeout(gomegaCfg.EventuallyTimeout)
	SetDefaultEventuallyPollingInterval(gomegaCfg.EventuallyPollingInterval)
	SetDefaultConsistentlyPollingInterval(gomegaCfg.ConsistentlyPollingInterval)

	ctrl.SetLogger(zap.New(zap.UseDevMode(true)))

	cfg, err := config.GetConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to get kubeconfig: %v\n", err)
		return 1
	}

	moduleCfg, err := loadOperatorConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load operator config: %v\n", err)
		return 1
	}
	moduleCfg.ApplicationsNamespace = support.IntegrationTestNamespace()
	moduleCfg.Controller.Metrics.BindAddress = "0"
	moduleCfg.Controller.Health.BindAddress = "0"
	moduleCfg.Controller.LeaderElection.Enabled = false
	moduleCfg.Controller.Pprof.BindAddress = "0"

	logger, err := moduleCfg.Controller.Zap.NewLogger()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to build zap logger: %v\n", err)
		return 1
	}

	ctrl.SetLogger(logger)

	cli, err := support.NewClient()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create client: %v\n", err)
		return 1
	}

	if err := support.EnsureNamespace(ctx, cli, moduleCfg.ApplicationsNamespace); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create namespace: %v\n", err)
		return 1
	}

	moduleCRD := &apiextensionsv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: componentsv1alpha1.TrainerCRDName},
	}
	if err := cli.Get(ctx, client.ObjectKeyFromObject(moduleCRD), moduleCRD); err != nil {
		fmt.Fprintf(
			os.Stderr,
			"Expected CRD %s to be installed before running integration tests: %v\n",
			componentsv1alpha1.TrainerCRDName,
			err,
		)
		return 1
	}

	if err := support.EnsureTrainerPreconditions(ctx, cli); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to install Trainer preconditions: %v\n", err)
		return 1
	}

	_ = cli.DeleteAllOf(ctx, &componentsv1alpha1.Trainer{})
	_ = cli.DeleteAllOf(ctx, &appsv1.Deployment{}, client.InNamespace(moduleCfg.ApplicationsNamespace))
	_ = cli.DeleteAllOf(ctx, &corev1.Service{}, client.InNamespace(moduleCfg.ApplicationsNamespace))

	mgr, err := modulemanager.New(ctx, cfg, moduleCfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create manager: %v\n", err)
		return 1
	}

	go func() {
		if err := mgr.Start(ctx); err != nil {
			fmt.Fprintf(os.Stderr, "Manager exited with error: %v\n", err)
		}
	}()

	if !mgr.GetCache().WaitForCacheSync(ctx) {
		fmt.Fprintf(os.Stderr, "Failed to sync manager cache\n")
		return 1
	}

	return m.Run()
}

func TestTrainer(t *testing.T) {
	k8sClient, err := support.NewClient()
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	t.Run("foundation", (&foundationTests{Client: k8sClient}).Execute)
}
