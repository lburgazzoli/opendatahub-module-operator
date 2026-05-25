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
	"testing"
	"time"

	componentsv1alpha1 "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-datasciencepipelines-operator/api/components/v1alpha1"
	dspcontroller "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-datasciencepipelines-operator/internal/controller/datasciencepipelines"
	moduleconfig "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-datasciencepipelines-operator/pkg/config"
	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-datasciencepipelines-operator/test/support"
	"github.com/lburgazzoli/gomega-matchers/pkg/matchers/jq"
	k8sm "github.com/lburgazzoli/gomega-matchers/pkg/matchers/k8s"
	. "github.com/onsi/gomega"
	operatorstatus "github.com/opendatahub-io/opendatahub-operator/v2/pkg/controller/status"
	"github.com/opendatahub-io/opendatahub-operator/v2/pkg/cluster"
	"github.com/opendatahub-io/opendatahub-operator/v2/pkg/metadata/labels"
	odhmanager "github.com/opendatahub-io/opendatahub-operator/v2/pkg/manager"
	promv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	"github.com/spf13/viper"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	k8serr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
)

const (
	timeout  = 90 * time.Second
	interval = 2 * time.Second

	labelPartOf            = "platform.opendatahub.io/part-of"
	annotationInstanceName = "platform.opendatahub.io/instance.name"
	annotationInstanceUID  = "platform.opendatahub.io/instance.uid"
	annotationType         = "platform.opendatahub.io/type"
	annotationVersion      = "platform.opendatahub.io/version"

	moduleCRDName            = "datasciencepipelines.components.platform.opendatahub.io"
	argoWorkflowCRDName      = "workflows.argoproj.io"
	testManagedByLabel       = "testing.opendatahub.io/managed-by"
	testManagedByValue       = "dsp-integration"
	legacyComponentName      = "data-science-pipelines-operator"
	workloadDeploymentName   = "data-science-pipelines-operator-controller-manager"
	workloadConfigMapName    = "data-science-pipelines-operator-dspo-config"
	workloadServiceMonName   = "data-science-pipelines-operator-service-monitor"
)

var (
	ctx             context.Context
	cancel          context.CancelFunc
	k8sClient       client.Client
	directClient    client.Client
	k               *k8sm.Matcher
	operatorCfgData map[string]string
	testScheme      = runtime.NewScheme()
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(testScheme))
	utilruntime.Must(apiextensionsv1.AddToScheme(testScheme))
	utilruntime.Must(promv1.AddToScheme(testScheme))
	utilruntime.Must(componentsv1alpha1.AddToScheme(testScheme))
}

func TestMain(m *testing.M) {
	os.Exit(runTestMain(m))
}

func runTestMain(m *testing.M) int {
	ctx, cancel = context.WithCancel(context.Background())
	defer cancel()

	ctrl.SetLogger(zap.New(zap.UseDevMode(true)))

	cfg, err := config.GetConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to get kubeconfig: %v\n", err)
		return 1
	}

	directClient, err = client.New(cfg, client.Options{Scheme: testScheme})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create direct client: %v\n", err)
		return 1
	}

	testNamespace := support.HelmNamespace()
	if err := support.EnsureNamespace(ctx, directClient, testNamespace); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create namespace: %v\n", err)
		return 1
	}

	moduleCRD := &apiextensionsv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: moduleCRDName},
	}
	if err := directClient.Get(ctx, client.ObjectKeyFromObject(moduleCRD), moduleCRD); err != nil {
		fmt.Fprintf(os.Stderr, "Expected CRD %s to be installed before running integration tests: %v\n", moduleCRDName, err)
		return 1
	}

	_ = directClient.DeleteAllOf(ctx, &componentsv1alpha1.DataSciencePipelines{})
	_ = directClient.DeleteAllOf(ctx, &appsv1.Deployment{}, client.InNamespace(testNamespace))
	_ = directClient.DeleteAllOf(ctx, &corev1.Service{}, client.InNamespace(testNamespace))
	_ = directClient.DeleteAllOf(ctx, &corev1.ConfigMap{}, client.InNamespace(testNamespace))
	_ = directClient.DeleteAllOf(ctx, &promv1.ServiceMonitor{}, client.InNamespace(testNamespace))

	viper.Set("rhai-applications-namespace", testNamespace)
	cluster.SetRHAIApplicationNamespace(testNamespace)

	operatorCfgData = support.MustReadConfigMapData(
		support.MustProjectFile("config", "manager", "configmap.yaml"),
	)

	moduleCfg := &moduleconfig.Config{
		PlatformType:          operatorCfgData[moduleconfig.KeyPlatformType],
		PlatformVersion:       operatorCfgData[moduleconfig.KeyPlatformVersion],
		ApplicationsNamespace: testNamespace,
		ManifestsPath:         support.MustProjectFile("config", "manifests"),
	}

	ctrlMgr, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme:         testScheme,
		Metrics:        metricsserver.Options{BindAddress: "0"},
		LeaderElection: false,
		Cache: cache.Options{
			DefaultNamespaces: map[string]cache.Config{
				testNamespace:       {},
				cache.AllNamespaces: {},
			},
		},
		Client: client.Options{
			Cache: &client.CacheOptions{
				Unstructured: true,
				DisableFor: []client.Object{
					&corev1.ConfigMap{},
					&corev1.Secret{},
					&apiextensionsv1.CustomResourceDefinition{},
				},
			},
		},
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create manager: %v\n", err)
		return 1
	}

	mgr := odhmanager.New(ctrlMgr, odhmanager.WithManifestsBasePath(
		support.MustProjectFile("config", "manifests"),
	))
	if err := dspcontroller.NewReconciler(ctx, mgr, moduleCfg, moduleCfg.Release()); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create reconciler: %v\n", err)
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

	k8sClient = mgr.GetClient()
	k = k8sm.New(k8sClient, testScheme)

	_ = directClient.Create(ctx, &rbacv1.ClusterRole{
		ObjectMeta: ctrl.ObjectMeta{Name: "integration-test-role"},
		Rules: []rbacv1.PolicyRule{{
			APIGroups: []string{"*"},
			Resources: []string{"*"},
			Verbs:     []string{"*"},
		}},
	})
	_ = directClient.Create(ctx, &rbacv1.ClusterRoleBinding{
		ObjectMeta: ctrl.ObjectMeta{Name: "integration-test-binding"},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "ClusterRole",
			Name:     "integration-test-role",
		},
		Subjects: []rbacv1.Subject{{
			Kind:     "Group",
			Name:     "system:masters",
			APIGroup: "rbac.authorization.k8s.io",
		}},
	})

	return m.Run()
}

type dspIntegrationTest struct {
	module             *componentsv1alpha1.DataSciencePipelines
	moduleCRD          *apiextensionsv1.CustomResourceDefinition
	workloadDeploy     *appsv1.Deployment
	workloadConfigMap  *corev1.ConfigMap
	workloadServiceMon *promv1.ServiceMonitor
}

func TestDataSciencePipelines(t *testing.T) {
	testNamespace := support.HelmNamespace()

	dt := &dspIntegrationTest{
		module: &componentsv1alpha1.DataSciencePipelines{
			ObjectMeta: metav1.ObjectMeta{
				Name: componentsv1alpha1.DataSciencePipelinesInstanceName,
			},
		},
		moduleCRD: &apiextensionsv1.CustomResourceDefinition{
			ObjectMeta: metav1.ObjectMeta{Name: moduleCRDName},
		},
		workloadDeploy: &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      workloadDeploymentName,
				Namespace: testNamespace,
			},
		},
		workloadConfigMap: &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      workloadConfigMapName,
				Namespace: testNamespace,
			},
		},
		workloadServiceMon: &promv1.ServiceMonitor{
			ObjectMeta: metav1.ObjectMeta{
				Name:      workloadServiceMonName,
				Namespace: testNamespace,
			},
		},
	}

	cleanupModule(t, dt.module)
	t.Cleanup(func() { cleanupModule(t, dt.module) })

	t.Run("should have module CRD installed", dt.testModuleCRDInstalled)
	t.Run("should fail when workflows CRD is missing and Argo is removed", dt.testMissingArgoWorkflowCRD)
	t.Run("should fail when workflows CRD is not ODH-owned", dt.testForeignOwnedArgoWorkflowCRD)
	t.Run("should become ready when workflows CRD is ODH-owned", dt.testBecomesReady)
	t.Run("should report module version and platform", dt.testModuleStatus)
	t.Run("should set platform labels and annotations", dt.testPlatformLabels)
	t.Run("should set owner references", dt.testOwnerReferences)
}

func cleanupModule(t *testing.T, module *componentsv1alpha1.DataSciencePipelines) {
	t.Helper()

	_ = k8sClient.Delete(ctx, module)
	g := NewWithT(t)
	g.Eventually(func(g Gomega) {
		current := &componentsv1alpha1.DataSciencePipelines{}
		err := directClient.Get(ctx, client.ObjectKeyFromObject(module), current)
		g.Expect(k8serr.IsNotFound(err)).To(BeTrue())
	}).WithContext(ctx).WithTimeout(timeout).WithPolling(interval).Should(Succeed())
	module.ResourceVersion = ""
	module.UID = ""
}

func manageArgoWorkflowCRD(t *testing.T) {
	t.Helper()

	state, err := support.CaptureWorkflowCRDState(ctx, directClient, argoWorkflowCRDName)
	if err != nil {
		t.Fatalf("capturing workflows CRD state: %v", err)
	}

	t.Cleanup(func() {
		if err := support.RestoreWorkflowCRDState(
			ctx,
			directClient,
			argoWorkflowCRDName,
			state,
			testManagedByLabel,
			testManagedByValue,
		); err != nil {
			t.Fatalf("restoring workflows CRD state: %v", err)
		}
	})
}

func ensureArgoWorkflowCRDMissing(t *testing.T) {
	t.Helper()
	manageArgoWorkflowCRD(t)

	crd := &apiextensionsv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: argoWorkflowCRDName},
	}
	err := directClient.Get(ctx, client.ObjectKeyFromObject(crd), crd)
	if k8serr.IsNotFound(err) {
		return
	}
	if err != nil {
		t.Fatalf("getting workflows CRD: %v", err)
	}
	if err := directClient.Delete(ctx, crd); err != nil && !k8serr.IsNotFound(err) {
		t.Fatalf("deleting workflows CRD: %v", err)
	}
	g := NewWithT(t)
	g.Eventually(func(g Gomega) {
		current := &apiextensionsv1.CustomResourceDefinition{}
		err := directClient.Get(ctx, client.ObjectKey{Name: argoWorkflowCRDName}, current)
		g.Expect(k8serr.IsNotFound(err)).To(BeTrue())
	}).WithContext(ctx).WithTimeout(timeout).WithPolling(interval).Should(Succeed())
}

func ensureArgoWorkflowCRDOwnedByODH(t *testing.T) {
	t.Helper()
	manageArgoWorkflowCRD(t)

	crd := loadOrCreateWorkflowCRD(t)
	odhLabel := labels.ODH.Component(legacyComponentName)
	if crd.Labels[odhLabel] == "true" {
		return
	}
	updateWorkflowCRDEventually(t, func(crd *apiextensionsv1.CustomResourceDefinition) {
		crd.Labels[odhLabel] = "true"
		crd.Labels[testManagedByLabel] = testManagedByValue
	})
}

func ensureArgoWorkflowCRDForeignOwned(t *testing.T) {
	t.Helper()
	manageArgoWorkflowCRD(t)

	crd := loadOrCreateWorkflowCRD(t)
	odhLabel := labels.ODH.Component(legacyComponentName)
	if crd.Labels[odhLabel] != "true" && crd.Labels[testManagedByLabel] == testManagedByValue {
		return
	}
	updateWorkflowCRDEventually(t, func(crd *apiextensionsv1.CustomResourceDefinition) {
		delete(crd.Labels, odhLabel)
		crd.Labels[testManagedByLabel] = testManagedByValue
	})
}

func loadOrCreateWorkflowCRD(t *testing.T) *apiextensionsv1.CustomResourceDefinition {
	t.Helper()

	crd := &apiextensionsv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: argoWorkflowCRDName},
	}
	err := directClient.Get(ctx, client.ObjectKeyFromObject(crd), crd)
	if err == nil {
		return updateWorkflowCRDEventually(t, func(crd *apiextensionsv1.CustomResourceDefinition) {
			crd.Labels[testManagedByLabel] = testManagedByValue
		})
	}
	if !k8serr.IsNotFound(err) {
		t.Fatalf("getting workflows CRD: %v", err)
	}

	crdPath := support.MustProjectFile(
		"config", "manifests", "datasciencepipelines", "argo", "crd.workflows.yaml",
	)
	if err := support.InstallCRDFile(ctx, directClient, crdPath); err != nil {
		t.Fatalf("installing workflows CRD: %v", err)
	}
	return updateWorkflowCRDEventually(t, func(crd *apiextensionsv1.CustomResourceDefinition) {
		crd.Labels[testManagedByLabel] = testManagedByValue
	})
}

func updateWorkflowCRDEventually(
	t *testing.T,
	mutate func(crd *apiextensionsv1.CustomResourceDefinition),
) *apiextensionsv1.CustomResourceDefinition {
	t.Helper()

	g := NewWithT(t)
	g.Eventually(func() error {
		latest := &apiextensionsv1.CustomResourceDefinition{}
		if err := directClient.Get(ctx, client.ObjectKey{Name: argoWorkflowCRDName}, latest); err != nil {
			return err
		}
		if latest.Labels == nil {
			latest.Labels = map[string]string{}
		}
		mutate(latest)
		return directClient.Update(ctx, latest)
	}).WithContext(ctx).WithTimeout(timeout).WithPolling(interval).Should(Succeed())

	current := &apiextensionsv1.CustomResourceDefinition{}
	if err := directClient.Get(ctx, client.ObjectKey{Name: argoWorkflowCRDName}, current); err != nil {
		t.Fatalf("reloading workflows CRD after update: %v", err)
	}

	return current
}

func createModule(t *testing.T, module *componentsv1alpha1.DataSciencePipelines) {
	t.Helper()
	cleanupModule(t, module)
	if err := k8sClient.Create(ctx, module); err != nil {
		t.Fatalf("creating module: %v", err)
	}
}

func (dt *dspIntegrationTest) testModuleCRDInstalled(t *testing.T) {
	g := NewWithT(t)
	g.Eventually(k.Get(dt.moduleCRD)).WithContext(ctx).WithTimeout(timeout).WithPolling(interval).Should(
		jq.Match(`.metadata.name == "%s"`, moduleCRDName),
	)
}

func (dt *dspIntegrationTest) testMissingArgoWorkflowCRD(t *testing.T) {
	ensureArgoWorkflowCRDMissing(t)

	module := dt.module.DeepCopy()
	module.Spec.ArgoWorkflowsControllers = &componentsv1alpha1.ArgoWorkflowsControllersSpec{
		ManagementState: "Removed",
	}
	createModule(t, module)

	g := NewWithT(t)
	g.Eventually(k.Get(module)).WithContext(ctx).WithTimeout(timeout).WithPolling(interval).Should(And(
		jq.Match(`.status.conditions[]? | select(.type == "%s") | .status == "False"`,
			operatorstatus.ConditionArgoWorkflowAvailable),
		jq.Match(`.status.conditions[]? | select(.type == "%s") | .reason == "%s"`,
			operatorstatus.ConditionArgoWorkflowAvailable,
			operatorstatus.DataSciencePipelinesArgoWorkflowsCRDMissingReason),
		jq.Match(`.status.conditions[]? | select(.type == "%s") | .status == "False"`,
			operatorstatus.ConditionTypeReady),
	))
}

func (dt *dspIntegrationTest) testForeignOwnedArgoWorkflowCRD(t *testing.T) {
	ensureArgoWorkflowCRDForeignOwned(t)

	module := dt.module.DeepCopy()
	createModule(t, module)

	g := NewWithT(t)
	g.Eventually(k.Get(module)).WithContext(ctx).WithTimeout(timeout).WithPolling(interval).Should(And(
		jq.Match(`.status.conditions[]? | select(.type == "%s") | .status == "False"`,
			operatorstatus.ConditionArgoWorkflowAvailable),
		jq.Match(`.status.conditions[]? | select(.type == "%s") | .reason == "%s"`,
			operatorstatus.ConditionArgoWorkflowAvailable,
			operatorstatus.DataSciencePipelinesDoesntOwnArgoCRDReason),
	))
}

func (dt *dspIntegrationTest) testBecomesReady(t *testing.T) {
	ensureArgoWorkflowCRDOwnedByODH(t)

	module := dt.module.DeepCopy()
	createModule(t, module)

	g := NewWithT(t)
	g.Eventually(k.Get(module)).WithContext(ctx).WithTimeout(timeout).WithPolling(interval).Should(And(
		jq.Match(`.status.phase == "Ready"`),
		jq.Match(`.status.conditions[]? | select(.type == "%s") | .status == "True"`,
			operatorstatus.ConditionTypeReady),
		jq.Match(`.status.conditions[]? | select(.type == "%s") | .status == "True"`,
			operatorstatus.ConditionArgoWorkflowAvailable),
		jq.Match(`.status.conditions[]? | select(.type == "%s") | .status == "True"`,
			operatorstatus.ConditionTypeProvisioningSucceeded),
	))

	g.Eventually(k.Get(dt.workloadDeploy)).WithContext(ctx).WithTimeout(timeout).WithPolling(interval).Should(
		jq.Match(`.status.readyReplicas >= 1`),
	)
	g.Eventually(k.Get(dt.workloadConfigMap)).WithContext(ctx).WithTimeout(timeout).WithPolling(interval).Should(
		jq.Match(`.metadata.name == "%s"`, workloadConfigMapName),
	)
	g.Eventually(k.Get(dt.workloadServiceMon)).WithContext(ctx).WithTimeout(timeout).WithPolling(interval).Should(
		jq.Match(`.metadata.name == "%s"`, workloadServiceMonName),
	)
}

func (dt *dspIntegrationTest) testModuleStatus(t *testing.T) {
	ensureArgoWorkflowCRDOwnedByODH(t)

	module := dt.module.DeepCopy()
	createModule(t, module)

	g := NewWithT(t)
	g.Eventually(k.Get(module)).WithContext(ctx).WithTimeout(timeout).WithPolling(interval).Should(And(
		jq.Match(`.status.module.version != ""`),
		jq.Match(`.status.module.platform.name == "%s"`,
			operatorCfgData[moduleconfig.KeyPlatformType]),
		jq.Match(`.status.module.sources | length > 0`),
		jq.Match(`.status.module.sources[0].renderer == "kustomize"`),
	))
}

func (dt *dspIntegrationTest) testPlatformLabels(t *testing.T) {
	ensureArgoWorkflowCRDOwnedByODH(t)

	module := dt.module.DeepCopy()
	createModule(t, module)

	g := NewWithT(t)
	g.Eventually(k.Get(dt.workloadDeploy)).WithContext(ctx).WithTimeout(timeout).WithPolling(interval).Should(And(
		jq.Match(`.metadata.labels."%s" == "%s"`, labelPartOf, componentsv1alpha1.DataSciencePipelinesComponentName),
		jq.Match(`.metadata.annotations."%s" != ""`, annotationInstanceName),
		jq.Match(`.metadata.annotations."%s" != ""`, annotationInstanceUID),
		jq.Match(`.metadata.annotations."%s" == "%s"`,
			annotationType,
			operatorCfgData[moduleconfig.KeyPlatformType]),
		jq.Match(`.metadata.annotations."%s" != ""`, annotationVersion),
	))
}

func (dt *dspIntegrationTest) testOwnerReferences(t *testing.T) {
	ensureArgoWorkflowCRDOwnedByODH(t)

	module := dt.module.DeepCopy()
	createModule(t, module)

	g := NewWithT(t)
	g.Eventually(k.Get(dt.workloadDeploy)).WithContext(ctx).WithTimeout(timeout).WithPolling(interval).Should(
		jq.Match(`.metadata.ownerReferences[] | select(.kind == "DataSciencePipelines") | .name == "%s"`,
			componentsv1alpha1.DataSciencePipelinesInstanceName),
	)
}
