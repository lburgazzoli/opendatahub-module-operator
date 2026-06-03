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

	"github.com/lburgazzoli/gomega-matchers/pkg/matchers/jq"
	k8sm "github.com/lburgazzoli/gomega-matchers/pkg/matchers/k8s"
	componentsv1alpha1 "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-datasciencepipelines-operator/api/components/v1alpha1"
	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-datasciencepipelines-operator/test/support"
	. "github.com/onsi/gomega"
	"github.com/opendatahub-io/opendatahub-operator/v2/pkg/metadata/labels"
	promv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	k8serr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
)

const (
	timeout  = 90 * time.Second
	interval = 2 * time.Second

	labelPartOf            = "platform.opendatahub.io/part-of"
	annotationInstanceName = "platform.opendatahub.io/instance.name"
	annotationInstanceUID  = "platform.opendatahub.io/instance.uid"
	annotationType         = "platform.opendatahub.io/type"
	annotationVersion      = "platform.opendatahub.io/version"

	operatorConfigMapName  = "opendatahub-datasciencepipelines-config"
	moduleCRDName          = "datasciencepipelines.components.platform.opendatahub.io"
	argoWorkflowCRDName    = "workflows.argoproj.io"
	testManagedByLabel     = "testing.opendatahub.io/managed-by"
	testManagedByValue     = "dsp-e2e"
	legacyComponentName    = "data-science-pipelines-operator"
	operatorDeploymentName = "opendatahub-datasciencepipelines-operator"
	workloadDeploymentName = "data-science-pipelines-operator-controller-manager"
	workloadServiceMonName = "data-science-pipelines-operator-service-monitor"
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

type dspE2ETest struct {
	module             *componentsv1alpha1.DataSciencePipelines
	moduleCRD          *apiextensionsv1.CustomResourceDefinition
	operatorDeploy     *appsv1.Deployment
	operatorCfgMap     *corev1.ConfigMap
	workloadDeploy     *appsv1.Deployment
	workloadServiceMon *promv1.ServiceMonitor
}

func TestDataSciencePipelines(t *testing.T) {
	operatorNamespace := support.OperatorNamespace()

	suite := &dspE2ETest{
		module: &componentsv1alpha1.DataSciencePipelines{
			ObjectMeta: metav1.ObjectMeta{
				Name: componentsv1alpha1.DataSciencePipelinesInstanceName,
			},
		},
		moduleCRD: &apiextensionsv1.CustomResourceDefinition{
			ObjectMeta: metav1.ObjectMeta{Name: moduleCRDName},
		},
		operatorDeploy: &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      operatorDeploymentName,
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
				Name:      workloadDeploymentName,
				Namespace: operatorNamespace,
			},
		},
		workloadServiceMon: &promv1.ServiceMonitor{
			ObjectMeta: metav1.ObjectMeta{
				Name:      workloadServiceMonName,
				Namespace: operatorNamespace,
			},
		},
	}
	foundation := &foundationTests{dspE2ETest: suite}

	cleanupModule(t, suite.module)
	t.Cleanup(func() { cleanupModule(t, suite.module) })

	eventuallyDeploymentReady(t, suite.operatorDeploy)

	t.Run("foundation", foundation.Execute)
}

func cleanupModule(t *testing.T, module *componentsv1alpha1.DataSciencePipelines) {
	t.Helper()

	_ = k8sClient.Delete(ctx, module)
	waitForSingletonDeleted(t, module)
}

func withStoppedOperator(t *testing.T, run func()) {
	t.Helper()

	operatorNamespace := support.OperatorNamespace()
	key := client.ObjectKey{
		Name:      operatorDeploymentName,
		Namespace: operatorNamespace,
	}

	deployment := &appsv1.Deployment{}
	if err := k8sClient.Get(ctx, key, deployment); err != nil {
		t.Fatalf("getting operator deployment: %v", err)
	}

	originalReplicas := int32(1)
	if deployment.Spec.Replicas != nil {
		originalReplicas = *deployment.Spec.Replicas
	}

	setOperatorReplicas(t, key, 0)
	defer setOperatorReplicas(t, key, originalReplicas)

	run()
}

func setOperatorReplicas(t *testing.T, key client.ObjectKey, replicas int32) {
	t.Helper()

	g := NewWithT(t)
	g.Eventually(func() error {
		deployment := &appsv1.Deployment{}
		if err := k8sClient.Get(ctx, key, deployment); err != nil {
			return err
		}

		if deployment.Spec.Replicas != nil && *deployment.Spec.Replicas == replicas {
			return nil
		}

		deployment.Spec.Replicas = &replicas
		return k8sClient.Update(ctx, deployment)
	}).WithContext(ctx).WithTimeout(timeout).WithPolling(interval).Should(Succeed())

	if replicas == 0 {
		g.Eventually(func(g Gomega) {
			deployment := &appsv1.Deployment{}
			err := k8sClient.Get(ctx, key, deployment)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(deployment.Status.Replicas).To(BeZero())
			g.Expect(deployment.Status.ReadyReplicas).To(BeZero())
		}).WithContext(ctx).WithTimeout(timeout).WithPolling(interval).Should(Succeed())

		return
	}

	g.Eventually(func(g Gomega) {
		deployment := &appsv1.Deployment{}
		err := k8sClient.Get(ctx, key, deployment)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(deployment.Status.ReadyReplicas).To(BeNumerically(">=", replicas))
	}).WithContext(ctx).WithTimeout(timeout).WithPolling(interval).Should(Succeed())
}

func manageArgoWorkflowCRD(t *testing.T) {
	t.Helper()

	state, err := support.CaptureWorkflowCRDState(ctx, k8sClient, argoWorkflowCRDName)
	if err != nil {
		t.Fatalf("capturing workflows CRD state: %v", err)
	}

	t.Cleanup(func() {
		if err := support.RestoreWorkflowCRDState(
			ctx,
			k8sClient,
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
	err := k8sClient.Get(ctx, client.ObjectKeyFromObject(crd), crd)
	if k8serr.IsNotFound(err) {
		return
	}
	if err != nil {
		t.Fatalf("getting workflows CRD: %v", err)
	}
	if err := k8sClient.Delete(ctx, crd); err != nil && !k8serr.IsNotFound(err) {
		t.Fatalf("deleting workflows CRD: %v", err)
	}
	waitForDeleted(t, crd)
}

func ensureArgoWorkflowCRDOwnedByODH(t *testing.T) {
	t.Helper()
	manageArgoWorkflowCRD(t)

	crd := loadOrCreateWorkflowCRD(t)
	odhLabel := labels.ODH.Component(legacyComponentName)
	if crd.Labels[odhLabel] == labels.True {
		return
	}
	updateWorkflowCRDEventually(t, func(crd *apiextensionsv1.CustomResourceDefinition) {
		crd.Labels[odhLabel] = labels.True
		crd.Labels[testManagedByLabel] = testManagedByValue
	})
}

func ensureArgoWorkflowCRDForeignOwned(t *testing.T) {
	t.Helper()
	manageArgoWorkflowCRD(t)

	crd := loadOrCreateWorkflowCRD(t)
	odhLabel := labels.ODH.Component(legacyComponentName)
	if crd.Labels[odhLabel] != labels.True && crd.Labels[testManagedByLabel] == testManagedByValue {
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
	err := k8sClient.Get(ctx, client.ObjectKeyFromObject(crd), crd)
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
	if err := support.InstallCRDFile(ctx, k8sClient, crdPath); err != nil {
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
		if err := k8sClient.Get(ctx, client.ObjectKey{Name: argoWorkflowCRDName}, latest); err != nil {
			return err
		}
		if latest.Labels == nil {
			latest.Labels = map[string]string{}
		}
		mutate(latest)
		return k8sClient.Update(ctx, latest)
	}).WithContext(ctx).WithTimeout(timeout).WithPolling(interval).Should(Succeed())

	current := &apiextensionsv1.CustomResourceDefinition{}
	if err := k8sClient.Get(ctx, client.ObjectKey{Name: argoWorkflowCRDName}, current); err != nil {
		t.Fatalf("reloading workflows CRD after update: %v", err)
	}

	return current
}

func createModule(t *testing.T, module *componentsv1alpha1.DataSciencePipelines) {
	t.Helper()

	if module.Annotations == nil {
		module.Annotations = map[string]string{}
	}
	module.Annotations["testing.opendatahub.io/reconcile-at"] = time.Now().UTC().Format(time.RFC3339Nano)

	current := &componentsv1alpha1.DataSciencePipelines{}
	err := k8sClient.Get(ctx, client.ObjectKeyFromObject(module), current)
	if k8serr.IsNotFound(err) {
		if err := k8sClient.Create(ctx, module); err != nil {
			t.Fatalf("creating module: %v", err)
		}

		return
	}
	if err != nil {
		t.Fatalf("getting module before upsert: %v", err)
	}

	current.Spec = module.Spec
	current.Annotations = module.Annotations
	if err := k8sClient.Update(ctx, current); err != nil {
		t.Fatalf("updating module: %v", err)
	}
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
