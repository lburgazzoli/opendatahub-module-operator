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

	"github.com/jackc/pgx/v5/pgxpool"
	k8sm "github.com/lburgazzoli/gomega-matchers/pkg/matchers/k8s"
	. "github.com/onsi/gomega"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	moduleconfig "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/pkg/config"
	modulemanager "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/pkg/manager"
	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/pkg/postgres"
	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/test/support"
)

type integrationEnv struct {
	Client    client.Client
	Namespace string
	Config    *moduleconfig.Config
}

type testDatabase struct {
	cfg       postgres.Config
	terminate func(context.Context) error
}

var sharedIntegrationEnv *integrationEnv

func TestMain(m *testing.M) {
	os.Exit(runTestMain(m))
}

func runTestMain(m *testing.M) int {
	gomegaCfg, err := support.LoadGomegaConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load test config: %v\n", err)
		return 1
	}
	SetDefaultEventuallyTimeout(gomegaCfg.EventuallyTimeout)
	SetDefaultEventuallyPollingInterval(gomegaCfg.EventuallyPollingInterval)
	SetDefaultConsistentlyPollingInterval(gomegaCfg.ConsistentlyPollingInterval)

	ctrl.SetLogger(zap.New(zap.UseDevMode(true)))

	restCfg, err := config.GetConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to read rest config: %v\n", err)
		return 1
	}

	moduleCfg, err := moduleconfig.LoadFromFS(nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load module config: %v\n", err)
		return 1
	}
	moduleCfg.Controller.LeaderElection.Enabled = false
	moduleCfg.Controller.Metrics.BindAddress = "0"
	moduleCfg.Controller.Health.BindAddress = "0"
	moduleCfg.Controller.Pprof.BindAddress = "0"
	moduleCfg.ApplicationsNamespace = support.IntegrationTestNamespace()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sharedIntegrationEnv, err = setupIntegrationEnv(ctx, restCfg, moduleCfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to set up integration environment: %v\n", err)
		return 1
	}

	return m.Run()
}

func newIntegrationEnv(t *testing.T) *integrationEnv {
	t.Helper()

	g := NewWithT(t)
	g.Expect(sharedIntegrationEnv).NotTo(BeNil(), "shared integration environment must be initialized in TestMain")

	return &integrationEnv{
		Client:    sharedIntegrationEnv.Client,
		Namespace: sharedIntegrationEnv.Namespace,
		Config:    sharedIntegrationEnv.Config,
	}
}

// startDatabase spins up a postgres:16 container for a claim integration suite.
func startDatabase(ctx context.Context) (*testDatabase, error) {
	ctr, err := tcpostgres.Run(ctx, "postgres:16",
		tcpostgres.WithDatabase("testdb"),
		tcpostgres.WithUsername("admin"),
		tcpostgres.WithPassword("adminpass"),
		tcpostgres.BasicWaitStrategies(),
	)
	if err != nil {
		return nil, fmt.Errorf("starting postgres container: %w", err)
	}
	connStr, err := ctr.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		_ = ctr.Terminate(ctx)
		return nil, fmt.Errorf("getting postgres connection string: %w", err)
	}
	cfg, err := postgres.ConfigFromDSN(connStr)
	if err != nil {
		_ = ctr.Terminate(ctx)
		return nil, fmt.Errorf("parsing postgres DSN: %w", err)
	}
	return &testDatabase{
		cfg: cfg,
		terminate: func(ctx context.Context) error {
			return ctr.Terminate(ctx)
		},
	}, nil
}

func (db *testDatabase) openAdminPool(ctx context.Context) (*pgxpool.Pool, error) {
	pool, err := pgxpool.New(ctx, db.cfg.DSN())
	if err != nil {
		return nil, fmt.Errorf("opening admin pool: %w", err)
	}
	return pool, nil
}

func (db *testDatabase) Close(ctx context.Context) error {
	if db == nil || db.terminate == nil {
		return nil
	}
	return db.terminate(ctx)
}

// setupIntegrationEnv starts the full module manager and returns the
// immutable-ish suite environment used by individual test groups.
func setupIntegrationEnv(
	ctx context.Context,
	restCfg *rest.Config,
	cfg *moduleconfig.Config,
	opts ...modulemanager.Option,
) (*integrationEnv, error) {
	mgr, err := modulemanager.New(ctx, restCfg, cfg, opts...)
	if err != nil {
		return nil, fmt.Errorf("creating manager: %w", err)
	}

	go func() {
		if err := mgr.Start(ctx); err != nil {
			fmt.Fprintf(os.Stderr, "manager stopped: %v\n", err)
		}
	}()

	if !mgr.GetCache().WaitForCacheSync(ctx) {
		return nil, fmt.Errorf("cache failed to sync")
	}

	cli := mgr.GetClient()
	ns := support.IntegrationTestNamespace()
	if err := support.EnsureNamespace(ctx, cli, ns); err != nil {
		return nil, fmt.Errorf("creating namespace %q: %w", ns, err)
	}

	return &integrationEnv{
		Client:    cli,
		Namespace: ns,
		Config:    cfg,
	}, nil
}

// deleteAndWait deletes obj (stripping its finalizers first if needed) and
// waits until it is fully gone from the API server. Safe to call when obj
// does not yet exist. Resets ResourceVersion/UID on obj so it can be
// re-created immediately after this call.
func (env *integrationEnv) deleteAndWait(ctx context.Context, t *testing.T, obj client.Object) {
	g := NewWithT(t)
	key := client.ObjectKeyFromObject(obj)

	// Read current state so we have the right resourceVersion / finalizers.
	if err := env.Client.Get(ctx, key, obj); err != nil {
		if apierrors.IsNotFound(err) {
			return
		}
		return // unknown error, best-effort only
	}

	// Strip finalizers so the API server can delete immediately.
	if len(obj.GetFinalizers()) > 0 {
		patch := client.MergeFrom(obj.DeepCopyObject().(client.Object))
		obj.SetFinalizers(nil)
		_ = env.Client.Patch(ctx, obj, patch)
	}
	_ = env.Client.Delete(ctx, obj)

	// Wait for the object to disappear (up to EventuallyTimeout).
	g.Eventually(ctx, k8sm.NotFound(env.Client, obj)).Should(BeTrue(), "waiting for %s to be deleted", key)

	// Clear server-assigned fields so the caller can re-create without conflict.
	obj.SetResourceVersion("")
	obj.SetUID("")
	obj.SetDeletionTimestamp(nil)
	obj.SetFinalizers(nil)
}
