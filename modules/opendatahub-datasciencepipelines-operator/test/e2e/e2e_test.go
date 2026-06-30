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
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/lburgazzoli/gomega-matchers/pkg/matchers/jq"
	k8sm "github.com/lburgazzoli/gomega-matchers/pkg/matchers/k8s"

	componentsv1alpha1 "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-datasciencepipelines-operator/api/components/v1alpha1"
	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-datasciencepipelines-operator/assets"
	dspcontroller "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-datasciencepipelines-operator/internal/controller/datasciencepipelines"
	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-datasciencepipelines-operator/test/support"
	"github.com/opendatahub-io/odh-platform-utilities/pkg/metadata/labels"
)

const (
	labelTrue          = "true"
	testManagedByLabel = "testing.opendatahub.io/managed-by"
	testManagedByValue = "dsp-e2e"

	operatorDeploymentName = "odh-datasciencepipelines-operator"
	workloadDeploymentName = "data-science-pipelines-operator-controller-manager"
	workloadServiceMonName = "data-science-pipelines-operator-service-monitor"
)

func TestMain(m *testing.M) {
	os.Exit(runTestMain(m))
}

func runTestMain(m *testing.M) int {
	cfg, err := support.LoadGomegaConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load test config: %v\n", err)
		return 1
	}

	SetDefaultEventuallyTimeout(cfg.EventuallyTimeout)
	SetDefaultEventuallyPollingInterval(cfg.EventuallyPollingInterval)
	SetDefaultConsistentlyPollingInterval(cfg.ConsistentlyPollingInterval)

	return m.Run()
}

func TestDataSciencePipelines(t *testing.T) {
	k8sClient, err := support.NewClient()
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	operatorDeploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      operatorDeploymentName,
			Namespace: support.OperatorNamespace(),
		},
	}
	NewWithT(t).Eventually(t.Context(), k8sm.Get(k8sClient, operatorDeploy)).Should(
		jq.Match(`.status.readyReplicas >= 1`),
	)

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

	if err := support.ApplyManifestFromFS(
		ctx,
		cli,
		assets.Manifests,
		"manifests/datasciencepipelines/argo/crd.workflows.yaml",
	); err != nil {
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

func withStoppedOperator(t *testing.T, cli client.Client, run func()) {
	t.Helper()
	ctx := t.Context()

	key := client.ObjectKey{
		Name:      operatorDeploymentName,
		Namespace: support.OperatorNamespace(),
	}

	deployment := &appsv1.Deployment{}
	if err := cli.Get(ctx, key, deployment); err != nil {
		t.Fatalf("getting operator deployment: %v", err)
	}

	originalReplicas := int32(1)
	if deployment.Spec.Replicas != nil {
		originalReplicas = *deployment.Spec.Replicas
	}

	setOperatorReplicas(t, cli, key, 0)
	defer setOperatorReplicas(t, cli, key, originalReplicas)

	run()
}

func setOperatorReplicas(t *testing.T, cli client.Client, key client.ObjectKey, replicas int32) {
	t.Helper()
	ctx := t.Context()

	g := NewWithT(t)
	g.Eventually(func() error {
		deployment := &appsv1.Deployment{}
		if err := cli.Get(ctx, key, deployment); err != nil {
			return err
		}

		if deployment.Spec.Replicas != nil && *deployment.Spec.Replicas == replicas {
			return nil
		}

		deployment.Spec.Replicas = &replicas
		return cli.Update(ctx, deployment)
	}).WithContext(ctx).Should(Succeed())

	if replicas == 0 {
		g.Eventually(func(g Gomega) {
			deployment := &appsv1.Deployment{}
			err := cli.Get(ctx, key, deployment)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(deployment.Status.Replicas).To(BeZero())
			g.Expect(deployment.Status.ReadyReplicas).To(BeZero())
		}).WithContext(ctx).Should(Succeed())

		return
	}

	g.Eventually(func(g Gomega) {
		deployment := &appsv1.Deployment{}
		err := cli.Get(ctx, key, deployment)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(deployment.Status.ReadyReplicas).To(BeNumerically(">=", replicas))
	}).WithContext(ctx).Should(Succeed())
}

func createModule(t *testing.T, cli client.Client, module *componentsv1alpha1.DataSciencePipelines) {
	t.Helper()

	if module.Annotations == nil {
		module.Annotations = map[string]string{}
	}
	module.Annotations["testing.opendatahub.io/reconcile-at"] = time.Now().UTC().Format(time.RFC3339Nano)

	_ = cli.Delete(t.Context(), module)
	NewWithT(t).Eventually(t.Context(), k8sm.NotFound(cli, module)).Should(BeTrue())
	module.SetResourceVersion("")
	module.SetUID("")

	t.Cleanup(func() {
		_ = cli.Delete(context.Background(), module)
	})

	current := &componentsv1alpha1.DataSciencePipelines{}
	err := cli.Get(t.Context(), client.ObjectKeyFromObject(module), current)
	if k8serr.IsNotFound(err) {
		if err := cli.Create(t.Context(), module); err != nil {
			t.Fatalf("creating module: %v", err)
		}

		return
	}
	if err != nil {
		t.Fatalf("getting module before upsert: %v", err)
	}

	current.Spec = module.Spec
	current.Annotations = module.Annotations
	if err := cli.Update(t.Context(), current); err != nil {
		t.Fatalf("updating module: %v", err)
	}
}
