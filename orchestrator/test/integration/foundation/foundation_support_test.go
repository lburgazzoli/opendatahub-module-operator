//go:build integration

package foundation

import (
	"context"
	"strings"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/lburgazzoli/opendatahub-module-operator/orchestrator/pkg/module"
	isupport "github.com/lburgazzoli/opendatahub-module-operator/orchestrator/test/integration/support"
)

const (
	alphaModuleName = "alpha"
	betaModuleName  = "beta"
	gammaModuleName = "gamma"
	gatedModuleName = "gated"
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

func foundationModules() []*module.Module {
	return []*module.Module{
		newModule(alphaModuleName, alphaGVK, 1, map[string]any{
			"module-name":   alphaModuleName,
			"platform-name": "TestPlatform",
			"test-key":      "test-value",
		}),
		newModule(betaModuleName, betaGVK, 2, map[string]any{
			"module-name": betaModuleName,
		}),
		newModule(gammaModuleName, gammaGVK, 2, map[string]any{
			"module-name": gammaModuleName,
		}),
	}
}

func foundationCleanupModules() []*module.Module {
	return []*module.Module{
		newModule(alphaModuleName, alphaGVK, 1, nil),
		newModule(betaModuleName, betaGVK, 2, nil),
		newModule(gammaModuleName, gammaGVK, 2, nil),
		newModule(gatedModuleName, gatedGVK, 1, nil),
	}
}

func newModule(
	name string,
	gvk schema.GroupVersionKind,
	runlevel int,
	values map[string]any,
) *module.Module {
	mod := &module.Module{
		Name:      name,
		GVK:       gvk,
		Namespace: isupport.TestNamespace + "-" + strings.ToLower(gvk.Kind),
		Runlevel:  runlevel,
	}
	if values != nil {
		mod.Config = func(_ context.Context, _ client.Client) (map[string]any, error) {
			return values, nil
		}
	}

	return mod
}
