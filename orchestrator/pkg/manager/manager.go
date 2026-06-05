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
	orchestratorconfig "github.com/lburgazzoli/opendatahub-module-operator/orchestrator/pkg/config"
	libcache "github.com/opendatahub-io/odh-platform-utilities/pkg/cache"
)

const (
	healthCheckName = "healthz"
	readyCheckName  = "readyz"
)

func New(
	ctx context.Context,
	kubeConfig *rest.Config,
	cfg *orchestratorconfig.Config,
	o *platform.Orchestrator,
) (ctrl.Manager, error) {
	if kubeConfig == nil {
		return nil, fmt.Errorf("kubeconfig is nil")
	}
	if cfg == nil {
		return nil, fmt.Errorf("config is nil")
	}

	scheme := NewScheme()

	cacheNamespaces := buildCacheNamespaces(o)

	mgrOpts := ctrl.Options{
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
				&configv1alpha1.Platform{}: {Label: k8slabels.Everything()},
			},
			ReaderFailOnMissingInformer: true,
		},
		Client: client.Options{
			Cache: &client.CacheOptions{
				Unstructured: true,
				DisableFor: []client.Object{
					&corev1.ConfigMap{},
					&corev1.Secret{},
				},
			},
		},
	}

	ctrlMgr, err := ctrl.NewManager(kubeConfig, mgrOpts)
	if err != nil {
		return nil, fmt.Errorf("creating manager: %w", err)
	}

	o.SetRecorder(ctrlMgr.GetEventRecorderFor("platform-orchestrator"))

	if err := platform.NewReconciler(ctx, ctrlMgr, o); err != nil {
		return nil, fmt.Errorf("creating platform reconciler: %w", err)
	}

	if err := ctrlMgr.AddHealthzCheck(healthCheckName, healthz.Ping); err != nil {
		return nil, fmt.Errorf("setting up health check: %w", err)
	}
	if err := ctrlMgr.AddReadyzCheck(readyCheckName, healthz.Ping); err != nil {
		return nil, fmt.Errorf("setting up ready check: %w", err)
	}

	return ctrlMgr, nil
}
