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
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	k8sm "github.com/lburgazzoli/gomega-matchers/pkg/matchers/k8s"
	infraApi "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/api/infrastructure/v1alpha1"
	. "github.com/onsi/gomega"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	moduleconfig "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/pkg/config"
	modulemanager "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/pkg/manager"
	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/pkg/postgres"
	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/test/support"
	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/test/support/cluster"
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

	testCluster, err := cluster.NewExternal()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create integration cluster: %v\n", err)
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
	moduleCfg.OperatorNamespace = support.IntegrationTestNamespace()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	defer func() {
		_ = testCluster.Stop(ctx)
	}()

	cli, err := testCluster.Client()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create integration client: %v\n", err)
		return 1
	}
	if err := cleanupIntegrationFixtures(ctx, cli, support.IntegrationTestNamespace()); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to clean integration fixtures: %v\n", err)
		return 1
	}

	if err := startIntegrationManager(ctx, testCluster.Config(), moduleCfg); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to start integration manager: %v\n", err)
		return 1
	}

	return m.Run()
}

func newIntegrationEnv(t *testing.T) (*integrationEnv, error) {
	t.Helper()

	testCluster, err := cluster.NewExternal()
	if err != nil {
		return nil, fmt.Errorf("creating integration cluster: %w", err)
	}
	cli, err := testCluster.Client()
	if err != nil {
		return nil, fmt.Errorf("creating test client: %w", err)
	}

	moduleCfg, err := moduleconfig.LoadFromFS(nil)
	if err != nil {
		return nil, fmt.Errorf("loading module config: %w", err)
	}
	moduleCfg.OperatorNamespace = support.IntegrationTestNamespace()

	return &integrationEnv{
		Client:    cli,
		Namespace: support.IntegrationTestNamespace(),
		Config:    moduleCfg,
	}, nil
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
		if ctr != nil {
			_ = ctr.Terminate(ctx)
		}
		return nil, fmt.Errorf("getting postgres connection string: %w", err)
	}
	cfg, err := postgres.ConfigFromDSN(connStr)
	if err != nil {
		if ctr != nil {
			_ = ctr.Terminate(ctx)
		}
		return nil, fmt.Errorf("parsing postgres DSN: %w", err)
	}

	return &testDatabase{
		cfg: cfg,
		terminate: func(ctx context.Context) error {
			if ctr == nil {
				return nil
			}

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

func openClaimPool(ctx context.Context, cfg postgres.Config) (*pgxpool.Pool, error) {
	pool, err := pgxpool.New(ctx, cfg.DSN())
	if err != nil {
		return nil, fmt.Errorf("opening claim pool: %w", err)
	}
	return pool, nil
}

func (db *testDatabase) Close(ctx context.Context) error {
	if db == nil || db.terminate == nil {
		return nil
	}
	return db.terminate(ctx)
}

// startIntegrationManager starts the full module manager and ensures the
// integration namespace exists before tests begin.
func startIntegrationManager(
	ctx context.Context,
	restCfg *rest.Config,
	cfg *moduleconfig.Config,
	opts ...modulemanager.Option,
) error {
	mgr, err := modulemanager.New(ctx, restCfg, cfg, opts...)
	if err != nil {
		return fmt.Errorf("creating manager: %w", err)
	}

	go func() {
		if err := mgr.Start(ctx); err != nil {
			fmt.Fprintf(os.Stderr, "manager stopped: %v\n", err)
		}
	}()

	if !mgr.GetCache().WaitForCacheSync(ctx) {
		return fmt.Errorf("cache failed to sync")
	}

	cli := mgr.GetClient()
	ns := support.IntegrationTestNamespace()
	if err := support.EnsureNamespace(ctx, cli, ns); err != nil {
		return fmt.Errorf("creating namespace %q: %w", ns, err)
	}

	return nil
}

func cleanupIntegrationFixtures(ctx context.Context, cli client.Client, namespace string) error {
	for _, item := range []struct {
		list    client.ObjectList
		matches func(client.Object) bool
	}{
		{
			list: &infraApi.DatabaseProviderList{},
			matches: func(obj client.Object) bool {
				return hasAnyPrefix(
					obj.GetName(),
					"provider-",
					"schema-provider-",
					"database-provider-",
				) || obj.GetAnnotations()["db.infrastructure.opendatahub.io/operator-namespace"] == namespace
			},
		},
		{
			list: &infraApi.SchemaClaimList{},
			matches: func(obj client.Object) bool {
				return obj.GetNamespace() == namespace && strings.HasPrefix(obj.GetName(), "sc-")
			},
		},
		{
			list: &infraApi.DatabaseClaimList{},
			matches: func(obj client.Object) bool {
				return obj.GetNamespace() == namespace && strings.HasPrefix(obj.GetName(), "dc-")
			},
		},
		{
			list: &corev1.SecretList{},
			matches: func(obj client.Object) bool {
				return obj.GetNamespace() == namespace && hasAnyPrefix(
					obj.GetName(),
					"provider-secret-",
					"schema-provider-",
					"database-provider-",
					"sc-",
					"dc-",
				)
			},
		},
	} {
		if isClusterScopedList(item.list) {
			if err := cli.List(ctx, item.list); err != nil {
				return err
			}
		} else {
			if err := cli.List(ctx, item.list, client.InNamespace(namespace)); err != nil {
				return err
			}
		}
		if err := forEachListedObject(item.list, func(obj client.Object) error {
			if !item.matches(obj) {
				return nil
			}
			return deleteObjectAndWait(ctx, cli, obj)
		}); err != nil {
			return err
		}
	}

	return nil
}

func deleteObjectAndWait(ctx context.Context, cli client.Client, obj client.Object) error {
	key := client.ObjectKeyFromObject(obj)
	if err := cli.Get(ctx, key, obj); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return err
	}

	if len(obj.GetFinalizers()) > 0 {
		patch := client.MergeFrom(obj.DeepCopyObject().(client.Object))
		obj.SetFinalizers(nil)
		if err := cli.Patch(ctx, obj, patch); err != nil && !apierrors.IsNotFound(err) {
			return err
		}
	}
	if err := cli.Delete(ctx, obj); err != nil && !apierrors.IsNotFound(err) {
		return err
	}

	deadline := time.Now().Add(30 * time.Second)
	for {
		if err := cli.Get(ctx, key, obj); err != nil {
			if apierrors.IsNotFound(err) {
				return nil
			}
			return err
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("timed out waiting for %s to be deleted", key)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(200 * time.Millisecond):
		}
	}
}

func forEachListedObject(list client.ObjectList, fn func(client.Object) error) error {
	switch items := list.(type) {
	case *infraApi.DatabaseProviderList:
		for i := range items.Items {
			if err := fn(&items.Items[i]); err != nil {
				return err
			}
		}
	case *infraApi.SchemaClaimList:
		for i := range items.Items {
			if err := fn(&items.Items[i]); err != nil {
				return err
			}
		}
	case *infraApi.DatabaseClaimList:
		for i := range items.Items {
			if err := fn(&items.Items[i]); err != nil {
				return err
			}
		}
	case *corev1.SecretList:
		for i := range items.Items {
			if err := fn(&items.Items[i]); err != nil {
				return err
			}
		}
	default:
		return fmt.Errorf("unsupported list type %T", list)
	}
	return nil
}

func isClusterScopedList(list client.ObjectList) bool {
	_, ok := list.(*infraApi.DatabaseProviderList)
	return ok
}

func hasAnyPrefix(value string, prefixes ...string) bool {
	for _, prefix := range prefixes {
		if strings.HasPrefix(value, prefix) {
			return true
		}
	}
	return false
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
