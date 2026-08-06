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

// Package integration holds this module's integration test suite, run against
// the connected cluster (docs/plan.md §11 "Definition of done").
package integration

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/go-logr/logr"
	. "github.com/onsi/gomega"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	moduleconfig "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/pkg/config"
	modulemanager "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/pkg/manager"
	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/pkg/postgres"
	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/test/support"
	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/test/support/cluster"
	testdb "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/test/support/db"
	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/test/support/portforward"
)

type integrationEnv struct {
	Cluster       cluster.Instance
	Client        client.Client
	Namespace     string
	Config        *moduleconfig.Config
	ClientFactory postgres.ClientFactory
	Database      *testdb.Instance
}

func TestIntegration(t *testing.T) {
	g := NewWithT(t)

	testCfg, err := support.LoadConfig()
	g.Expect(err).NotTo(HaveOccurred())

	SetDefaultEventuallyTimeout(testCfg.Gomega.EventuallyTimeout)
	SetDefaultEventuallyPollingInterval(testCfg.Gomega.EventuallyPollingInterval)
	SetDefaultConsistentlyPollingInterval(testCfg.Gomega.ConsistentlyPollingInterval)

	if testCfg.Operator.Logs {
		ctrl.SetLogger(zap.New(zap.UseDevMode(true)))
	} else {
		ctrl.SetLogger(logr.Discard())
	}

	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)

	tc, err := startIntegrationCluster(ctx, t, testCfg)
	g.Expect(err).NotTo(HaveOccurred())

	t.Cleanup(func() {
		_ = tc.Stop(context.Background())
	})

	moduleCfg, err := moduleconfig.Load()
	g.Expect(err).NotTo(HaveOccurred())

	moduleCfg.Controller.LeaderElection.Enabled = false
	moduleCfg.Controller.Metrics.BindAddress = "0"
	moduleCfg.Controller.Health.BindAddress = "0"
	moduleCfg.Controller.Pprof.BindAddress = "0"
	moduleCfg.OperatorNamespace = support.IntegrationTestNamespace()

	tracker, err := portforward.NewTracker(tc.Config())
	g.Expect(err).NotTo(HaveOccurred())
	t.Cleanup(func() {
		_ = tracker.Close(context.Background())
	})

	clientFactory := testdb.NewForwardingClientFactory(tracker)

	_, err = startIntegrationManager(
		ctx,
		tc.Config(),
		moduleCfg,
		modulemanager.WithPostgresClientFactory(clientFactory),
	)
	g.Expect(err).NotTo(HaveOccurred())

	sharedDB, err := testdb.Start(
		ctx,
		testdb.Options{
			Client:        tc.Client(),
			ClientFactory: clientFactory,
			Namespace:     support.IntegrationTestNamespace(),
			Image:         moduleCfg.Internal.PostgresImage,
		},
	)
	g.Expect(err).NotTo(HaveOccurred())
	t.Cleanup(func() {
		if err := sharedDB.Close(context.Background()); err != nil {
			t.Errorf("closing integration database: %v", err)
		}
	})

	t.Run("schema claim", func(t *testing.T) {
		env, err := newIntegrationEnv(tc, tc.Client(), clientFactory, sharedDB)
		g := NewWithT(t)
		g.Expect(err).NotTo(HaveOccurred())

		suite, err := newSchemaClaimSuite(t, env)
		g.Expect(err).NotTo(HaveOccurred())
		suite.Run(t)
	})

	t.Run("database claim", func(t *testing.T) {
		env, err := newIntegrationEnv(tc, tc.Client(), clientFactory, sharedDB)
		g := NewWithT(t)
		g.Expect(err).NotTo(HaveOccurred())

		suite, err := newDatabaseClaimSuite(t, env)
		g.Expect(err).NotTo(HaveOccurred())
		suite.Run(t)
	})

	t.Run("database provider external", func(t *testing.T) {
		env, err := newIntegrationEnv(tc, tc.Client(), clientFactory, sharedDB)
		g := NewWithT(t)
		g.Expect(err).NotTo(HaveOccurred())

		suite, err := newDatabaseProviderSuite(t, env)
		g.Expect(err).NotTo(HaveOccurred())
		suite.Run(t)
	})

	t.Run("database provider embedded", func(t *testing.T) {
		env, err := newIntegrationEnv(tc, tc.Client(), clientFactory, sharedDB)
		g := NewWithT(t)
		g.Expect(err).NotTo(HaveOccurred())

		suite := newInternalDatabaseProviderSuite(t, env)
		suite.Run(t)
	})
}

func newIntegrationEnv(
	testCluster cluster.Instance,
	cli client.Client,
	clientFactory postgres.ClientFactory,
	database *testdb.Instance,
) (*integrationEnv, error) {
	moduleCfg, err := moduleconfig.Load()
	if err != nil {
		return nil, fmt.Errorf("loading module config: %w", err)
	}
	moduleCfg.OperatorNamespace = support.IntegrationTestNamespace()

	return &integrationEnv{
		Cluster:       testCluster,
		Client:        cli,
		Namespace:     support.IntegrationTestNamespace(),
		Config:        moduleCfg,
		ClientFactory: clientFactory,
		Database:      database,
	}, nil
}

func startIntegrationCluster(
	ctx context.Context,
	t *testing.T,
	cfg *support.Config,
) (cluster.Instance, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config is nil")
	}

	tc, err := cluster.New(ctx, cfg, cluster.WithLogFn(t.Logf))
	if err != nil {
		return nil, fmt.Errorf("starting integration cluster: %w", err)
	}

	cli := tc.Client()
	if cli == nil {
		_ = tc.Stop(ctx)
		return nil, fmt.Errorf("integration client is nil")
	}

	if err := tc.Setup(ctx); err != nil {
		_ = tc.Stop(ctx)
		return nil, fmt.Errorf("setting up integration cluster: %w", err)
	}

	if err := support.InstallCRD(ctx, cli); err != nil {
		_ = tc.Stop(ctx)
		return nil, fmt.Errorf("installing integration CRDs: %w", err)
	}

	return tc, nil
}

// startIntegrationManager starts the full module manager and ensures the
// integration namespace exists before tests begin.
func startIntegrationManager(
	ctx context.Context,
	restCfg *rest.Config,
	cfg *moduleconfig.Config,
	opts ...modulemanager.Option,
) (ctrl.Manager, error) {
	mgr, err := modulemanager.New(ctx, restCfg, cfg, opts...)
	if err != nil {
		return nil, fmt.Errorf("creating manager: %w", err)
	}

	go func() {
		if err := mgr.Start(ctx); err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "manager stopped: %v\n", err)
		}
	}()

	if !mgr.GetCache().WaitForCacheSync(ctx) {
		return nil, fmt.Errorf("cache failed to sync")
	}

	ns := support.IntegrationTestNamespace()

	if err := support.EnsureNamespace(ctx, mgr.GetClient(), ns); err != nil {
		return nil, fmt.Errorf("creating namespace %q: %w", ns, err)
	}

	return mgr, nil
}
