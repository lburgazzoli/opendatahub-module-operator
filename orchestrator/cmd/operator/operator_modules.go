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

package operator

import (
	"fmt"
	"path/filepath"
	"strings"

	orchestratorconfig "github.com/lburgazzoli/opendatahub-module-operator/orchestrator/pkg/config"
	"github.com/lburgazzoli/opendatahub-module-operator/orchestrator/pkg/module"
	"github.com/lburgazzoli/opendatahub-module-operator/orchestrator/pkg/resources/gvk"
)

func newModules(cfg *orchestratorconfig.Config) ([]*module.Module, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config is nil")
	}

	specs := []module.ModuleSpec{
		{
			Name:      "ray",
			GVK:       gvk.Ray,
			Namespace: cfg.Namespace() + "-" + strings.ToLower(gvk.Ray.Kind),
			ChartPath: filepath.Join(cfg.ChartsPath, "opendatahub-ray-operator"),
			Runlevel:  2,
		},
		{
			Name:      "spark",
			GVK:       gvk.Spark,
			Namespace: cfg.Namespace() + "-" + strings.ToLower(gvk.Spark.Kind),
			ChartPath: filepath.Join(cfg.ChartsPath, "opendatahub-spark-operator"),
			Runlevel:  2,
		},
	}

	modules := make([]*module.Module, 0, len(specs))
	for _, spec := range specs {
		mod, err := module.NewModule(spec)
		if err != nil {
			return nil, fmt.Errorf("creating module %q: %w", spec.Name, err)
		}
		modules = append(modules, mod)
	}

	return modules, nil
}
