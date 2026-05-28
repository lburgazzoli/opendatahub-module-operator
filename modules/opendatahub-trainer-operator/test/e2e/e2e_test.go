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

	componentsv1alpha1 "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-trainer-operator/api/components/v1alpha1"
	moduleconfig "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-trainer-operator/pkg/config"
	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-trainer-operator/pkg/version"
	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-trainer-operator/test/support"
	"github.com/opendatahub-io/opendatahub-operator/v2/pkg/controller/status"
)

const (
	timeout  = 3 * time.Minute
	interval = 2 * time.Second

	labelPartOf            = "platform.opendatahub.io/part-of"
	annotationInstanceName = "platform.opendatahub.io/instance.name"
	annotationInstanceUID  = "platform.opendatahub.io/instance.uid"
	annotationType         = "platform.opendatahub.io/type"
	annotationVersion      = "platform.opendatahub.io/version"

	operatorConfigMapName = "opendatahub-trainer-config"
	moduleCRDName         = "trainers.components.platform.opendatahub.io"
	jobSetOperatorCRDName = "jobsetoperators.operator.openshift.io"
	jobSetOperatorCRName  = "cluster"
	jobSetCRDName         = "jobsets.jobset.x-k8s.io"
)

var (
	ctx                     context.Context
	cancel                  context.CancelFunc
	k8sClient               client.Client
	k                       *k8sm.Matcher
	manageJobSetOperatorCRD bool
	manageJobSetOperatorCR  bool
	manageJobSetCRD         bool

	testScheme = runtime.NewScheme()
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

	preconditions, err := support.EnsureTrainerPreconditionsIfMissing(ctx, k8sClient)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to ensure Trainer preconditions: %v\n", err)
		return 1
	}
	manageJobSetOperatorCRD = preconditions.ManageJobSetOperatorCRD
	manageJobSetOperatorCR = preconditions.ManageJobSetOperatorCR
	manageJobSetCRD = preconditions.ManageJobSetCRD

	k = k8sm.New(k8sClient, testScheme)

	return m.Run()
}

type trainerE2ETest struct {
	module         *componentsv1alpha1.Trainer
	moduleCRD      *apiextensionsv1.CustomResourceDefinition
	operatorDeploy *appsv1.Deployment
	operatorCfgMap *corev1.ConfigMap
	workloadDeploy *appsv1.Deployment
}

func TestTrainer(t *testing.T) {
	operatorNamespace := support.OperatorNamespace()

	rt := &trainerE2ETest{
		module: &componentsv1alpha1.Trainer{
			ObjectMeta: metav1.ObjectMeta{
				Name: componentsv1alpha1.TrainerInstanceName,
			},
		},
		moduleCRD: &apiextensionsv1.CustomResourceDefinition{
			ObjectMeta: metav1.ObjectMeta{Name: moduleCRDName},
		},
		operatorDeploy: &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "opendatahub-trainer-operator",
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
				Name:      "kubeflow-trainer-controller-manager",
				Namespace: operatorNamespace,
			},
		},
	}

	// Clean up any leftover CR from a previous run.
	_ = k8sClient.Delete(ctx, rt.module)
	waitForSingletonDeleted(t, rt.module)

	t.Cleanup(func() {
		_ = k8sClient.Delete(ctx, rt.module)
	})

	// Gate: if the operator is not running, fail immediately — don't
	// let subsequent tests hang waiting for resources that won't appear.
	eventuallyDeploymentReady(t, rt.operatorDeploy)

	t.Run("should have module CRD installed", rt.testModuleCRDInstalled)
	t.Run("should have operator ConfigMap deployed", rt.testOperatorConfigMap)
	t.Run("should report not ready when JobSet operator CRD is missing", rt.testJobSetOperatorCRDMissing)
	t.Run("should report not ready when JobSet operator CR is missing", rt.testJobSetOperatorCRMissing)
	t.Run("should report not ready when JobSet CRD is missing", rt.testJobSetCRDMissing)
	t.Run("should become ready", rt.testBecomesReady)
	t.Run("should report module version and platform", rt.testModuleStatus)
	t.Run("should set platform labels and annotations", rt.testPlatformLabels)
	t.Run("should set owner references", rt.testOwnerReferences)
}

func (rt *trainerE2ETest) testModuleCRDInstalled(t *testing.T) {
	g := NewWithT(t)

	g.Eventually(k.Get(rt.moduleCRD)).WithContext(ctx).WithTimeout(timeout).WithPolling(interval).Should(
		jq.Match(`.metadata.name == "%s"`, moduleCRDName),
	)
}

func (rt *trainerE2ETest) expectDependenciesUnavailable(
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

func (rt *trainerE2ETest) testJobSetOperatorCRDMissing(t *testing.T) {
	if !manageJobSetOperatorCRD {
		t.Skip("JobSetOperator CRD already exists on cluster; skipping destructive absence test")
	}

	g := NewWithT(t)

	jobSetOperatorCRD := &apiextensionsv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: jobSetOperatorCRDName},
	}
	g.Expect(k8sClient.Delete(ctx, jobSetOperatorCRD)).To(Succeed())
	waitForDeleted(t, jobSetOperatorCRD)
	t.Cleanup(func() {
		_, _ = support.EnsureStubCRDIfMissing(
			ctx,
			k8sClient,
			jobSetOperatorCRDName,
			"operator.openshift.io",
			"v1",
			"JobSetOperator",
			"jobsetoperators",
		)
		_, _ = support.EnsureStubJobSetOperatorCRIfMissing(ctx, k8sClient)
		_ = k8sClient.Delete(ctx, rt.module)
		waitForSingletonDeleted(t, rt.module)
	})

	rt.module.ResourceVersion = ""
	g.Expect(k8sClient.Create(ctx, rt.module)).To(Succeed())
	rt.expectDependenciesUnavailable(t, status.JobSetOperatorNotInstalledMessage)
}

func (rt *trainerE2ETest) testJobSetOperatorCRMissing(t *testing.T) {
	if !manageJobSetOperatorCR {
		t.Skip("JobSetOperator CR already exists on cluster; skipping destructive absence test")
	}

	g := NewWithT(t)

	jobSetOperatorCR := support.NewStubJobSetOperatorCR()
	g.Expect(k8sClient.Delete(ctx, jobSetOperatorCR)).To(Succeed())
	waitForDeleted(t, jobSetOperatorCR)
	t.Cleanup(func() {
		_, _ = support.EnsureStubJobSetOperatorCRIfMissing(ctx, k8sClient)
		_ = k8sClient.Delete(ctx, rt.module)
		waitForSingletonDeleted(t, rt.module)
	})

	rt.module.ResourceVersion = ""
	g.Expect(k8sClient.Create(ctx, rt.module)).To(Succeed())
	rt.expectDependenciesUnavailable(t, status.JobSetOperatorCRNotFoundMessage)
}

func (rt *trainerE2ETest) testJobSetCRDMissing(t *testing.T) {
	if !manageJobSetCRD {
		t.Skip("JobSet CRD already exists on cluster; skipping destructive absence test")
	}

	g := NewWithT(t)

	jobSetCRD := &apiextensionsv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: jobSetCRDName},
	}
	g.Expect(k8sClient.Delete(ctx, jobSetCRD)).To(Succeed())
	waitForDeleted(t, jobSetCRD)
	t.Cleanup(func() {
		_, _ = support.EnsureStubCRDIfMissing(
			ctx,
			k8sClient,
			jobSetCRDName,
			"jobset.x-k8s.io",
			"v1alpha2",
			"JobSet",
			"jobsets",
		)
		_ = k8sClient.Delete(ctx, rt.module)
		waitForSingletonDeleted(t, rt.module)
	})

	rt.module.ResourceVersion = ""
	g.Expect(k8sClient.Create(ctx, rt.module)).To(Succeed())
	rt.expectDependenciesUnavailable(t, status.JobSetCRDMissingMessage)
}

func (rt *trainerE2ETest) testOperatorConfigMap(t *testing.T) {
	g := NewWithT(t)

	g.Eventually(k.Get(rt.operatorCfgMap)).WithContext(ctx).WithTimeout(timeout).WithPolling(interval).Should(And(
		jq.Match(`.data."%s" != ""`, moduleconfig.KeyPlatformType),
		jq.Match(`.data."%s" != ""`, moduleconfig.KeyPlatformVersion),
	))
}

func (rt *trainerE2ETest) testBecomesReady(t *testing.T) {
	g := NewWithT(t)

	rt.module.ResourceVersion = ""
	g.Expect(k8sClient.Create(ctx, rt.module)).To(Succeed())

	g.Eventually(k.Get(rt.module)).WithContext(ctx).WithTimeout(timeout).WithPolling(interval).Should(And(
		jq.Match(`.status.phase == "Ready"`),
		jq.Match(`.status.conditions[] | select(.type == "Ready") | .status == "True"`),
		jq.Match(`.status.conditions[] | select(.type == "ProvisioningSucceeded") | .status == "True"`),
	))

	eventuallyDeploymentReady(t, rt.workloadDeploy)
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

func (rt *trainerE2ETest) testModuleStatus(t *testing.T) {
	g := NewWithT(t)
	operatorCfg := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      operatorConfigMapName,
			Namespace: support.OperatorNamespace(),
		},
	}
	workloadDeploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kubeflow-trainer-controller-manager",
			Namespace: support.OperatorNamespace(),
		},
	}

	g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(operatorCfg), operatorCfg)).To(Succeed())
	g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(workloadDeploy), workloadDeploy)).To(Succeed())

	platformType := operatorCfg.Data[moduleconfig.KeyPlatformType]
	workloadVersion := workloadDeploy.Annotations[annotationVersion]

	g.Eventually(k.Get(rt.module)).WithContext(ctx).WithTimeout(timeout).WithPolling(interval).Should(And(
		jq.Match(`.status.module.version == "%s"`, version.Version),
		jq.Match(`.status.module.buildSource == "%s"`,
			version.BuildSource()),
		jq.Match(`.status.module.platform.name == "%s"`, platformType),
		jq.Match(`.status.module.platform.version == "%s"`, workloadVersion),
		jq.Match(`.status.module.sources | length > 0`),
		jq.Match(`.status.module.sources[0].path != ""`),
		jq.Match(`.status.module.sources[0].renderer == "kustomize"`),
	))
}

func (rt *trainerE2ETest) testPlatformLabels(t *testing.T) {
	g := NewWithT(t)
	module := rt.module.DeepCopy()
	operatorCfg := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      operatorConfigMapName,
			Namespace: support.OperatorNamespace(),
		},
	}

	g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(module), module)).To(Succeed())
	g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(operatorCfg), operatorCfg)).To(Succeed())

	g.Eventually(k.Get(rt.workloadDeploy)).WithContext(ctx).WithTimeout(timeout).WithPolling(interval).Should(And(
		jq.Match(`.metadata.labels."%s" == "trainer"`, labelPartOf),
		jq.Match(`.metadata.annotations."%s" == "%s"`,
			annotationInstanceName,
			module.GetName()),
		jq.Match(`.metadata.annotations."%s" == "%s"`,
			annotationInstanceUID,
			string(module.GetUID())),
		jq.Match(`.metadata.annotations."%s" == "%s"`,
			annotationType,
			operatorCfg.Data[moduleconfig.KeyPlatformType]),
		jq.Match(`.metadata.annotations."%s" == "%s"`,
			annotationVersion,
			module.Status.Module.Platform.Version.String()),
	))
}

func (rt *trainerE2ETest) testOwnerReferences(t *testing.T) {
	g := NewWithT(t)

	g.Eventually(k.Get(rt.workloadDeploy)).WithContext(ctx).WithTimeout(timeout).WithPolling(interval).Should(
		jq.Match(`.metadata.ownerReferences[] | select(.kind == "Trainer") | .name == "%s"`,
			componentsv1alpha1.TrainerInstanceName),
	)
}
