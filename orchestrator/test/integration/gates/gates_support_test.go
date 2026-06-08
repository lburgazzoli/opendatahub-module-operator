//go:build integration

package gates

import (
	"context"
	"strings"
	"testing"

	"github.com/lburgazzoli/gomega-matchers/pkg/matchers/jq"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"

	configApi "github.com/lburgazzoli/opendatahub-module-operator/orchestrator/api/config/v1alpha1"
	"github.com/lburgazzoli/opendatahub-module-operator/orchestrator/pkg/module"
	isupport "github.com/lburgazzoli/opendatahub-module-operator/orchestrator/test/integration/support"
	testsupport "github.com/lburgazzoli/opendatahub-module-operator/orchestrator/test/support"
)

const (
	alphaModuleName = "alpha"
	betaModuleName  = "beta"
	gammaModuleName = "gamma"
	gatedModuleName = "gated"
	testAdminAckKey = "adminAcks.testGate"
)

var (
	componentsGV = schema.GroupVersion{
		Group:   "test.opendatahub.io",
		Version: "v1alpha1",
	}
	alphaGVK = componentsGV.WithKind("Alpha")
	betaGVK  = componentsGV.WithKind("Beta")
	gammaGVK = componentsGV.WithKind("Gamma")
	gatedGVK = componentsGV.WithKind("Gated")
)

func gatesModules() []*module.Module {
	return []*module.Module{
		newModule(alphaModuleName, alphaGVK, 1, map[string]any{
			"module-name": alphaModuleName,
		}, nil),
		newModule(gatedModuleName, gatedGVK, 1, map[string]any{
			"module-name": gatedModuleName,
		}, []module.AdminAck{{
			Name:        testAdminAckKey,
			Description: "Acknowledge gated module rollout",
		}}),
	}
}

func gatesCleanupModules() []*module.Module {
	return []*module.Module{
		newModule(alphaModuleName, alphaGVK, 1, nil, nil),
		newModule(betaModuleName, betaGVK, 2, nil, nil),
		newModule(gammaModuleName, gammaGVK, 2, nil, nil),
		newModule(gatedModuleName, gatedGVK, 1, nil, nil),
	}
}

func newModule(
	name string,
	gvk schema.GroupVersionKind,
	runlevel int,
	values map[string]any,
	adminAcks []module.AdminAck,
) *module.Module {
	mod := &module.Module{
		Name:      name,
		GVK:       gvk,
		Namespace: isupport.TestNamespace + "-" + strings.ToLower(gvk.Kind),
		Runlevel:  runlevel,
		AdminAcks: adminAcks,
	}
	if values != nil {
		mod.Config = func(_ context.Context, _ client.Client) (map[string]any, error) {
			return values, nil
		}
	}

	return mod
}

func assertAdminAckBlocked(t *testing.T, suite *isupport.Suite, messageSubstring string) {
	t.Helper()
	g := NewWithT(t)

	g.Eventually(t.Context(), suite.K.List(&configApi.PlatformOperatorList{})).Should(
		jq.Match(`length == 0`),
	)
	g.Eventually(t.Context(), suite.K.Get(testsupport.Platform())).Should(
		jq.Matchf(
			`(.status.conditions // []) | any(
				.type == "ModulesReady" and
				.status == "False" and
				.reason == "AdminAcksRequired" and
				(.message | contains(%q))
			)`,
			messageSubstring,
		),
	)
}
