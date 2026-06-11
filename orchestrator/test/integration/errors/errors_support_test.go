package errors

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/lburgazzoli/opendatahub-module-operator/orchestrator/pkg/module"
	isupport "github.com/lburgazzoli/opendatahub-module-operator/orchestrator/test/integration/support"
	testsupport "github.com/lburgazzoli/opendatahub-module-operator/orchestrator/test/support"
)

const (
	healthyModuleName = "healthy"
	failingModuleName = "failing"
	brokenModuleName  = "broken"
)

var (
	componentsGV = schema.GroupVersion{
		Group:   "test.opendatahub.io",
		Version: "v1alpha1",
	}
	healthyGVK = componentsGV.WithKind("Healthy")
	failingGVK = componentsGV.WithKind("Failing")
	brokenGVK  = componentsGV.WithKind("Broken")
)

func errorsModules() []*module.Module {
	return []*module.Module{
		newModule(healthyModuleName, healthyGVK, map[string]any{
			"module-name": healthyModuleName,
		}),
		newFailingConfigModule(),
		newBrokenChartModule(),
	}
}

func newFailingConfigModule() *module.Module {
	mod := newModule(failingModuleName, failingGVK, nil)
	mod.Config = func(_ context.Context, _ client.Client) (map[string]any, error) {
		return nil, fmt.Errorf("simulated config failure")
	}

	return mod
}

func newBrokenChartModule() *module.Module {
	mod := newModule(brokenModuleName, brokenGVK, map[string]any{
		"module-name": brokenModuleName,
	})
	mod.Manifests.Chart.Path = brokenChartPath()

	return mod
}

func newModule(
	name string,
	gvk schema.GroupVersionKind,
	values map[string]any,
) *module.Module {
	mod := &module.Module{
		Name:      name,
		GVK:       gvk,
		Namespace: isupport.TestNamespace + "-" + strings.ToLower(gvk.Kind),
		Runlevel:  1,
	}
	if values != nil {
		mod.Config = func(_ context.Context, _ client.Client) (map[string]any, error) {
			return values, nil
		}
	}

	return mod
}

func brokenChartPath() string {
	root, err := testsupport.ProjectRoot()
	if err != nil {
		panic(fmt.Sprintf("finding project root: %v", err))
	}

	return filepath.Join(root, "orchestrator", "test", "support", "testdata", "charts", "broken-module")
}
