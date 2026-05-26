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

	admissionv1 "k8s.io/api/admissionregistration/v1"
	imagev1 "github.com/openshift/api/image/v1"
	. "github.com/onsi/gomega"
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

	"github.com/lburgazzoli/gomega-matchers/pkg/matchers/jq"
	k8sm "github.com/lburgazzoli/gomega-matchers/pkg/matchers/k8s"

	componentsv1alpha1 "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-trainer-operator/api/components/v1alpha1"
	trainercontroller "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-trainer-operator/internal/controller/trainer"
	moduleconfig "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-trainer-operator/pkg/config"
	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-trainer-operator/test/support"
	"github.com/opendatahub-io/opendatahub-operator/v2/pkg/cluster"
	"github.com/opendatahub-io/opendatahub-operator/v2/pkg/controller/status"
	odhmanager "github.com/opendatahub-io/opendatahub-operator/v2/pkg/manager"
)

const (
	defaultTestNamespace = "integration-test"
	timeout              = 90 * time.Second
	interval             = 2 * time.Second

	labelPartOf            = "platform.opendatahub.io/part-of"
	annotationInstanceName = "platform.opendatahub.io/instance.name"
	annotationInstanceUID  = "platform.opendatahub.io/instance.uid"
	annotationType         = "platform.opendatahub.io/type"
	annotationVersion      = "platform.opendatahub.io/version"
	moduleCRDName          = "trainers.components.platform.opendatahub.io"
	jobSetOperatorCRDName  = "jobsetoperators.operator.openshift.io"
	jobSetOperatorCRName   = "cluster"
	jobSetCRDName          = "jobsets.jobset.x-k8s.io"
	workloadDeployName     = "kubeflow-trainer-controller-manager"
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

func envOrDefault(key string, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}

	return defaultValue
}

func init() {
	utilruntime.Must(admissionv1.AddToScheme(testScheme))
	utilruntime.Must(clientgoscheme.AddToScheme(testScheme))
	utilruntime.Must(apiextensionsv1.AddToScheme(testScheme))
	utilruntime.Must(imagev1.AddToScheme(testScheme))
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
		fmt.Fprintf(os.Stderr, "Failed to create client: %v\n", err)
		return 1
	}

	testNamespace := envOrDefault("INTEGRATION_TEST_NAMESPACE", defaultTestNamespace)

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

	if err := support.EnsureStubCRD(ctx, directClient, jobSetOperatorCRDName,
		"operator.openshift.io", "v1", "JobSetOperator", "jobsetoperators"); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to install JobSetOperator stub CRD: %v\n", err)
		return 1
	}
	if err := support.EnsureStubJobSetOperatorCR(ctx, directClient); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create JobSetOperator stub CR: %v\n", err)
		return 1
	}
	if err := support.EnsureStubCRD(ctx, directClient, jobSetCRDName,
		"jobset.x-k8s.io", "v1alpha2", "JobSet", "jobsets"); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to install JobSet stub CRD: %v\n", err)
		return 1
	}

	// Clean up leftovers from previous runs.
	_ = directClient.DeleteAllOf(ctx, &componentsv1alpha1.Trainer{})
	_ = directClient.DeleteAllOf(ctx, &appsv1.Deployment{}, client.InNamespace(testNamespace))
	_ = directClient.DeleteAllOf(ctx, &corev1.Service{}, client.InNamespace(testNamespace))

	viper.Set("rhai-applications-namespace", testNamespace)
	cluster.SetRHAIApplicationNamespace(testNamespace)

	operatorCfgData = support.MustReadConfigMapData(
		support.MustProjectFile("config", "manager", "configmap.yaml"))

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
				},
			},
		},
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create manager: %v\n", err)
		return 1
	}

	mgr := odhmanager.New(ctrlMgr, odhmanager.WithManifestsBasePath(
		support.MustProjectFile("config", "manifests")))

	if err := trainercontroller.NewReconciler(ctx, mgr, moduleCfg, moduleCfg.Release()); err != nil {
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

type trainerTest struct {
	module         *componentsv1alpha1.Trainer
	moduleCRD      *apiextensionsv1.CustomResourceDefinition
	workloadDeploy *appsv1.Deployment
}

func TestTrainer(t *testing.T) {
	testNamespace := envOrDefault("INTEGRATION_TEST_NAMESPACE", defaultTestNamespace)

	rt := &trainerTest{
		module: &componentsv1alpha1.Trainer{
			ObjectMeta: metav1.ObjectMeta{
				Name: componentsv1alpha1.TrainerInstanceName,
			},
		},
		moduleCRD: &apiextensionsv1.CustomResourceDefinition{
			ObjectMeta: metav1.ObjectMeta{Name: moduleCRDName},
		},
		workloadDeploy: &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      workloadDeployName,
				Namespace: testNamespace,
			},
		},
	}

	// Clean up any leftover CR from a previous run before starting.
	_ = k8sClient.Delete(ctx, rt.module)
	g := NewWithT(t)
	g.Eventually(func(g Gomega) {
		module := &componentsv1alpha1.Trainer{}
		err := k8sClient.Get(ctx, client.ObjectKeyFromObject(rt.module), module)
		g.Expect(k8serr.IsNotFound(err)).To(BeTrue())
	}).WithContext(ctx).WithTimeout(timeout).WithPolling(interval).Should(Succeed())
	rt.module.ResourceVersion = ""

	t.Cleanup(func() {
		_ = k8sClient.Delete(ctx, rt.module)
	})

	t.Run("should have module CRD installed", rt.testModuleCRDInstalled)
	t.Run("should report not ready when JobSet operator CRD is missing", rt.testJobSetOperatorCRDMissing)
	t.Run("should report not ready when JobSet operator CR is missing", rt.testJobSetOperatorCRMissing)
	t.Run("should report not ready when JobSet CRD is missing", rt.testJobSetCRDMissing)
	t.Run("should become ready", rt.testBecomesReady)
	t.Run("should report module version and platform", rt.testModuleStatus)
	t.Run("should set platform labels and annotations", rt.testPlatformLabels)
	t.Run("should set owner references", rt.testOwnerReferences)
}

func (rt *trainerTest) testModuleCRDInstalled(t *testing.T) {
	g := NewWithT(t)

	g.Eventually(k.Get(rt.moduleCRD)).WithContext(ctx).WithTimeout(timeout).WithPolling(interval).Should(
		jq.Match(`.metadata.name == "%s"`, moduleCRDName),
	)
}

func (rt *trainerTest) waitForModuleDeleted(t *testing.T) {
	t.Helper()

	g := NewWithT(t)
	g.Eventually(func(g Gomega) {
		module := &componentsv1alpha1.Trainer{}
		err := k8sClient.Get(ctx, client.ObjectKeyFromObject(rt.module), module)
		g.Expect(k8serr.IsNotFound(err)).To(BeTrue())
	}).WithContext(ctx).WithTimeout(timeout).WithPolling(interval).Should(Succeed())
	rt.module.ResourceVersion = ""
}

func waitForDeleted(
	t *testing.T,
	obj client.Object,
) {
	t.Helper()

	g := NewWithT(t)
	g.Eventually(func(g Gomega) {
		fresh := obj.DeepCopyObject().(client.Object)
		err := directClient.Get(ctx, client.ObjectKeyFromObject(obj), fresh)
		g.Expect(k8serr.IsNotFound(err)).To(BeTrue())
	}).WithContext(ctx).WithTimeout(timeout).WithPolling(interval).Should(Succeed())
}

func (rt *trainerTest) expectDependenciesUnavailable(
	t *testing.T,
	expectedMessage string,
) {
	t.Helper()

	g := NewWithT(t)
	g.Eventually(k.Get(rt.module)).WithContext(ctx).WithTimeout(timeout).WithPolling(interval).Should(And(
		jq.Match(`(.status.conditions // []) | any(.[]; .type == "%s" and .status == "%s")`,
			status.ConditionDependenciesAvailable, metav1.ConditionFalse),
		jq.Match(`(.status.conditions // []) | any(.[]; .type == "%s" and .reason == "%s")`,
			status.ConditionDependenciesAvailable, "PreConditionFailed"),
		jq.Match(`(.status.conditions // []) | any(.[]; .type == "%s" and .message == "%s")`,
			status.ConditionDependenciesAvailable, expectedMessage),
		jq.Match(`(.status.conditions // []) | any(.[]; .type == "%s" and .status == "%s")`,
			status.ConditionTypeReady, metav1.ConditionFalse),
		jq.Match(`(.status.conditions // []) | any(.[]; .type == "%s" and .status == "%s")`,
			status.ConditionTypeProvisioningSucceeded, metav1.ConditionFalse),
	))
}

func (rt *trainerTest) testJobSetOperatorCRDMissing(t *testing.T) {
	g := NewWithT(t)

	jobSetOperatorCRD := &apiextensionsv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: jobSetOperatorCRDName},
	}
	g.Expect(directClient.Delete(ctx, jobSetOperatorCRD)).To(Succeed())
	waitForDeleted(t, jobSetOperatorCRD)
	t.Cleanup(func() {
		_ = support.EnsureStubCRD(ctx, directClient, jobSetOperatorCRDName,
			"operator.openshift.io", "v1", "JobSetOperator", "jobsetoperators")
		_ = support.EnsureStubJobSetOperatorCR(ctx, directClient)
		_ = k8sClient.Delete(ctx, rt.module)
		rt.waitForModuleDeleted(t)
	})

	rt.module.ResourceVersion = ""
	g.Expect(k8sClient.Create(ctx, rt.module)).To(Succeed())
	rt.expectDependenciesUnavailable(t, status.JobSetOperatorNotInstalledMessage)
}

func (rt *trainerTest) testJobSetOperatorCRMissing(t *testing.T) {
	g := NewWithT(t)

	jobSetOperatorCR := support.NewStubJobSetOperatorCR()
	g.Expect(directClient.Delete(ctx, jobSetOperatorCR)).To(Succeed())
	waitForDeleted(t, jobSetOperatorCR)
	t.Cleanup(func() {
		_ = support.EnsureStubJobSetOperatorCR(ctx, directClient)
		_ = k8sClient.Delete(ctx, rt.module)
		rt.waitForModuleDeleted(t)
	})

	rt.module.ResourceVersion = ""
	g.Expect(k8sClient.Create(ctx, rt.module)).To(Succeed())
	rt.expectDependenciesUnavailable(t, status.JobSetOperatorCRNotFoundMessage)
}

func (rt *trainerTest) testJobSetCRDMissing(t *testing.T) {
	g := NewWithT(t)

	jobSetCRD := &apiextensionsv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: jobSetCRDName},
	}
	g.Expect(directClient.Delete(ctx, jobSetCRD)).To(Succeed())
	waitForDeleted(t, jobSetCRD)
	t.Cleanup(func() {
		_ = support.EnsureStubCRD(ctx, directClient, jobSetCRDName,
			"jobset.x-k8s.io", "v1alpha2", "JobSet", "jobsets")
		_ = k8sClient.Delete(ctx, rt.module)
		rt.waitForModuleDeleted(t)
	})

	rt.module.ResourceVersion = ""
	g.Expect(k8sClient.Create(ctx, rt.module)).To(Succeed())
	rt.expectDependenciesUnavailable(t, status.JobSetCRDMissingMessage)
}

func (rt *trainerTest) testBecomesReady(t *testing.T) {
	g := NewWithT(t)

	rt.module.ResourceVersion = ""
	g.Expect(k8sClient.Create(ctx, rt.module)).To(Succeed())

	g.Eventually(k.Get(rt.module)).WithContext(ctx).WithTimeout(timeout).WithPolling(interval).Should(And(
		jq.Match(`.status.phase == "Ready"`),
		jq.Match(`.status.conditions[] | select(.type == "Ready") | .status == "True"`),
		jq.Match(`.status.conditions[] | select(.type == "ProvisioningSucceeded") | .status == "True"`),
	))

	g.Eventually(k.Get(rt.workloadDeploy)).WithContext(ctx).WithTimeout(timeout).WithPolling(interval).Should(
		jq.Match(`.status.readyReplicas >= 1`),
	)
}

func (rt *trainerTest) testModuleStatus(t *testing.T) {
	g := NewWithT(t)

	g.Eventually(k.Get(rt.module)).WithContext(ctx).WithTimeout(timeout).WithPolling(interval).Should(And(
		jq.Match(`.status.module.version != ""`),
		jq.Match(`.status.module.platform.name == "%s"`,
			operatorCfgData[moduleconfig.KeyPlatformType]),
		jq.Match(`.status.module.sources | length > 0`),
		jq.Match(`.status.module.sources[0].renderer == "kustomize"`),
	))
}

func (rt *trainerTest) testPlatformLabels(t *testing.T) {
	g := NewWithT(t)

	g.Eventually(k.Get(rt.workloadDeploy)).WithContext(ctx).WithTimeout(timeout).WithPolling(interval).Should(And(
		jq.Match(`.metadata.labels."%s" == "trainer"`, labelPartOf),
		jq.Match(`.metadata.annotations."%s" != ""`, annotationInstanceName),
		jq.Match(`.metadata.annotations."%s" != ""`, annotationInstanceUID),
		jq.Match(`.metadata.annotations."%s" == "%s"`,
			annotationType,
			operatorCfgData[moduleconfig.KeyPlatformType]),
		jq.Match(`.metadata.annotations."%s" != ""`, annotationVersion),
	))
}

func (rt *trainerTest) testOwnerReferences(t *testing.T) {
	g := NewWithT(t)

	g.Eventually(k.Get(rt.workloadDeploy)).WithContext(ctx).WithTimeout(timeout).WithPolling(interval).Should(
		jq.Match(`.metadata.ownerReferences[] | select(.kind == "Trainer") | .name == "%s"`,
			componentsv1alpha1.TrainerInstanceName),
	)
}
