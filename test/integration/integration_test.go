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

	. "github.com/onsi/gomega"
	"github.com/spf13/viper"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
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

	componentsv1alpha1 "github.com/lburgazzoli/opendatahub-module-operator/api/components/v1alpha1"
	"github.com/lburgazzoli/opendatahub-module-operator/internal/controller/mymodule"
	moduleconfig "github.com/lburgazzoli/opendatahub-module-operator/pkg/config"
	"github.com/lburgazzoli/opendatahub-module-operator/test/support"
	"github.com/opendatahub-io/opendatahub-operator/v2/pkg/cluster"
	odhmanager "github.com/opendatahub-io/opendatahub-operator/v2/pkg/manager"
	"github.com/opendatahub-io/opendatahub-operator/v2/pkg/resources"
)

const (
	testNamespace = "integration-test"
	timeout       = 90 * time.Second
	interval      = 2 * time.Second

	labelPartOf            = "platform.opendatahub.io/part-of"
	annotationInstanceName = "platform.opendatahub.io/instance.name"
	annotationInstanceUID  = "platform.opendatahub.io/instance.uid"
	annotationType         = "platform.opendatahub.io/type"
	annotationVersion      = "platform.opendatahub.io/version"
	moduleCRDName          = "mymodules.components.platform.opendatahub.io"
)

var (
	ctx             context.Context
	cancel          context.CancelFunc
	k8sClient       client.Client
	k               *k8sm.Matcher
	operatorCfgData map[string]string
	testScheme      = runtime.NewScheme()
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(testScheme))
	utilruntime.Must(apiextensionsv1.AddToScheme(testScheme))
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

	directClient, err := client.New(cfg, client.Options{Scheme: testScheme})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create client: %v\n", err)
		return 1
	}

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

	// Clean up leftovers from previous runs.
	_ = directClient.DeleteAllOf(ctx, &componentsv1alpha1.MyModule{})
	_ = directClient.DeleteAllOf(ctx, &appsv1.Deployment{}, client.InNamespace(testNamespace))
	_ = directClient.DeleteAllOf(ctx, &corev1.Service{}, client.InNamespace(testNamespace))
	_ = directClient.DeleteAllOf(ctx, &networkingv1.Ingress{}, client.InNamespace(testNamespace))

	viper.Set("rhai-applications-namespace", testNamespace)
	cluster.SetRHAIApplicationNamespace(testNamespace)

	operatorCfgData = support.MustReadConfigMapData(
		support.MustProjectFile("config", "manager", "configmap.yaml"))

	moduleCfg := &moduleconfig.Config{
		PlatformType:          operatorCfgData[moduleconfig.KeyPlatformType],
		PlatformVersion:       operatorCfgData[moduleconfig.KeyPlatformVersion],
		ApplicationsNamespace: testNamespace,
		ManifestsPath:         support.MustProjectFile("config", "manifests"),
		WebhooksEnabled:       false,
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

	if err := mymodule.NewReconciler(ctx, mgr, moduleCfg, moduleCfg.Release()); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create reconciler: %v\n", err)
		return 1
	}

	go func() {
		if err := mgr.Start(ctx); err != nil {
			fmt.Fprintf(os.Stderr, "Manager exited with error: %v\n", err)
		}
	}()

	k8sClient = mgr.GetClient()
	k = k8sm.New(k8sClient, testScheme)

	// RBAC — ignore already-exists errors.
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

// myModuleTest holds shared test fixtures for the MyModule integration tests.
type myModuleTest struct {
	module          *componentsv1alpha1.MyModule
	moduleCRD       *apiextensionsv1.CustomResourceDefinition
	ingress         *networkingv1.Ingress
	workloadDeploy  *appsv1.Deployment
	workloadService *corev1.Service
}

func TestMyModule(t *testing.T) {
	mt := &myModuleTest{
		module: &componentsv1alpha1.MyModule{
			ObjectMeta: metav1.ObjectMeta{
				Name: componentsv1alpha1.MyModuleInstanceName,
			},
		},
		moduleCRD: &apiextensionsv1.CustomResourceDefinition{
			ObjectMeta: metav1.ObjectMeta{Name: moduleCRDName},
		},
		ingress: &networkingv1.Ingress{
			ObjectMeta: metav1.ObjectMeta{
				Name:      mymodule.IngressName,
				Namespace: testNamespace,
			},
			Spec: networkingv1.IngressSpec{
				DefaultBackend: &networkingv1.IngressBackend{
					Service: &networkingv1.IngressServiceBackend{
						Name: "mymodule-workload",
						Port: networkingv1.ServiceBackendPort{Number: 8080},
					},
				},
			},
		},
		workloadDeploy: &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "mymodule-workload",
				Namespace: testNamespace,
			},
		},
		workloadService: &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "mymodule-workload",
				Namespace: testNamespace,
			},
		},
	}

	// Clean up any leftover singleton objects from a previous run.
	_ = k8sClient.Delete(ctx, mt.module)
	_ = k8sClient.Delete(ctx, mt.ingress)
	g := NewWithT(t)
	g.Eventually(func(g Gomega) {
		module := &componentsv1alpha1.MyModule{}
		err := k8sClient.Get(ctx, client.ObjectKeyFromObject(mt.module), module)
		g.Expect(k8serr.IsNotFound(err)).To(BeTrue())
	}).WithContext(ctx).WithTimeout(timeout).WithPolling(interval).Should(Succeed())
	g.Eventually(func(g Gomega) {
		ingress := &networkingv1.Ingress{}
		err := k8sClient.Get(ctx, client.ObjectKeyFromObject(mt.ingress), ingress)
		g.Expect(k8serr.IsNotFound(err)).To(BeTrue())
	}).WithContext(ctx).WithTimeout(timeout).WithPolling(interval).Should(Succeed())
	mt.module.ResourceVersion = ""
	mt.ingress.ResourceVersion = ""

	t.Cleanup(func() {
		_ = k8sClient.Delete(ctx, mt.module)
		_ = k8sClient.Delete(ctx, mt.ingress)
	})

	t.Run("should have module CRD installed", mt.testModuleCRDInstalled)
	t.Run("should block when Ingress is missing", mt.testIngressBlocks)
	t.Run("should recover when Ingress is created", mt.testIngressRecovers)
	t.Run("should expose config values", mt.testConfigValues)
	t.Run("should report module version and platform", mt.testModuleStatus)
	t.Run("should set platform labels and annotations", mt.testPlatformLabels)
	t.Run("should set owner references", mt.testOwnerReferences)
	t.Run("should not annotate ingress on fresh install", mt.testUpgradeAnnotationAbsentOnFreshInstall)
	t.Run("should annotate ingress on upgrade", mt.testUpgradeAnnotatesIngress)
	t.Run("should not update version on upgrade fault", mt.testUpgradeFaultInjection)
}

func (mt *myModuleTest) testModuleCRDInstalled(t *testing.T) {
	g := NewWithT(t)

	g.Eventually(k.Get(mt.moduleCRD)).WithContext(ctx).WithTimeout(timeout).WithPolling(interval).Should(
		jq.Match(`.metadata.name == "%s"`, moduleCRDName),
	)
}

func (mt *myModuleTest) testIngressBlocks(t *testing.T) {
	g := NewWithT(t)

	_ = k8sClient.Delete(ctx, mt.module)
	_ = k8sClient.Delete(ctx, mt.ingress)

	mt.module.ResourceVersion = ""
	g.Expect(k8sClient.Create(ctx, mt.module)).To(Succeed())

	g.Eventually(k.Get(mt.module)).WithContext(ctx).WithTimeout(timeout).WithPolling(interval).Should(And(
		jq.Match(`.status.phase == "Not Ready"`),
		jq.Match(`[(.status.conditions // [])[] | select(.type == "%s" and .status == "False")] | length > 0`,
			mymodule.ConditionIngressAvailable),
	))
}

func (mt *myModuleTest) testIngressRecovers(t *testing.T) {
	g := NewWithT(t)

	mt.ingress.ResourceVersion = ""
	g.Expect(k8sClient.Create(ctx, mt.ingress)).To(Succeed())

	g.Eventually(k.Get(mt.module)).WithContext(ctx).WithTimeout(timeout).WithPolling(interval).Should(And(
		jq.Match(`.status.phase == "Ready"`),
		jq.Match(`.status.conditions[] | select(.type == "Ready") | .status == "True"`),
		jq.Match(`.status.conditions[] | select(.type == "%s") | .status == "True"`,
			mymodule.ConditionIngressAvailable),
		jq.Match(`.status.conditions[] | select(.type == "ProvisioningSucceeded") | .status == "True"`),
	))

	g.Eventually(k.Get(mt.workloadDeploy)).WithContext(ctx).WithTimeout(timeout).WithPolling(interval).Should(
		jq.Match(`.status.readyReplicas >= 1`),
	)
}

func (mt *myModuleTest) testConfigValues(t *testing.T) {
	g := NewWithT(t)

	g.Eventually(k.Get(mt.module)).WithContext(ctx).WithTimeout(timeout).WithPolling(interval).Should(And(
		jq.Match(`.status.configValues."%s" == "%s"`,
			moduleconfig.KeyPlatformType,
			operatorCfgData[moduleconfig.KeyPlatformType]),
		jq.Match(`.status.configValues."%s" == "%s"`,
			moduleconfig.KeyPlatformVersion,
			operatorCfgData[moduleconfig.KeyPlatformVersion]),
	))
}

func (mt *myModuleTest) testModuleStatus(t *testing.T) {
	g := NewWithT(t)

	g.Eventually(k.Get(mt.module)).WithContext(ctx).WithTimeout(timeout).WithPolling(interval).Should(And(
		jq.Match(`.status.module.version != ""`),
		jq.Match(`.status.module.platform.name == "%s"`,
			operatorCfgData[moduleconfig.KeyPlatformType]),
		jq.Match(`.status.module.sources | length > 0`),
		jq.Match(`.status.module.sources[0].renderer == "kustomize"`),
	))
}

func (mt *myModuleTest) testPlatformLabels(t *testing.T) {
	g := NewWithT(t)

	g.Eventually(k.Get(mt.workloadDeploy)).WithContext(ctx).WithTimeout(timeout).WithPolling(interval).Should(And(
		jq.Match(`.metadata.labels."%s" == "mymodule"`, labelPartOf),
		jq.Match(`.metadata.annotations."%s" != ""`, annotationInstanceName),
		jq.Match(`.metadata.annotations."%s" != ""`, annotationInstanceUID),
		jq.Match(`.metadata.annotations."%s" == "%s"`,
			annotationType,
			operatorCfgData[moduleconfig.KeyPlatformType]),
		jq.Match(`.metadata.annotations."%s" != ""`, annotationVersion),
	))

	g.Eventually(k.Get(mt.workloadService)).WithContext(ctx).WithTimeout(timeout).WithPolling(interval).Should(And(
		jq.Match(`.metadata.labels."%s" == "mymodule"`, labelPartOf),
		jq.Match(`.metadata.annotations."%s" == "%s"`,
			annotationType,
			operatorCfgData[moduleconfig.KeyPlatformType]),
		jq.Match(`.metadata.annotations."%s" != ""`, annotationVersion),
	))
}

func (mt *myModuleTest) testOwnerReferences(t *testing.T) {
	g := NewWithT(t)

	g.Eventually(k.Get(mt.workloadDeploy)).WithContext(ctx).WithTimeout(timeout).WithPolling(interval).Should(
		jq.Match(`.metadata.ownerReferences[] | select(.kind == "MyModule") | .name == "%s"`,
			componentsv1alpha1.MyModuleInstanceName),
	)

	g.Eventually(k.Get(mt.workloadService)).WithContext(ctx).WithTimeout(timeout).WithPolling(interval).Should(
		jq.Match(`.metadata.ownerReferences[] | select(.kind == "MyModule") | .name == "%s"`,
			componentsv1alpha1.MyModuleInstanceName),
	)
}

func (mt *myModuleTest) testUpgradeAnnotationAbsentOnFreshInstall(t *testing.T) {
	g := NewWithT(t)

	g.Eventually(k.Get(mt.ingress)).WithContext(ctx).WithTimeout(timeout).WithPolling(interval).Should(And(
		jq.Match(`.metadata.annotations // {} | has("%s") | not`, mymodule.AnnotationManagedVersion),
		jq.Match(`.metadata.annotations // {} | has("%s") | not`, mymodule.AnnotationUpgradedFrom),
	))
}

func (mt *myModuleTest) testUpgradeAnnotatesIngress(t *testing.T) {
	g := NewWithT(t)

	// Re-read the module to get the latest resourceVersion.
	g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mt.module), mt.module)).To(Succeed())

	// Patch the module status to simulate a previous lower version.
	patch := client.MergeFrom(mt.module.DeepCopy())
	mt.module.Status.Module.Version = "0.0.0-0"
	g.Expect(k8sClient.Status().Patch(ctx, mt.module, patch)).To(Succeed())

	// Re-read after status patch changed resourceVersion, then trigger reconcile
	// by touching an annotation.
	g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mt.module), mt.module)).To(Succeed())

	annPatch := client.MergeFrom(mt.module.DeepCopy())
	resources.SetAnnotation(mt.module, "test-trigger", time.Now().String())
	g.Expect(k8sClient.Patch(ctx, mt.module, annPatch)).To(Succeed())

	// Wait for the Ingress to get both upgrade annotations.
	g.Eventually(k.Get(mt.ingress)).WithContext(ctx).WithTimeout(timeout).WithPolling(interval).Should(And(
		jq.Match(`.metadata.annotations."%s" != ""`, mymodule.AnnotationManagedVersion),
		jq.Match(`.metadata.annotations."%s" == "0.0.0-0"`, mymodule.AnnotationUpgradedFrom),
	))
}

func (mt *myModuleTest) testUpgradeFaultInjection(t *testing.T) {
	g := NewWithT(t)

	// Record the current module version from status before injecting the fault.
	g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mt.module), mt.module)).To(Succeed())
	versionBefore := mt.module.Status.Module.Version.String()

	// Add the fault injection annotation to the Ingress.
	g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mt.ingress), mt.ingress)).To(Succeed())

	ingressPatch := client.MergeFrom(mt.ingress.DeepCopy())
	resources.SetAnnotation(mt.ingress, mymodule.AnnotationInjectUpgradeFault, "true")
	g.Expect(k8sClient.Patch(ctx, mt.ingress, ingressPatch)).To(Succeed())

	// Patch the module status to simulate a lower version, triggering upgrade.
	g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mt.module), mt.module)).To(Succeed())

	statusPatch := client.MergeFrom(mt.module.DeepCopy())
	mt.module.Status.Module.Version = "0.0.0-0"
	g.Expect(k8sClient.Status().Patch(ctx, mt.module, statusPatch)).To(Succeed())

	// Trigger a reconcile.
	g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mt.module), mt.module)).To(Succeed())

	annPatch := client.MergeFrom(mt.module.DeepCopy())
	resources.SetAnnotation(mt.module, "test-trigger", time.Now().String())
	g.Expect(k8sClient.Patch(ctx, mt.module, annPatch)).To(Succeed())

	// Give the controller time to attempt the upgrade. The upgrade will
	// fail because of the fault annotation, so the status version should
	// remain at "0.0.1" (the patched value) rather than being updated
	// to the current operator version.
	g.Consistently(k.Get(mt.module)).WithContext(ctx).WithTimeout(10 * time.Second).WithPolling(interval).Should(
		jq.Match(`.status.module.version == "0.0.0-0"`),
	)

	// Clean up: remove the fault annotation so subsequent tests are not affected.
	g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(mt.ingress), mt.ingress)).To(Succeed())

	cleanPatch := client.MergeFrom(mt.ingress.DeepCopy())
	ann := mt.ingress.GetAnnotations()
	delete(ann, mymodule.AnnotationInjectUpgradeFault)
	mt.ingress.SetAnnotations(ann)
	g.Expect(k8sClient.Patch(ctx, mt.ingress, cleanPatch)).To(Succeed())

	// After removing the fault, the upgrade should succeed and version should update.
	g.Eventually(k.Get(mt.module)).WithContext(ctx).WithTimeout(timeout).WithPolling(interval).Should(
		jq.Match(`.status.module.version == "%s"`, versionBefore),
	)
}
