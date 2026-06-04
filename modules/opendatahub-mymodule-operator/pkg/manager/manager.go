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

	"github.com/spf13/viper"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	k8slabels "k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	webhookserver "sigs.k8s.io/controller-runtime/pkg/webhook"

	componentsv1alpha1 "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-mymodule-operator/api/components/v1alpha1"
	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-mymodule-operator/internal/controller/mymodule"
	moduleconfig "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-mymodule-operator/pkg/config"
	libcache "github.com/opendatahub-io/odh-platform-utilities/pkg/cache"
	"github.com/opendatahub-io/opendatahub-operator/v2/pkg/cluster"
	odhmanager "github.com/opendatahub-io/opendatahub-operator/v2/pkg/manager"
	odhLabels "github.com/opendatahub-io/opendatahub-operator/v2/pkg/metadata/labels"
)

const (
	healthCheckName = "healthz"
	readyCheckName  = "readyz"
)

type Option func(*ctrl.Options)

func NewScheme() *runtime.Scheme {
	scheme := runtime.NewScheme()

	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(componentsv1alpha1.AddToScheme(scheme))

	return scheme
}

func New(
	ctx context.Context,
	kubeConfig *rest.Config,
	cfg *moduleconfig.Config,
	opts ...Option,
) (ctrl.Manager, error) {
	if kubeConfig == nil {
		return nil, fmt.Errorf("kubeconfig is nil")
	}
	if cfg == nil {
		return nil, fmt.Errorf("config is nil")
	}

	viper.Set("rhai-applications-namespace", cfg.ApplicationsNamespace)
	cluster.SetRHAIApplicationNamespace(cfg.ApplicationsNamespace)

	scheme := NewScheme()
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
			DefaultTransform: libcache.StripUnusedFields(),
			DefaultNamespaces: map[string]cache.Config{
				cfg.ApplicationsNamespace: {
					LabelSelector: k8slabels.SelectorFromSet(k8slabels.Set{
						odhLabels.PlatformPartOf: odhLabels.NormalizePartOfValue(componentsv1alpha1.MyModuleKind),
					}),
				},
				cache.AllNamespaces: {
					LabelSelector: k8slabels.SelectorFromSet(k8slabels.Set{
						odhLabels.PlatformPartOf: odhLabels.NormalizePartOfValue(componentsv1alpha1.MyModuleKind),
					}),
				},
			},
			ByObject: map[client.Object]cache.ByObject{
				&componentsv1alpha1.MyModule{}: {Label: k8slabels.Everything()},
				// Ingress is an external dependency, so it must stay visible even
				// before the controller has had a chance to label managed resources.
				&networkingv1.Ingress{}: {
					Namespaces: map[string]cache.Config{
						cfg.ApplicationsNamespace: {
							LabelSelector: k8slabels.Everything(),
						},
					},
				},
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

	if cfg.Controller.Webhook.Enabled {
		mgrOpts.WebhookServer = webhookserver.NewServer(webhookserver.Options{
			Port:    cfg.Controller.Webhook.Port,
			CertDir: cfg.Controller.Webhook.CertDir,
		})
	}

	for _, opt := range opts {
		if opt != nil {
			opt(&mgrOpts)
		}
	}

	ctrlMgr, err := ctrl.NewManager(kubeConfig, mgrOpts)
	if err != nil {
		return nil, fmt.Errorf("creating manager: %w", err)
	}

	mgr := odhmanager.New(
		ctrlMgr,
		odhmanager.WithManifestsBasePath(cfg.ManifestsPath),
	)

	if err := mymodule.NewReconciler(ctx, mgr, cfg, cfg.Release()); err != nil {
		return nil, fmt.Errorf("creating mymodule reconciler: %w", err)
	}

	if err := mgr.AddHealthzCheck(healthCheckName, healthz.Ping); err != nil {
		return nil, fmt.Errorf("setting up health check: %w", err)
	}
	if err := mgr.AddReadyzCheck(readyCheckName, healthz.Ping); err != nil {
		return nil, fmt.Errorf("setting up ready check: %w", err)
	}

	return mgr, nil
}
