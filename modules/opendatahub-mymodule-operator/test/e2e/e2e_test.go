//go:build e2e

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

package e2e

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	. "github.com/onsi/gomega"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	k8serr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"

	"github.com/lburgazzoli/gomega-matchers/pkg/matchers/jq"
	k8sm "github.com/lburgazzoli/gomega-matchers/pkg/matchers/k8s"

	componentsv1alpha1 "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-mymodule-operator/api/components/v1alpha1"
	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-mymodule-operator/internal/controller/mymodule"
)

const (
	defaultOperatorNamespace = "opendatahub-mymodule-system"
	timeout                  = 90 * time.Second
	interval                 = 2 * time.Second

	labelPartOf            = "platform.opendatahub.io/part-of"
	annotationInstanceName = "platform.opendatahub.io/instance.name"
	annotationInstanceUID  = "platform.opendatahub.io/instance.uid"
	annotationType         = "platform.opendatahub.io/type"
	annotationVersion      = "platform.opendatahub.io/version"

	operatorConfigMapName = "opendatahub-mymodule-config"
	moduleCRDName         = "mymodules.components.platform.opendatahub.io"
)

var (
	operatorNamespace = envOrDefault("OPERATOR_NAMESPACE", defaultOperatorNamespace)
	ctx               context.Context
	cancel            context.CancelFunc
	k8sClient         client.Client
	k                 *k8sm.Matcher

	testScheme = runtime.NewScheme()
)

func envOrDefault(key string, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}

	return defaultValue
}

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

	cfg, err := config.GetConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to get kubeconfig: %v\n", err)
		return 1
	}

	k8sClient, err = client.New(cfg, client.Options{Scheme: testScheme})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create client: %v\n", err)
		return 1
	}

	k = k8sm.New(k8sClient, testScheme)

	return m.Run()
}

// myModuleE2ETest holds shared test fixtures for the MyModule e2e tests.
type myModuleE2ETest struct {
	module          *componentsv1alpha1.MyModule
	moduleCRD       *apiextensionsv1.CustomResourceDefinition
	ingress         *networkingv1.Ingress
	operatorDeploy  *appsv1.Deployment
	operatorCfgMap  *corev1.ConfigMap
	workloadDeploy  *appsv1.Deployment
	workloadService *corev1.Service
}

func TestMyModule(t *testing.T) {
	suite := &myModuleE2ETest{
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
				Namespace: operatorNamespace,
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
		operatorDeploy: &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "opendatahub-mymodule-operator",
				Namespace: operatorNamespace,
			},
		},
		operatorCfgMap: &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      operatorConfigMapName,
				Namespace: operatorNamespace,
			},
		},
		workloadDeploy: &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "mymodule-workload",
				Namespace: operatorNamespace,
			},
		},
		workloadService: &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "mymodule-workload",
				Namespace: operatorNamespace,
			},
		},
	}
	foundation := &foundationTests{myModuleE2ETest: suite}

	// Clean up any leftover singleton objects from a previous run.
	_ = k8sClient.Delete(ctx, suite.module)
	_ = k8sClient.Delete(ctx, suite.ingress)
	waitForSingletonDeleted(t, suite.module)
	waitForSingletonDeleted(t, suite.ingress)

	t.Cleanup(func() {
		_ = k8sClient.Delete(ctx, suite.module)
		_ = k8sClient.Delete(ctx, suite.ingress)
	})

	eventuallyDeploymentReady(t, suite.operatorDeploy)

	t.Run("foundation", foundation.Execute)
}

func waitForDeleted(t *testing.T, obj client.Object) {
	t.Helper()

	g := NewWithT(t)
	g.Eventually(func(g Gomega) {
		fresh := obj.DeepCopyObject().(client.Object)
		err := k8sClient.Get(ctx, client.ObjectKeyFromObject(obj), fresh)
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
