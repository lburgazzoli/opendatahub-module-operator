package e2e

import (
	"context"
	"os"
	"testing"

	k8sm "github.com/lburgazzoli/gomega-matchers/pkg/matchers/k8s"
	"github.com/lburgazzoli/gomega-matchers/pkg/matchers/k8s/condition"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	infraApi "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/api/infrastructure/v1alpha1"
	servicesApi "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/api/services/v1alpha1"
	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/internal/controller/databaseprovider"
	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/test/support"
)

func e2eTestNamespace() string {
	if namespace := os.Getenv("E2E_TEST_NAMESPACE"); namespace != "" {
		return namespace
	}

	return defaultE2ETestNamespace
}

func TestDatabaseOperatorE2EFoundation(t *testing.T) {
	suite := newE2ESuite(t)

	t.Run("installs module CRDs", suite.testCRDsInstalled)
	t.Run("deploys operator config", suite.testOperatorConfigMap)
}

func (st *e2eSuite) testCRDsInstalled(t *testing.T) {
	g := NewWithT(t)

	for _, name := range []string{
		infraApi.SchemaClaimResource + "." + infraApi.GroupName,
		infraApi.DatabaseClaimResource + "." + infraApi.GroupName,
		infraApi.DatabaseProviderResource + "." + infraApi.GroupName,
		servicesApi.DatabaseServiceCRDName,
	} {
		crd := &apiextensionsv1.CustomResourceDefinition{
			ObjectMeta: metav1.ObjectMeta{Name: name},
		}
		g.Eventually(t.Context(), k8sm.Lookup(st.Client, crd)).Should(Succeed())
	}
}

func (st *e2eSuite) testOperatorConfigMap(t *testing.T) {
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      operatorConfigMapName,
			Namespace: st.operatorNamespace,
		},
	}

	NewWithT(t).Eventually(t.Context(), k8sm.Get(st.Client, cm)).Should(
		WithTransform(k8sm.Data(), SatisfyAll(
			HaveKey("platformType"),
			HaveKey("platformVersion"),
		)),
	)
}

func (st *e2eSuite) ensureWorkloadNamespace(t *testing.T) {
	t.Helper()
	NewWithT(t).Expect(support.EnsureNamespace(t.Context(), st.Client, st.workloadNamespace)).To(Succeed())
}

func (st *e2eSuite) waitProviderReachable(t *testing.T, provider *infraApi.DatabaseProvider) {
	t.Helper()

	NewWithT(t).Eventually(t.Context(), k8sm.Get(st.Client, provider)).Should(
		WithTransform(k8sm.ConditionsOf[metav1.Condition](),
			ContainElement(condition.Is(databaseprovider.ConditionReachable, metav1.ConditionTrue))),
	)
}

func (st *e2eSuite) expectProviderUnreachable(
	t *testing.T,
	provider *infraApi.DatabaseProvider,
	reason string,
	messageSubstring string,
) {
	t.Helper()

	NewWithT(t).Eventually(t.Context(), k8sm.Get(st.Client, provider)).Should(
		WithTransform(k8sm.ConditionsOf[metav1.Condition](), ContainElement(
			SatisfyAll(
				HaveField("Type", Equal(databaseprovider.ConditionReachable)),
				HaveField("Status", Equal(metav1.ConditionFalse)),
				HaveField("Reason", Equal(reason)),
				HaveField("Message", ContainSubstring(messageSubstring)),
			),
		)),
	)
}

func (st *e2eSuite) deleteAndWait(ctx context.Context, t *testing.T, obj client.Object) {
	t.Helper()

	key := client.ObjectKeyFromObject(obj)
	if err := st.Client.Get(ctx, key, obj); err != nil {
		if apierrors.IsNotFound(err) {
			return
		}
		t.Fatalf("reading %s before delete: %v", key, err)
	}

	if err := st.Client.Delete(ctx, obj); err != nil && !apierrors.IsNotFound(err) {
		t.Fatalf("deleting %s: %v", key, err)
	}

	NewWithT(t).Eventually(ctx, k8sm.NotFound(st.Client, obj)).Should(BeTrue(), "waiting for %s to be deleted", key)
}
