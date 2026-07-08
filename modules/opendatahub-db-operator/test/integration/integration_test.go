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

	k8sm "github.com/lburgazzoli/gomega-matchers/pkg/matchers/k8s"
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

const sharedProviderName = "shared-external"

type integrationEnv struct {
	Client       client.Client
	Namespace    string
	ProviderName string
	PGCfg        postgres.Config
	Config       *moduleconfig.Config
}

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

	return m.Run()
}

func newIntegrationEnv(t *testing.T) *integrationEnv {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	ctrl.SetLogger(zap.New(zap.UseDevMode(true)))

	g := NewWithT(t)

	restCfg, err := config.GetConfig()
	g.Expect(err).NotTo(HaveOccurred())

	moduleCfg, err := moduleconfig.LoadFromFS(nil)
	g.Expect(err).NotTo(HaveOccurred())
	moduleCfg.Controller.LeaderElection.Enabled = false
	moduleCfg.Controller.Metrics.BindAddress = "0"
	moduleCfg.Controller.Health.BindAddress = "0"
	moduleCfg.Controller.Pprof.BindAddress = "0"
	moduleCfg.ApplicationsNamespace = support.IntegrationTestNamespace()

	pgCfg, cleanupPg, err := startSharedPostgres(ctx)
	g.Expect(err).NotTo(HaveOccurred())
	t.Cleanup(cleanupPg)

	env, err := setupIntegrationEnv(ctx, restCfg, moduleCfg, pgCfg)
	g.Expect(err).NotTo(HaveOccurred())
	return env
}

// startSharedPostgres spins up a postgres:16 container shared across all claim
// tests. Returns the Config and a cleanup func. Docker/testcontainers are a
// required prerequisite for this suite: if the shared database cannot be
// established, TestMain fails fast rather than leaving nil globals for
// individual tests to skip around.
func startSharedPostgres(ctx context.Context) (postgres.Config, func(), error) {
	ctr, err := tcpostgres.Run(ctx, "postgres:16",
		tcpostgres.WithDatabase("testdb"),
		tcpostgres.WithUsername("admin"),
		tcpostgres.WithPassword("adminpass"),
		tcpostgres.BasicWaitStrategies(),
	)
	if err != nil {
		return postgres.Config{}, func() {}, fmt.Errorf("starting shared postgres container: %w", err)
	}
	connStr, err := ctr.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		_ = ctr.Terminate(ctx)
		return postgres.Config{}, func() {}, fmt.Errorf("getting shared postgres connection string: %w", err)
	}
	cfg, err := postgres.ConfigFromDSN(connStr)
	if err != nil {
		_ = ctr.Terminate(ctx)
		return postgres.Config{}, func() {}, fmt.Errorf("parsing shared postgres DSN: %w", err)
	}
	return cfg, func() { _ = ctr.Terminate(ctx) }, nil
}

// setupIntegrationEnv starts the full module manager, creates the shared
// External DatabaseProvider pointing at the testcontainers postgres, and
// returns the immutable-ish suite environment used by individual test groups.
func setupIntegrationEnv(
	ctx context.Context,
	restCfg *rest.Config,
	cfg *moduleconfig.Config,
	pgCfg postgres.Config,
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
		return nil, fmt.Errorf("creating shared admin secret: %w", err)
	}

	provider := &infraApi.DatabaseProvider{
		ObjectMeta: metav1.ObjectMeta{
			Name: sharedProviderName,
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
		return nil, fmt.Errorf("creating shared provider: %w", err)
	}

	return &integrationEnv{
		Client:       cli,
		Namespace:    ns,
		ProviderName: provider.Name,
		PGCfg:        pgCfg,
		Config:       cfg,
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
