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

func (st *e2eSuite) runFoundation(t *testing.T) {
	t.Run("installs module CRDs", st.testCRDsInstalled)
	t.Run("deploys operator config", st.testOperatorConfigMap)
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
	g := NewWithT(t)

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      operatorConfigMapName,
			Namespace: st.operatorNamespace,
		},
	}

	g.Eventually(t.Context(), k8sm.Get(st.Client, cm)).Should(
		WithTransform(k8sm.Data(), SatisfyAll(
			HaveKey("platformType"),
			HaveKey("platformVersion"),
		)),
	)
}

func (st *e2eSuite) ensureWorkloadNamespace(t *testing.T) {
	t.Helper()

	g := NewWithT(t)
	g.Expect(support.EnsureNamespace(t.Context(), st.Client, st.workloadNamespace)).To(Succeed())
}

func (st *e2eSuite) waitProviderReachable(t *testing.T, provider *infraApi.DatabaseProvider) {
	t.Helper()

	g := NewWithT(t)
	g.Eventually(t.Context(), k8sm.Get(st.Client, provider)).Should(
		WithTransform(k8sm.ConditionsOf[metav1.Condition](),
			ContainElement(condition.Is(databaseprovider.ConditionReachable, metav1.ConditionTrue))),
	)
}

func (st *e2eSuite) deleteAndWait(ctx context.Context, t *testing.T, obj client.Object) {
	t.Helper()

	g := NewWithT(t)
	g.Expect(support.DeleteAndWait(ctx, st.Client, obj)).To(Succeed())
}
