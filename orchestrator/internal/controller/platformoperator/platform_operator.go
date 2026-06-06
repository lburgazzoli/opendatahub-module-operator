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

package platformoperator

import (
	"sync"

	engine "github.com/k8s-manifest-kit/engine/pkg"
	"sigs.k8s.io/controller-runtime/pkg/client"

	configApi "github.com/lburgazzoli/opendatahub-module-operator/orchestrator/api/config/v1alpha1"
	orchestratorconfig "github.com/lburgazzoli/opendatahub-module-operator/orchestrator/pkg/config"
	"github.com/lburgazzoli/opendatahub-module-operator/orchestrator/pkg/module"
)

// ModuleReconciler handles deployment of all modules via a single controller.
type ModuleReconciler struct {
	o      module.Orchestration
	cfg    *orchestratorconfig.Config
	client client.Client

	mu       sync.RWMutex
	contexts map[string]*moduleContext
}

type moduleContext struct {
	module    *module.Module
	engine    *engine.Engine
	chartInfo configApi.ChartInfo
}
