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

package databaseclaim

import (
	"context"
	"crypto/sha256"
	"fmt"
	"regexp"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	infraApi "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/api/infrastructure/v1alpha1"
	moduleconfig "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/pkg/config"
	dbcontroller "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/pkg/controller"
	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/pkg/postgres"
)

const (
	ConditionProvisioned = "Provisioned"
	FinalizerName        = "infrastructure.opendatahub.io/databaseclaim-cleanup"
	maxRoleLen           = 63
)

var nonIdentRe = regexp.MustCompile(`[^a-z0-9_]`)

// roleNameFor derives a deterministic PostgreSQL role name from the claim.
func roleNameFor(obj *infraApi.DatabaseClaim) string {
	raw := fmt.Sprintf("%s_%s", obj.Namespace, obj.Name)
	safe := nonIdentRe.ReplaceAllString(raw, "_")
	if len(safe) > maxRoleLen {
		h := fmt.Sprintf("%x", sha256.Sum256([]byte(safe)))[:8]
		safe = safe[:maxRoleLen-9] + "_" + h
	}
	return safe
}

// openPool reads the provider's admin Secret and opens a pgxpool.Pool.
func openPool(
	ctx context.Context,
	cli client.Client,
	provider *infraApi.DatabaseProvider,
	cfg *moduleconfig.Config,
) (postgres.Config, *pgxpool.Pool, error) {
	providerCfg, err := dbcontroller.LoadProviderConfig(ctx, cli, provider, cfg.OperatorNamespace)
	if err != nil {
		return postgres.Config{}, nil, err
	}
	pool, err := pgxpool.New(ctx, providerCfg.DSN())
	if err != nil {
		return postgres.Config{}, nil, fmt.Errorf(
			"opening pool: %w",
			postgres.SanitizeError(err, providerCfg.Password),
		)
	}
	return providerCfg, pool, nil
}

func wrapQuickRetry(op string, err error) error {
	if err == nil {
		return nil
	}
	wrapped := fmt.Errorf("%s: %w", op, err)
	if retryErr := dbcontroller.StopWithQuickRetryIfConnectionRefused(wrapped); retryErr != nil {
		return retryErr
	}
	return wrapped
}

// withGrace wraps a cleanup function with the controller's grace-period policy.
// Calls pkg/controller.CleanupWithGrace with this controller's recorder and
// cfg.GracePeriod so call sites only supply the object and the cleanup func.
func (c *Controller) withGrace(ctx context.Context, obj client.Object, fn func(context.Context) error) error {
	err := fn(ctx)
	switch {
	case err == nil:
		return nil
	case obj.GetDeletionTimestamp() != nil && time.Since(obj.GetDeletionTimestamp().Time) > c.cfg.GracePeriod:
		c.Recorder.Eventf(
			obj,
			nil,
			corev1.EventTypeWarning,
			"CleanupGracePeriodExpired",
			"FinalizerCleanup",
			"Cleanup failed after %s grace period; allowing finalizer removal: %v",
			c.cfg.GracePeriod,
			err,
		)
		return nil
	default:
		return fmt.Errorf("cleanup failed (will retry within grace period): %w", err)
	}
}
