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

package manager

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	k8slabels "k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	configv1alpha1 "github.com/lburgazzoli/opendatahub-module-operator/orchestrator/api/config/v1alpha1"
	"github.com/lburgazzoli/opendatahub-module-operator/orchestrator/internal/controller/platform"
	"github.com/lburgazzoli/opendatahub-module-operator/orchestrator/internal/controller/platformoperator"
	orchestratorconfig "github.com/lburgazzoli/opendatahub-module-operator/orchestrator/pkg/config"
	"github.com/lburgazzoli/opendatahub-module-operator/orchestrator/pkg/module"
	libcache "github.com/opendatahub-io/odh-platform-utilities/pkg/cache"
	odhmgr "github.com/opendatahub-io/opendatahub-operator/v2/pkg/manager"
)

const (
	healthCheckName = "healthz"
	readyCheckName  = "readyz"
)

// SetupReconcilers wraps the given manager so that GetClient() returns a
// client that routes typed reads through the unstructured cache (matching
// the framework's unstructured watches), then registers the Platform and
// PlatformOperator reconcilers.
func SetupReconcilers(
	ctx context.Context,
	mgr ctrl.Manager,
	registry *module.Registry,
	cfg *orchestratorconfig.Config,
) error {
	wrapped := odhmgr.New(mgr)

	if err := platform.NewReconciler(ctx, wrapped, registry, cfg); err != nil {
		return fmt.Errorf("creating platform reconciler: %w", err)
	}

	if err := platformoperator.NewModuleReconciler(ctx, wrapped, registry, cfg); err != nil {
		return fmt.Errorf("creating module reconciler: %w", err)
	}

	return nil
}

func New(
	ctx context.Context,
	kubeConfig *rest.Config,
	cfg *orchestratorconfig.Config,
	registry *module.Registry,
) (ctrl.Manager, error) {
	if kubeConfig == nil {
		return nil, fmt.Errorf("kubeconfig is nil")
	}
	if cfg == nil {
		return nil, fmt.Errorf("config is nil")
	}

	scheme := NewScheme()
	cacheNamespaces := buildCacheNamespaces(registry)

	ctrlMgr, err := ctrl.NewManager(kubeConfig, ctrl.Options{
		Scheme: scheme,
		Metrics: metricsserver.Options{
			BindAddress: cfg.Controller.Metrics.BindAddress,
		},
		HealthProbeBindAddress:        cfg.Controller.Health.BindAddress,
		PprofBindAddress:              cfg.Controller.Pprof.BindAddress,
		LeaderElection:                cfg.Controller.LeaderElection.Enabled,
		LeaderElectionID:              cfg.Controller.LeaderElection.ID,
		LeaderElectionReleaseOnCancel: true,
		Cache: cache.Options{
			DefaultTransform:  libcache.StripUnusedFields(),
			DefaultNamespaces: cacheNamespaces,
			ByObject: map[client.Object]cache.ByObject{
				&configv1alpha1.Platform{}:         {Label: k8slabels.Everything()},
				&configv1alpha1.PlatformOperator{}: {Label: k8slabels.Everything()},
			},
			ReaderFailOnMissingInformer: false,
		},
		Client: client.Options{
			Cache: &client.CacheOptions{
				Unstructured: true,
				DisableFor: []client.Object{
					&corev1.Namespace{},
					&corev1.ConfigMap{},
					&corev1.Secret{},
				},
			},
		},
	})

	if err != nil {
		return nil, fmt.Errorf("creating manager: %w", err)
	}

	mgr := odhmgr.New(ctrlMgr)

	if err := platform.NewReconciler(ctx, mgr, registry, cfg); err != nil {
		return nil, fmt.Errorf("creating platform reconciler: %w", err)
	}

	if err := platformoperator.NewModuleReconciler(ctx, mgr, registry, cfg); err != nil {
		return nil, fmt.Errorf("creating module reconciler: %w", err)
	}

	if err := mgr.AddHealthzCheck(healthCheckName, healthz.Ping); err != nil {
		return nil, fmt.Errorf("setting up health check: %w", err)
	}
	if err := mgr.AddReadyzCheck(readyCheckName, healthz.Ping); err != nil {
		return nil, fmt.Errorf("setting up ready check: %w", err)
	}

	return mgr, nil
}
