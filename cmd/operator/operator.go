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

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	componentsv1alpha1 "github.com/lburgazzoli/opendatahub-module-operator/api/components/v1alpha1"
	"github.com/lburgazzoli/opendatahub-module-operator/internal/controller/components/mymodule"
	libcache "github.com/lburgazzoli/opendatahub-module-operator/pkg/cache"
	moduleconfig "github.com/lburgazzoli/opendatahub-module-operator/pkg/config"
	"github.com/opendatahub-io/opendatahub-operator/v2/pkg/cluster"
	odhmanager "github.com/opendatahub-io/opendatahub-operator/v2/pkg/manager"
)

const (
	healthCheckName = "healthz"
	readyCheckName  = "readyz"
)

var scheme = runtime.NewScheme()

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(componentsv1alpha1.AddToScheme(scheme))
}

// NewCommand returns the cobra command for the operator subcommand.
func NewCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "operator",
		Short: "Start the module operator",
		RunE:  run,
	}

	return cmd
}

func run(cmd *cobra.Command, _ []string) error {
	// Load operator config from ConfigMap files, env vars.
	cfg, err := moduleconfig.Load()
	if err != nil {
		return fmt.Errorf("loading operator config: %w", err)
	}

	ctrl.SetLogger(zap.New(zap.UseDevMode(false)))

	// Set the applications namespace so that the operator's kustomize render
	// action can determine the target namespace without requiring DSCI.
	viper.Set("rhai-applications-namespace", cfg.ApplicationsNamespace)
	cluster.SetRHAIApplicationNamespace(cfg.ApplicationsNamespace)

	ctrlMgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme: scheme,
		Metrics: metricsserver.Options{
			BindAddress: cfg.MetricsAddr,
		},
		HealthProbeBindAddress:        cfg.HealthProbeAddr,
		PprofBindAddress:              cfg.PprofAddr,
		LeaderElection:                cfg.LeaderElect,
		LeaderElectionID:              cfg.LeaderElectionID,
		LeaderElectionReleaseOnCancel: true,

		// Cache configuration:
		// - Strip managedFields and last-applied-configuration to reduce memory.
		// - Scope the default watch to the applications namespace and
		//   cluster-scoped resources.
		Cache: cache.Options{
			DefaultTransform: libcache.StripUnusedFields(),
			DefaultNamespaces: map[string]cache.Config{
				cfg.ApplicationsNamespace: {},
				cache.AllNamespaces:       {},
			},
			// Fail with ErrResourceNotCached if a Get/List is called for a
			// resource type that has no informer running. This prevents
			// silent live API calls for types the controller forgot to
			// Owns() or Watches().
			ReaderFailOnMissingInformer: true,
		},

		// Client configuration:
		// - Enable cache reads for unstructured objects (used by kustomize rendering).
		// - Disable caching for ConfigMaps and Secrets so they are always read
		//   fresh (they change frequently and may contain sensitive data).
		Client: client.Options{
			Cache: &client.CacheOptions{
				Unstructured: true,
				DisableFor: []client.Object{
					&corev1.ConfigMap{},
					&corev1.Secret{},
				},
			},
		},
	})
	if err != nil {
		return fmt.Errorf("creating manager: %w", err)
	}

	// Wrap the manager with the manifests base path provider so that
	// ReconcilerFor can read it via the manifestsBasePathProvider interface.
	mgr := odhmanager.New(
		ctrlMgr,
		odhmanager.WithManifestsBasePath(cfg.ManifestsPath),
	)

	// Register controllers.
	if err := mymodule.NewReconciler(cmd.Context(), mgr, cfg); err != nil {
		return fmt.Errorf("creating mymodule reconciler: %w", err)
	}

	if err := mgr.AddHealthzCheck(healthCheckName, healthz.Ping); err != nil {
		return fmt.Errorf("setting up health check: %w", err)
	}
	if err := mgr.AddReadyzCheck(readyCheckName, healthz.Ping); err != nil {
		return fmt.Errorf("setting up ready check: %w", err)
	}

	return mgr.Start(cmd.Context())
}
