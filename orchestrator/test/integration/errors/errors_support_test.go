package errors

import (
	"context"
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/lburgazzoli/opendatahub-module-operator/orchestrator/pkg/module"
	isupport "github.com/lburgazzoli/opendatahub-module-operator/orchestrator/test/integration/support"
)

const (
	healthyModuleName = "healthy"
	failingModuleName = "failing"
)

var (
	componentsGV = schema.GroupVersion{
		Group:   "test.opendatahub.io",
		Version: "v1alpha1",
	}
	healthyGVK = componentsGV.WithKind("Healthy")
	failingGVK = componentsGV.WithKind("Failing")
)

func errorsModules() []*module.Module {
	healthy := newModule(healthyModuleName, healthyGVK, 1, map[string]any{
		"module-name": healthyModuleName,
	})

	failing := newModule(failingModuleName, failingGVK, 1, nil)
	failing.Config = func(_ context.Context, _ client.Client) (map[string]any, error) {
		return nil, fmt.Errorf("simulated config failure")
	}

	return []*module.Module{healthy, failing}
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
