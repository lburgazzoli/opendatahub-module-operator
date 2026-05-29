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

	componentsv1alpha1 "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-trustyai-operator/api/components/v1alpha1"
	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-trustyai-operator/test/support"
)

const (
	timeout  = 90 * time.Second
	interval = 2 * time.Second

	labelPartOf            = "platform.opendatahub.io/part-of"
	annotationInstanceName = "platform.opendatahub.io/instance.name"
	annotationInstanceUID  = "platform.opendatahub.io/instance.uid"
	annotationType         = "platform.opendatahub.io/type"
	annotationVersion      = "platform.opendatahub.io/version"

	operatorConfigMapName = "opendatahub-trustyai-config"
	moduleCRDName         = "trustyais.components.platform.opendatahub.io"
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

type trustyAIE2ETest struct {
	module            *componentsv1alpha1.TrustyAI
	operatorNamespace string
	manageKserveCR    bool
}

func TestTrustyAI(t *testing.T) {
	suite := &trustyAIE2ETest{
		module: &componentsv1alpha1.TrustyAI{
			ObjectMeta: metav1.ObjectMeta{
				Name: componentsv1alpha1.TrustyAIInstanceName,
			},
		},
		operatorNamespace: support.OperatorNamespace(),
	}
	foundation := newFoundationTests(suite)

	// Clean up any leftover CR from a previous run.
	_ = k8sClient.Delete(ctx, suite.module)
	waitForSingletonDeleted(t, suite.module)

	t.Cleanup(func() {
		_ = k8sClient.Delete(ctx, suite.module)
	})

	// Gate: operator must be running before tests proceed.
	eventuallyDeploymentReady(t, foundation.operatorDeploy)

	preconditions, err := support.EnsureTrustyAIPreconditionsIfMissing(ctx, k8sClient)
	NewWithT(t).Expect(err).NotTo(HaveOccurred())
	suite.manageKserveCR = preconditions.ManageKserveCR

	t.Run("foundation", foundation.Execute)
}

// testPreconditionCRMissing verifies that the TrustyAI CR reports ProvisioningSucceeded=False
// when the Kserve CR (a required precondition) is absent.
func (rt *trustyAIE2ETest) testPreconditionCRMissing(t *testing.T) {
	if !rt.manageKserveCR {
		t.Skip("Kserve CR already exists on cluster; skipping destructive absence test")
	}

	g := NewWithT(t)

	kserveCR := support.NewStubKserveCR()
	g.Expect(k8sClient.Delete(ctx, kserveCR)).To(Succeed())
	waitForDeleted(t, kserveCR)
	t.Cleanup(func() {
		_, _ = support.EnsureStubKserveCRIfMissing(ctx, k8sClient)
	})

	rt.module.ResourceVersion = ""
	g.Expect(k8sClient.Create(ctx, rt.module)).To(Succeed())
	t.Cleanup(func() {
		_ = k8sClient.Delete(ctx, rt.module)
		waitForSingletonDeleted(t, rt.module)
	})

	g.Eventually(k.Get(rt.module)).WithContext(ctx).WithTimeout(timeout).WithPolling(interval).Should(
		jq.Match(`(.status.conditions // []) | any(.[]; .type == "ProvisioningSucceeded" and .status == "False")`),
	)
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
