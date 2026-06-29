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
	k8serr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	promv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"

	componentsv1alpha1 "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-datasciencepipelines-operator/api/components/v1alpha1"
	dspcontroller "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-datasciencepipelines-operator/internal/controller/datasciencepipelines"
	moduleconfig "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-datasciencepipelines-operator/pkg/config"
	modulemanager "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-datasciencepipelines-operator/pkg/manager"
	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-datasciencepipelines-operator/test/support"
	"github.com/opendatahub-io/odh-platform-utilities/pkg/metadata/labels"

	k8sm "github.com/lburgazzoli/gomega-matchers/pkg/matchers/k8s"
)

const (
	labelTrue          = "true"
	testManagedByLabel = "testing.opendatahub.io/managed-by"
	testManagedByValue = "dsp-integration"

	workloadConfigMapName  = "data-science-pipelines-operator-dspo-config"
	workloadServiceMonName = "data-science-pipelines-operator-service-monitor"
)

func loadOperatorConfig() (*moduleconfig.Config, error) {
	moduleCfg, err := moduleconfig.LoadFromFS(nil)
	if err != nil {
		return nil, fmt.Errorf("loading operator config: %w", err)
	}

	moduleCfg.ApplicationsNamespace = support.IntegrationTestNamespace()

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
		ObjectMeta: metav1.ObjectMeta{Name: componentsv1alpha1.DataSciencePipelinesCRDName},
	}
	if err := cli.Get(ctx, client.ObjectKeyFromObject(moduleCRD), moduleCRD); err != nil {
		fmt.Fprintf(
			os.Stderr,
			"Expected CRD %s to be installed before running integration tests: %v\n",
			componentsv1alpha1.DataSciencePipelinesCRDName,
			err,
		)
		return 1
	}

	_ = cli.DeleteAllOf(ctx, &componentsv1alpha1.DataSciencePipelines{})
	_ = cli.DeleteAllOf(ctx, &appsv1.Deployment{}, client.InNamespace(moduleCfg.ApplicationsNamespace))
	_ = cli.DeleteAllOf(ctx, &corev1.Service{}, client.InNamespace(moduleCfg.ApplicationsNamespace))
	_ = cli.DeleteAllOf(ctx, &corev1.ConfigMap{}, client.InNamespace(moduleCfg.ApplicationsNamespace))
	_ = cli.DeleteAllOf(ctx, &promv1.ServiceMonitor{}, client.InNamespace(moduleCfg.ApplicationsNamespace))

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

func TestDataSciencePipelines(t *testing.T) {
	k8sClient, err := support.NewClient()
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	t.Run("foundation", (&foundationTests{Client: k8sClient}).Execute)
}

func manageArgoWorkflowCRD(t *testing.T, cli client.Client) {
	t.Helper()

	state, err := support.CaptureWorkflowCRDState(t.Context(), cli, dspcontroller.ArgoWorkflowCRD)
	if err != nil {
		t.Fatalf("capturing workflows CRD state: %v", err)
	}

	t.Cleanup(func() {
		if err := support.RestoreWorkflowCRDState(
			context.Background(),
			cli,
			dspcontroller.ArgoWorkflowCRD,
			state,
			testManagedByLabel,
			testManagedByValue,
		); err != nil {
			t.Fatalf("restoring workflows CRD state: %v", err)
		}
	})
}

func ensureArgoWorkflowCRDMissing(t *testing.T, cli client.Client) {
	t.Helper()
	ctx := t.Context()
	manageArgoWorkflowCRD(t, cli)

	crd := &apiextensionsv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: dspcontroller.ArgoWorkflowCRD},
	}
	err := cli.Get(ctx, client.ObjectKeyFromObject(crd), crd)
	if k8serr.IsNotFound(err) {
		return
	}
	if err != nil {
		t.Fatalf("getting workflows CRD: %v", err)
	}
	if err := cli.Delete(ctx, crd); err != nil && !k8serr.IsNotFound(err) {
		t.Fatalf("deleting workflows CRD: %v", err)
	}
	NewWithT(t).Eventually(ctx, k8sm.NotFound(cli, crd)).Should(BeTrue())
}

func ensureArgoWorkflowCRDOwnedByODH(t *testing.T, cli client.Client) {
	t.Helper()
	manageArgoWorkflowCRD(t, cli)

	crd := loadOrCreateWorkflowCRD(t, cli)
	odhLabel := labels.ODHAppPrefix + "/" + dspcontroller.LegacyComponentName
	if crd.Labels[odhLabel] == labelTrue {
		return
	}
	updateWorkflowCRDEventually(t, cli, func(crd *apiextensionsv1.CustomResourceDefinition) {
		crd.Labels[odhLabel] = labelTrue
		crd.Labels[testManagedByLabel] = testManagedByValue
	})
}

func ensureArgoWorkflowCRDForeignOwned(t *testing.T, cli client.Client) {
	t.Helper()
	manageArgoWorkflowCRD(t, cli)

	crd := loadOrCreateWorkflowCRD(t, cli)
	odhLabel := labels.ODHAppPrefix + "/" + dspcontroller.LegacyComponentName
	if crd.Labels[odhLabel] != labelTrue && crd.Labels[testManagedByLabel] == testManagedByValue {
		return
	}
	updateWorkflowCRDEventually(t, cli, func(crd *apiextensionsv1.CustomResourceDefinition) {
		delete(crd.Labels, odhLabel)
		crd.Labels[testManagedByLabel] = testManagedByValue
	})
}

func loadOrCreateWorkflowCRD(t *testing.T, cli client.Client) *apiextensionsv1.CustomResourceDefinition {
	t.Helper()
	ctx := t.Context()

	crd := &apiextensionsv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: dspcontroller.ArgoWorkflowCRD},
	}
	err := cli.Get(ctx, client.ObjectKeyFromObject(crd), crd)
	if err == nil {
		return updateWorkflowCRDEventually(t, cli, func(crd *apiextensionsv1.CustomResourceDefinition) {
			crd.Labels[testManagedByLabel] = testManagedByValue
		})
	}
	if !k8serr.IsNotFound(err) {
		t.Fatalf("getting workflows CRD: %v", err)
	}

	crdPath := support.MustProjectFile(
		"config", "manifests", "datasciencepipelines", "argo", "crd.workflows.yaml",
	)
	if err := support.InstallCRDFile(ctx, cli, crdPath); err != nil {
		t.Fatalf("installing workflows CRD: %v", err)
	}
	return updateWorkflowCRDEventually(t, cli, func(crd *apiextensionsv1.CustomResourceDefinition) {
		crd.Labels[testManagedByLabel] = testManagedByValue
	})
}

func updateWorkflowCRDEventually(
	t *testing.T,
	cli client.Client,
	mutate func(crd *apiextensionsv1.CustomResourceDefinition),
) *apiextensionsv1.CustomResourceDefinition {
	t.Helper()
	ctx := t.Context()

	g := NewWithT(t)
	g.Eventually(func() error {
		latest := &apiextensionsv1.CustomResourceDefinition{}
		if err := cli.Get(ctx, client.ObjectKey{Name: dspcontroller.ArgoWorkflowCRD}, latest); err != nil {
			return err
		}
		if latest.Labels == nil {
			latest.Labels = map[string]string{}
		}
		mutate(latest)
		return cli.Update(ctx, latest)
	}).WithContext(ctx).Should(Succeed())

	current := &apiextensionsv1.CustomResourceDefinition{}
	if err := cli.Get(ctx, client.ObjectKey{Name: dspcontroller.ArgoWorkflowCRD}, current); err != nil {
		t.Fatalf("reloading workflows CRD after update: %v", err)
	}

	return current
}

func createModule(t *testing.T, cli client.Client, module *componentsv1alpha1.DataSciencePipelines) {
	t.Helper()

	_ = cli.Delete(t.Context(), module)
	NewWithT(t).Eventually(t.Context(), k8sm.NotFound(cli, module)).Should(BeTrue())
	module.SetResourceVersion("")
	module.SetUID("")

	t.Cleanup(func() {
		_ = cli.Delete(context.Background(), module)
	})

	if err := cli.Create(t.Context(), module); err != nil {
		t.Fatalf("creating module: %v", err)
	}
}
