package gates

import (
	"fmt"
	"os"
	"testing"

	k8sm "github.com/lburgazzoli/gomega-matchers/pkg/matchers/k8s"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	k8serr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/lburgazzoli/gomega-matchers/pkg/matchers/jq"
	isupport "github.com/lburgazzoli/opendatahub-module-operator/orchestrator/test/integration/support"
	testsupport "github.com/lburgazzoli/opendatahub-module-operator/orchestrator/test/support"
)

func TestMain(m *testing.M) {
	suite, err := isupport.Setup(isupport.RunConfig{
		Modules:        gatesModules(),
		CleanupModules: gatesCleanupModules(),
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

func TestMissingConfigMapBlocksGatedModules(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()
	suite := isupport.NewSuite(t)
	suite.SetupTest(t)

	g.Expect(suite.Client.Create(ctx, isupport.NewPlatformWithModules([]string{
		alphaModuleName,
		gatedModuleName,
	}))).To(Succeed())

	assertAdminAckBlocked(t, suite, "Acknowledge gated module rollout")
}

func TestFalseAdminAckBlocksGatedModules(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()
	suite := isupport.NewSuite(t)
	suite.SetupTest(t)

	g.Expect(suite.Client.Create(ctx, isupport.NewPlatformWithModules([]string{
		alphaModuleName,
		gatedModuleName,
	}))).To(Succeed())

	namespace := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: suite.Config.Namespace()}}
	g.Expect(suite.Client.Create(ctx, namespace)).To(Or(
		Succeed(),
		Satisfy(k8serr.IsAlreadyExists),
	))

	adminAcks := isupport.AdminAcksConfigMap(suite.Config.Namespace())
	adminAcks.Data = map[string]string{
		testAdminAckKey: "false",
	}
	g.Expect(suite.Client.Create(ctx, adminAcks)).To(Succeed())

	assertAdminAckBlocked(t, suite, `value: "false"`)
}

func TestUpdatingAdminAckFromFalseToTrueUnblocksModules(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()
	suite := isupport.NewSuite(t)
	suite.SetupTest(t)

	g.Expect(suite.Client.Create(ctx, isupport.NewPlatformWithModules([]string{
		alphaModuleName,
		gatedModuleName,
	}))).To(Succeed())

	namespace := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: suite.Config.Namespace()}}
	g.Expect(suite.Client.Create(ctx, namespace)).To(Or(
		Succeed(),
		Satisfy(k8serr.IsAlreadyExists),
	))

	adminAcks := isupport.AdminAcksConfigMap(suite.Config.Namespace())
	adminAcks.Data = map[string]string{
		testAdminAckKey: "false",
	}
	g.Expect(suite.Client.Create(ctx, adminAcks)).To(Succeed())

	assertAdminAckBlocked(t, suite, `value: "false"`)

	g.Eventually(
		ctx,
		k8sm.Update(
			suite.K,
			isupport.AdminAcksConfigMap(suite.Config.Namespace()),
			func(cm *corev1.ConfigMap) {
				cm.Data[testAdminAckKey] = "true"
			},
		),
	).Should(
		WithTransform(k8sm.Data(), HaveKeyWithValue(testAdminAckKey, Equal("true"))),
	)

	for _, moduleName := range []string{alphaModuleName, gatedModuleName} {
		g.Eventually(ctx, suite.K.Get(testsupport.PlatformOperator(moduleName))).Should(
			testsupport.HaveTrackedResources(),
		)
	}
	g.Eventually(ctx, suite.K.Get(testsupport.Platform())).Should(
		jq.Match(`(.status.conditions // []) | all(.reason != "AdminAcksRequired")`),
	)
}
