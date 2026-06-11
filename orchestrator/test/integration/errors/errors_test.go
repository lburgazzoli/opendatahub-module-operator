package errors

import (
	"fmt"
	"os"
	"testing"

	k8sm "github.com/lburgazzoli/gomega-matchers/pkg/matchers/k8s"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	configApi "github.com/lburgazzoli/opendatahub-module-operator/orchestrator/api/config/v1alpha1"
	isupport "github.com/lburgazzoli/opendatahub-module-operator/orchestrator/test/integration/support"
	testsupport "github.com/lburgazzoli/opendatahub-module-operator/orchestrator/test/support"
)

func TestMain(m *testing.M) {
	suite, err := isupport.Setup(isupport.RunConfig{
		Modules: errorsModules(),
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to setup integration suite: %v\n", err)
		os.Exit(1)
	}

	code := suite.Run(m)
	if err := suite.TearDown(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to teardown integration suite: %v\n", err)
		if code == 0 {
			code = 1
		}
	}

	os.Exit(code)
}

func TestModuleWithFailingConfigDoesNotDeploy(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()
	suite := isupport.NewSuite(t)
	suite.SetupTest(t)

	p := &configApi.Platform{
		ObjectMeta: metav1.ObjectMeta{Name: configApi.PlatformInstanceName},
		Spec: configApi.PlatformSpec{
			Modules: []string{healthyModuleName, failingModuleName},
		},
	}
	g.Expect(suite.Client.Create(ctx, p)).To(Succeed())

	g.Eventually(ctx, k8sm.Get(suite.Client, testsupport.PlatformOperator(healthyModuleName))).Should(
		testsupport.HaveTrackedResources(),
	)

	g.Consistently(ctx, k8sm.Get(suite.Client, testsupport.PlatformOperator(failingModuleName))).
		WithTimeout(isupport.Timeout).
		Should(testsupport.HaveNoTrackedResources())

	g.Eventually(ctx, k8sm.Get(suite.Client, testsupport.Platform())).Should(
		WithTransform(k8sm.Conditions(), ContainElement(SatisfyAll(
			HaveKeyWithValue("type", configApi.ConditionReady),
			HaveKeyWithValue("status", string(metav1.ConditionFalse)),
		))),
	)
}
