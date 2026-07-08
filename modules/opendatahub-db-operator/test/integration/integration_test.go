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
	"net/http"
	"os"
	"testing"
	"time"

	. "github.com/onsi/gomega"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	infraApi "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/api/infrastructure/v1alpha1"
	moduleconfig "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/pkg/config"
	modulemanager "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/pkg/manager"
	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/pkg/postgres"
	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/test/support"
)

// Package-level shared state initialised once in TestMain.
var (
	sharedClient   client.Client
	sharedProvider *infraApi.DatabaseProvider // External provider pointing at the shared postgres
	sharedPgCfg    postgres.Config            // direct connection info for assertion helpers
	sharedCfg      *moduleconfig.Config
)

func TestMain(m *testing.M) {
	ctx := context.Background()
	g := NewGomegaWithT(&testing.T{})

	gomegaCfg, err := support.LoadGomegaConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load test config: %v\n", err)
		os.Exit(1)
	}
	SetDefaultEventuallyTimeout(gomegaCfg.EventuallyTimeout)
	SetDefaultEventuallyPollingInterval(gomegaCfg.EventuallyPollingInterval)
	SetDefaultConsistentlyPollingInterval(gomegaCfg.ConsistentlyPollingInterval)

	ctrl.SetLogger(zap.New(zap.UseDevMode(true)))

	restCfg, err := config.GetConfig()
	g.Expect(err).NotTo(HaveOccurred())

	sharedCfg, err = moduleconfig.LoadFromFS(nil)
	g.Expect(err).NotTo(HaveOccurred())
	sharedCfg.Controller.LeaderElection.Enabled = false
	sharedCfg.Controller.Metrics.BindAddress = "0"
	sharedCfg.Controller.Health.BindAddress = "0"
	sharedCfg.Controller.Pprof.BindAddress = "0"
	sharedCfg.ApplicationsNamespace = support.IntegrationTestNamespace()

	// ------------------------------------------------------------------
	// Shared postgres (testcontainers). Skipped gracefully if Docker is
	// unavailable -- only the claim tests that depend on it will fail.
	// ------------------------------------------------------------------
	pgCfg, cleanupPg := startSharedPostgres(ctx)
	defer cleanupPg()

	// ------------------------------------------------------------------
	// Shared claim manager -- registers only SchemaClaim + DatabaseClaim
	// reconcilers, not the full module manager. SkipNameValidation so
	// sequential tests can reuse the same controller name across restarts.
	// ------------------------------------------------------------------
	sharedClient, sharedProvider = setupClaimsManager(ctx, restCfg, sharedCfg, pgCfg)
	sharedPgCfg = pgCfg

	os.Exit(m.Run())
}

// startSharedPostgres spins up a postgres:16 container shared across all claim
// tests. Returns the Config and a cleanup func.
func startSharedPostgres(ctx context.Context) (postgres.Config, func()) {
	ctr, err := tcpostgres.Run(ctx, "postgres:16",
		tcpostgres.WithDatabase("testdb"),
		tcpostgres.WithUsername("admin"),
		tcpostgres.WithPassword("adminpass"),
		tcpostgres.BasicWaitStrategies(),
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "WARNING: cannot start Postgres container (Docker not available?): %v\n", err)
		return postgres.Config{}, func() {}
	}
	connStr, err := ctr.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		_ = ctr.Terminate(ctx)
		fmt.Fprintf(os.Stderr, "getting connection string: %v\n", err)
		return postgres.Config{}, func() {}
	}
	cfg, err := postgres.ConfigFromDSN(connStr)
	if err != nil {
		_ = ctr.Terminate(ctx)
		fmt.Fprintf(os.Stderr, "parsing DSN: %v\n", err)
		return postgres.Config{}, func() {}
	}
	return cfg, func() { _ = ctr.Terminate(ctx) }
}

// setupClaimsManager starts the full module manager (all reconcilers), creates
// the shared External DatabaseProvider pointing at the testcontainers postgres,
// and returns the client and provider for use by claim tests.
func setupClaimsManager(
	ctx context.Context,
	restCfg *rest.Config,
	cfg *moduleconfig.Config,
	pgCfg postgres.Config,
) (client.Client, *infraApi.DatabaseProvider) {
	if pgCfg.Host == "" {
		return nil, nil // docker unavailable -- claim tests will skip
	}

	mgr, err := modulemanager.New(ctx, restCfg, cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "creating manager: %v\n", err)
		return nil, nil
	}

	go func() {
		if err := mgr.Start(ctx); err != nil {
			fmt.Fprintf(os.Stderr, "manager stopped: %v\n", err)
		}
	}()

	if !mgr.GetCache().WaitForCacheSync(ctx) {
		fmt.Fprintf(os.Stderr, "cache failed to sync\n")
		return nil, nil
	}

	cli := mgr.GetClient()
	ns := support.IntegrationTestNamespace()
	if err := support.EnsureNamespace(ctx, cli, ns); err != nil {
		fmt.Fprintf(os.Stderr, "creating namespace: %v\n", err)
		return cli, nil
	}

	// Create the shared admin Secret and External DatabaseProvider.
	adminSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "shared-admin", Namespace: ns},
		StringData: map[string]string{
			postgres.SecretKeyHost:     pgCfg.Host,
			postgres.SecretKeyPort:     fmt.Sprintf("%d", pgCfg.Port),
			postgres.SecretKeyUser:     pgCfg.User,
			postgres.SecretKeyPassword: pgCfg.Password,
			postgres.SecretKeyDatabase: pgCfg.DBName,
		},
	}
	_ = cli.Delete(ctx, adminSecret) // delete stale from previous run
	if err := cli.Create(ctx, adminSecret); err != nil {
		fmt.Fprintf(os.Stderr, "creating admin secret: %v\n", err)
		return cli, nil
	}

	provider := &infraApi.DatabaseProvider{
		ObjectMeta: metav1.ObjectMeta{
			Name: "shared-external",
			Annotations: map[string]string{
				"db.infrastructure.opendatahub.io/operator-namespace": ns,
			},
		},
		Spec: infraApi.DatabaseProviderSpec{
			Type: infraApi.ProviderTypeExternal,
			External: &infraApi.ExternalProviderSpec{
				ConnectionSecretRef: corev1.SecretReference{
					Name:      adminSecret.Name,
					Namespace: ns,
				},
			},
		},
	}
	_ = cli.Delete(ctx, provider)
	if err := cli.Create(ctx, provider); err != nil {
		fmt.Fprintf(os.Stderr, "creating shared provider: %v\n", err)
		return cli, nil
	}

	return cli, provider
}

// deleteAndWait deletes obj (stripping its finalizers first if needed) and
// waits until it is fully gone from the API server. Safe to call when obj
// does not yet exist. Resets ResourceVersion/UID on obj so it can be
// re-created immediately after this call.
func deleteAndWait(ctx context.Context, cli client.Client, obj client.Object) {
	g := NewWithT(&testing.T{})
	key := client.ObjectKeyFromObject(obj)

	// Read current state so we have the right resourceVersion / finalizers.
	if err := cli.Get(ctx, key, obj); err != nil {
		if apierrors.IsNotFound(err) {
			return
		}
		return // unknown error, best-effort only
	}

	// Strip finalizers so the API server can delete immediately.
	if len(obj.GetFinalizers()) > 0 {
		patch := client.MergeFrom(obj.DeepCopyObject().(client.Object))
		obj.SetFinalizers(nil)
		_ = cli.Patch(ctx, obj, patch)
	}
	_ = cli.Delete(ctx, obj)

	// Wait for the object to disappear (up to EventuallyTimeout).
	g.Eventually(func() bool {
		err := cli.Get(ctx, key, obj)
		return apierrors.IsNotFound(err)
	}).Should(BeTrue(), "waiting for %s to be deleted", key)

	// Clear server-assigned fields so the caller can re-create without conflict.
	obj.SetResourceVersion("")
	obj.SetUID("")
	obj.SetDeletionTimestamp(nil)
	obj.SetFinalizers(nil)
}

// ---- legacy smoke test (task-01) ----------------------------------------

// TestManagerStartup verifies the full manager reaches a healthy state.
// Skipped when the shared manager (started in TestMain for claim tests) is
// already running -- starting a second manager would conflict on controller names.
func TestManagerStartup(t *testing.T) {
	if sharedClient != nil {
		t.Skip("shared manager already running (skipping duplicate startup test)")
	}
	g := NewWithT(t)

	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()

	restCfg, err := config.GetConfig()
	g.Expect(err).NotTo(HaveOccurred())

	moduleCfg, err := moduleconfig.LoadFromFS(nil)
	g.Expect(err).NotTo(HaveOccurred())
	moduleCfg.ApplicationsNamespace = support.IntegrationTestNamespace()
	moduleCfg.Controller.Metrics.BindAddress = "0"
	moduleCfg.Controller.Health.BindAddress = "127.0.0.1:18081"
	moduleCfg.Controller.LeaderElection.Enabled = false
	moduleCfg.Controller.Pprof.BindAddress = "0"

	mgr, err := modulemanager.New(ctx, restCfg, moduleCfg)
	g.Expect(err).NotTo(HaveOccurred())

	mgrDone := make(chan error, 1)
	go func() { mgrDone <- mgr.Start(ctx) }()
	g.Expect(mgr.GetCache().WaitForCacheSync(ctx)).To(BeTrue())

	g.Eventually(func() (*http.Response, error) {
		return http.Get("http://127.0.0.1:18081/healthz")
	}).Should(HaveHTTPStatus(http.StatusOK))
	g.Eventually(func() (*http.Response, error) {
		return http.Get("http://127.0.0.1:18081/readyz")
	}).Should(HaveHTTPStatus(http.StatusOK))

	cancel()
	g.Eventually(mgrDone).Should(Receive())
}
