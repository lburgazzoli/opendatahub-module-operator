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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"

	k8sm "github.com/lburgazzoli/gomega-matchers/pkg/matchers/k8s"
	"github.com/lburgazzoli/gomega-matchers/pkg/matchers/jq"

	componentsv1alpha1 "github.com/lburgazzoli/opendatahub-module-operator/api/components/v1alpha1"
	"github.com/lburgazzoli/opendatahub-module-operator/internal/controller/mymodule"
	moduleconfig "github.com/lburgazzoli/opendatahub-module-operator/pkg/config"
)

const (
	operatorNamespace = "opendatahub-module-operator-system"
	timeout           = 2 * time.Minute
	interval          = 2 * time.Second

	labelPartOf            = "platform.opendatahub.io/part-of"
	annotationInstanceName = "platform.opendatahub.io/instance.name"
	annotationInstanceUID  = "platform.opendatahub.io/instance.uid"
	annotationType         = "platform.opendatahub.io/type"
	annotationVersion      = "platform.opendatahub.io/version"

	operatorConfigMapName = "opendatahub-module-operator-config"
)

var (
	ctx       context.Context
	cancel    context.CancelFunc
	k8sClient client.Client
	k         *k8sm.Matcher

	testScheme = runtime.NewScheme()
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(testScheme))
	utilruntime.Must(componentsv1alpha1.AddToScheme(testScheme))
}

func TestMain(m *testing.M) {
	ctx, cancel = context.WithCancel(context.Background())
	defer cancel()

	cfg, err := config.GetConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to get kubeconfig: %v\n", err)
		os.Exit(1)
	}

	k8sClient, err = client.New(cfg, client.Options{Scheme: testScheme})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create client: %v\n", err)
		os.Exit(1)
	}

	k = k8sm.New(k8sClient, testScheme)

	os.Exit(m.Run())
}

// myModuleE2ETest holds shared test fixtures for the MyModule e2e tests.
type myModuleE2ETest struct {
	module          *componentsv1alpha1.MyModule
	ingress         *networkingv1.Ingress
	operatorDeploy  *appsv1.Deployment
	operatorCfgMap  *corev1.ConfigMap
	workloadDeploy  *appsv1.Deployment
	workloadService *corev1.Service
}

func TestMyModule(t *testing.T) {
	mt := &myModuleE2ETest{
		module: &componentsv1alpha1.MyModule{
			ObjectMeta: metav1.ObjectMeta{
				Name: componentsv1alpha1.MyModuleInstanceName,
			},
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
				Name:      "opendatahub-module-operator-controller-manager",
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

	t.Cleanup(func() {
		_ = k8sClient.Delete(ctx, mt.module)
		_ = k8sClient.Delete(ctx, mt.ingress)
	})

	t.Run("operator should be running", mt.testOperatorRunning)
	t.Run("should have ConfigMap volume mounted", mt.testConfigMapVolume)
	t.Run("should have operator ConfigMap deployed", mt.testOperatorConfigMap)
	t.Run("should block when Ingress is missing", mt.testIngressBlocks)
	t.Run("should recover when Ingress is created", mt.testIngressRecovers)
	t.Run("should expose config values", mt.testConfigValues)
	t.Run("should report module version and platform", mt.testModuleStatus)
	t.Run("should set platform labels and annotations", mt.testPlatformLabels)
	t.Run("should have webhook-injected labels", mt.testWebhookLabels)
	t.Run("should set owner references", mt.testOwnerReferences)
}

func (mt *myModuleE2ETest) testOperatorRunning(t *testing.T) {
	g := NewWithT(t)

	g.Eventually(k.Get(mt.operatorDeploy)).WithContext(ctx).WithTimeout(timeout).WithPolling(interval).Should(
		jq.Match(`.status.readyReplicas >= 1`),
	)
}

func (mt *myModuleE2ETest) testConfigMapVolume(t *testing.T) {
	g := NewWithT(t)

	g.Eventually(k.Get(mt.operatorDeploy)).WithContext(ctx).WithTimeout(timeout).WithPolling(interval).Should(
		jq.Match(`.spec.template.spec.volumes[] | select(.name == "config") | .configMap.name != ""`),
	)

	g.Eventually(k.Get(mt.operatorDeploy)).WithContext(ctx).WithTimeout(timeout).WithPolling(interval).Should(
		jq.Match(`.spec.template.spec.containers[0].volumeMounts[] | select(.name == "config") | .mountPath == "/etc/controller/config"`),
	)

	g.Eventually(k.Get(mt.operatorDeploy)).WithContext(ctx).WithTimeout(timeout).WithPolling(interval).Should(
		jq.Match(`.spec.template.spec.containers[0].env[] | select(.name == "ODH_MODULE_OPERATOR_CONFIGURATION_PATH") | .value == "/etc/controller/config"`),
	)
}

func (mt *myModuleE2ETest) testOperatorConfigMap(t *testing.T) {
	g := NewWithT(t)

	g.Eventually(k.Get(mt.operatorCfgMap)).WithContext(ctx).WithTimeout(timeout).WithPolling(interval).Should(And(
		jq.Match(`.data."%s" != ""`, moduleconfig.KeyPlatformType),
		jq.Match(`.data."%s" != ""`, moduleconfig.KeyPlatformVersion),
	))
}

func (mt *myModuleE2ETest) testIngressBlocks(t *testing.T) {
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

func (mt *myModuleE2ETest) testIngressRecovers(t *testing.T) {
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

func (mt *myModuleE2ETest) testConfigValues(t *testing.T) {
	g := NewWithT(t)

	g.Eventually(k.Get(mt.module)).WithContext(ctx).WithTimeout(timeout).WithPolling(interval).Should(And(
		jq.Match(`.status.configValues."%s" != ""`, moduleconfig.KeyPlatformType),
		jq.Match(`.status.configValues."%s" != ""`, moduleconfig.KeyPlatformVersion),
	))
}

func (mt *myModuleE2ETest) testModuleStatus(t *testing.T) {
	g := NewWithT(t)

	g.Eventually(k.Get(mt.module)).WithContext(ctx).WithTimeout(timeout).WithPolling(interval).Should(And(
		jq.Match(`.status.module.version != ""`),
		jq.Match(`.status.module.platform.name != ""`),
		jq.Match(`.status.module.sources | length > 0`),
		jq.Match(`.status.module.sources[0].renderer == "kustomize"`),
	))
}

func (mt *myModuleE2ETest) testPlatformLabels(t *testing.T) {
	g := NewWithT(t)

	g.Eventually(k.Get(mt.workloadDeploy)).WithContext(ctx).WithTimeout(timeout).WithPolling(interval).Should(And(
		jq.Match(`.metadata.labels."%s" == "mymodule"`, labelPartOf),
		jq.Match(`.metadata.annotations."%s" != ""`, annotationInstanceName),
		jq.Match(`.metadata.annotations."%s" != ""`, annotationInstanceUID),
		jq.Match(`.metadata.annotations."%s" != ""`, annotationType),
		jq.Match(`.metadata.annotations."%s" != ""`, annotationVersion),
	))

	g.Eventually(k.Get(mt.workloadService)).WithContext(ctx).WithTimeout(timeout).WithPolling(interval).Should(And(
		jq.Match(`.metadata.labels."%s" == "mymodule"`, labelPartOf),
		jq.Match(`.metadata.annotations."%s" != ""`, annotationType),
		jq.Match(`.metadata.annotations."%s" != ""`, annotationVersion),
	))
}

func (mt *myModuleE2ETest) testWebhookLabels(t *testing.T) {
	g := NewWithT(t)

	g.Eventually(k.Get(mt.workloadDeploy)).WithContext(ctx).WithTimeout(timeout).WithPolling(interval).Should(And(
		jq.Match(`.metadata.labels."mymodule.opendatahub.io/version" != ""`),
		jq.Match(`.metadata.labels."mymodule.opendatahub.io/platform" != ""`),
	))
}

func (mt *myModuleE2ETest) testOwnerReferences(t *testing.T) {
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
