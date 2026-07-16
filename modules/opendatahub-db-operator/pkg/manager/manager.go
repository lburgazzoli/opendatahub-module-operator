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

	"github.com/opendatahub-io/odh-platform-utilities/framework/resources"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	infraApi "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/api/infrastructure/v1alpha1"
	servicesv1alpha1 "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/api/services/v1alpha1"
	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/internal/controller/databaseclaim"
	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/internal/controller/databaseprovider"
	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/internal/controller/databaseservice"
	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/internal/controller/schemaclaim"
	moduleconfig "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/pkg/config"
	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/pkg/postgres"
	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/pkg/resources/gvk"
	odhmanager "github.com/opendatahub-io/odh-platform-utilities/framework/manager"
	libcache "github.com/opendatahub-io/odh-platform-utilities/pkg/cache"
)

const (
	healthCheckName = "healthz"
	readyCheckName  = "readyz"
)

// NewScheme registers all types this module needs.
func NewScheme() (*runtime.Scheme, error) {
	scheme := runtime.NewScheme()

	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		return nil, fmt.Errorf("adding client-go scheme: %w", err)
	}
	if err := apiextensionsv1.AddToScheme(scheme); err != nil {
		return nil, fmt.Errorf("adding apiextensions scheme: %w", err)
	}
	if err := infraApi.AddToScheme(scheme); err != nil {
		return nil, fmt.Errorf("adding infrastructure scheme: %w", err)
	}
	if err := servicesv1alpha1.AddToScheme(scheme); err != nil {
		return nil, fmt.Errorf("adding services scheme: %w", err)
	}

	return scheme, nil
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

	scheme, err := NewScheme()
	if err != nil {
		return nil, err
	}

	managerOpts := Options{
		PostgresClientFactory: postgres.DefaultClientFactory,
	}
	for _, opt := range opts {
		if opt != nil {
			opt.applyOption(&managerOpts)
		}
	}

	ctrlMgr, err := ctrl.NewManager(kubeConfig, ctrl.Options{
		Scheme: scheme,
		Metrics: metricsserver.Options{
			BindAddress: cfg.Controller.Metrics.BindAddress,
		},
		HealthProbeBindAddress:        cfg.Controller.Health.BindAddress,
		PprofBindAddress:              cfg.Controller.Pprof.BindAddress,
		LeaderElection:                cfg.Controller.LeaderElection.Enabled,
		LeaderElectionID:              cfg.Controller.LeaderElection.ID,
		LeaderElectionNamespace:       cfg.OperatorNamespace,
		LeaderElectionReleaseOnCancel: true,
		Cache: cache.Options{
			DefaultTransform:            libcache.StripUnusedFields(),
			ReaderFailOnMissingInformer: true,
		},
		Client: client.Options{
			Cache: &client.CacheOptions{
				Unstructured: true,
				DisableFor: []client.Object{
					&corev1.ConfigMap{},
					&corev1.Secret{},
					resources.GvkToUnstructured(gvk.CertManagerIssuer),
					resources.GvkToUnstructured(gvk.CertManagerCertificate),
				},
			},
		},
	})

	if err != nil {
		return nil, fmt.Errorf("creating manager: %w", err)
	}

	mgr := odhmanager.New(
		ctrlMgr,
	)

	if err := databaseservice.NewReconciler(ctx, mgr, cfg, databaseservice.Options{
		Recorder: mgr.GetEventRecorder(servicesv1alpha1.DatabaseServiceResource),
	}); err != nil {
		return nil, fmt.Errorf("creating databaseservice reconciler: %w", err)
	}

	if err := schemaclaim.NewReconciler(ctx, mgr, cfg, schemaclaim.Options{
		Recorder:              mgr.GetEventRecorder(infraApi.SchemaClaimResource),
		PostgresClientFactory: managerOpts.PostgresClientFactory,
	}); err != nil {
		return nil, fmt.Errorf("creating schemaclaim reconciler: %w", err)
	}

	if err := databaseclaim.NewReconciler(ctx, mgr, cfg, databaseclaim.Options{
		Recorder:              mgr.GetEventRecorder(infraApi.DatabaseClaimResource),
		PostgresClientFactory: managerOpts.PostgresClientFactory,
	}); err != nil {
		return nil, fmt.Errorf("creating databaseclaim reconciler: %w", err)
	}

	if err := databaseprovider.NewReconciler(ctx, mgr, cfg, databaseprovider.Options{
		Recorder:              mgr.GetEventRecorder(infraApi.DatabaseProviderResource),
		PostgresClientFactory: managerOpts.PostgresClientFactory,
	}); err != nil {
		return nil, fmt.Errorf("creating databaseprovider reconciler: %w", err)
	}

	if err := mgr.AddHealthzCheck(healthCheckName, healthz.Ping); err != nil {
		return nil, fmt.Errorf("setting up health check: %w", err)
	}
	if err := mgr.AddReadyzCheck(readyCheckName, healthz.Ping); err != nil {
		return nil, fmt.Errorf("setting up ready check: %w", err)
	}

	return mgr, nil
}
