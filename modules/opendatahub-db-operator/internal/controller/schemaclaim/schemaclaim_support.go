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

package schemaclaim

import (
	"context"
	"crypto/sha256"
	"fmt"
	"regexp"
	"strings"
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
	// ConditionProvisioned is the primary machine-readable condition consumers
	// gate on (docs/plan.md §5 status contract).
	ConditionProvisioned = "Provisioned"

	// FinalizerName is registered on every SchemaClaim before DDL runs so that
	// deletion always triggers the cleanup action.
	FinalizerName = "infrastructure.opendatahub.io/schemaclaim-cleanup"

	// maxSchemaLen is PostgreSQL's identifier length limit.
	maxSchemaLen = 63
)

var nonIdentRe = regexp.MustCompile(`[^a-z0-9_]`)

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
		return postgres.Config{}, nil, fmt.Errorf("opening pool: %w", err)
	}
	return providerCfg, pool, nil
}

// roleNameFor derives a deterministic PostgreSQL role name from the claim.
func roleNameFor(obj *infraApi.SchemaClaim) string {
	raw := fmt.Sprintf("%s_%s", obj.Namespace, obj.Name)
	safe := nonIdentRe.ReplaceAllString(raw, "_")
	if len(safe) > maxSchemaLen {
		h := fmt.Sprintf("%x", sha256.Sum256([]byte(safe)))[:8]
		safe = safe[:maxSchemaLen-9] + "_" + h
	}
	return safe
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

// ensureSchema creates the schema if needed and reports whether it already
// existed before this reconciliation.
func ensureSchema(ctx context.Context, pool *pgxpool.Pool, schema string) (bool, error) {
	schemaExists, err := postgres.SchemaExists(ctx, pool, schema)
	if err != nil {
		return false, wrapQuickRetry("checking schema existence", err)
	}
	if err := postgres.CreateSchema(ctx, pool, schema); err != nil {
		return false, wrapQuickRetry("creating schema", err)
	}
	return schemaExists, nil
}

// resolveSchema returns the schema name to use for a claim. If spec.schema is
// set it is returned as-is (the CEL marker on the CRD already enforces the
// pattern and length limits). Otherwise the default "${namespace}_${name}" is
// computed, sanitized, and truncated/hashed to stay within PostgreSQL's 63-byte
// identifier limit.
func resolveSchema(namespace, name, specSchema string) string {
	if specSchema != "" {
		return specSchema
	}

	raw := strings.ToLower(namespace + "_" + name)
	safe := nonIdentRe.ReplaceAllString(raw, "_")
	if len(safe) <= maxSchemaLen {
		return safe
	}

	// Truncate to 54 chars + 8-char hex hash of the full original name so it's
	// still unique and deterministic after truncation.
	h := fmt.Sprintf("%x", sha256.Sum256([]byte(safe)))[:8]

	return safe[:maxSchemaLen-9] + "_" + h
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
