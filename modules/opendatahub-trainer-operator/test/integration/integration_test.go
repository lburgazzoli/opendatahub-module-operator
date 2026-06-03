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
	imagev1 "github.com/openshift/api/image/v1"
	promv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	"github.com/spf13/viper"
	admissionv1 "k8s.io/api/admissionregistration/v1"

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
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/lburgazzoli/gomega-matchers/pkg/matchers/jq"
	k8sm "github.com/lburgazzoli/gomega-matchers/pkg/matchers/k8s"

	componentsv1alpha1 "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-trainer-operator/api/components/v1alpha1"
	moduleconfig "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-trainer-operator/pkg/config"
	modulemanager "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-trainer-operator/pkg/manager"
	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-trainer-operator/test/support"
	"github.com/opendatahub-io/opendatahub-operator/v2/pkg/cluster"
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
	ctx                    context.Context
	cancel                 context.CancelFunc
	k8sClient              client.Client
	directClient           client.Client
	k                      *k8sm.Matcher
	operatorCfgData        map[string]string
	operatorReleaseVersion string
	testScheme             = runtime.NewScheme()
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

	if err := support.EnsureTrainerPreconditions(ctx, directClient); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to install Trainer preconditions: %v\n", err)
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
		PlatformName:          operatorCfgData[moduleconfig.KeyPlatformName],
		PlatformVersion:       operatorCfgData[moduleconfig.KeyPlatformVersion],
		ApplicationsNamespace: testNamespace,
		ManifestsPath:         support.MustProjectFile("config", "manifests"),
		MetricsAddr:           "0",
		HealthProbeAddr:       "0",
		PprofAddr:             "",
		LeaderElect:           false,
	}
	operatorReleaseVersion = moduleCfg.Release().Version.String()

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

	suite := &trainerTest{
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
	foundation := &foundationTests{trainerTest: suite}

	// Clean up any leftover CR from a previous run before starting.
	_ = k8sClient.Delete(ctx, suite.module)
	waitForSingletonDeleted(t, suite.module)

	t.Cleanup(func() {
		_ = k8sClient.Delete(ctx, suite.module)
	})

	t.Run("foundation", foundation.Execute)
}

func waitForDeleted(t *testing.T, obj client.Object) {
	t.Helper()

	g := NewWithT(t)
	g.Eventually(func(g Gomega) {
		fresh := obj.DeepCopyObject().(client.Object)
		err := directClient.Get(ctx, client.ObjectKeyFromObject(obj), fresh)
		g.Expect(k8serr.IsNotFound(err)).To(BeTrue())
	}).WithContext(ctx).WithTimeout(timeout).WithPolling(interval).Should(Succeed())
}

func waitForSingletonDeleted(t *testing.T, obj client.Object) {
	t.Helper()

	waitForDeleted(t, obj)
	obj.SetResourceVersion("")
	obj.SetUID("")
}

func eventuallyDeploymentReady(t *testing.T, deploy *appsv1.Deployment) {
	t.Helper()

	g := NewWithT(t)
	g.Eventually(k.Get(deploy)).WithContext(ctx).WithTimeout(timeout).WithPolling(interval).Should(
		jq.Match(`.status.readyReplicas >= 1`),
	)
}
