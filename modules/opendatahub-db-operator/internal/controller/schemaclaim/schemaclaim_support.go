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

	"github.com/jackc/pgx/v5/pgxpool"
	"sigs.k8s.io/controller-runtime/pkg/client"

	dbcontroller "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/pkg/controller"
	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/pkg/postgres"
)

const (
	// ConditionProvisioned is re-exported from pkg/controller so that action
	// code in this package can reference it locally without an extra import.
	ConditionProvisioned = dbcontroller.ConditionProvisioned

	// FinalizerName is registered on every SchemaClaim before DDL runs so that
	// deletion always triggers the cleanup action.
	FinalizerName = "infrastructure.opendatahub.io/schemaclaim-cleanup"

	// maxSchemaLen is PostgreSQL's identifier length limit.
	maxSchemaLen = 63
)

// nonIdentRe strips characters that are not valid in PostgreSQL identifiers.
// Used only by resolveSchema; role-name sanitisation uses RoleNameFor in
// pkg/controller which has its own copy of this pattern.
var nonIdentRe = regexp.MustCompile(`[^a-z0-9_]`)

// ensureSchema creates the schema if needed and reports whether it already
// existed before this reconciliation.
func ensureSchema(ctx context.Context, pool *pgxpool.Pool, schema string) (bool, error) {
	schemaExists, err := postgres.SchemaExists(ctx, pool, schema)
	if err != nil {
		return false, dbcontroller.WrapQuickRetry("checking schema existence", err)
	}
	if err := postgres.CreateSchema(ctx, pool, schema); err != nil {
		return false, dbcontroller.WrapQuickRetry("creating schema", err)
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

// withGrace delegates to the shared pkg/controller.WithGrace free function,
// binding this controller's Recorder and GracePeriod.
func (c *Controller) withGrace(ctx context.Context, obj client.Object, fn func(context.Context) error) error {
	return dbcontroller.WithGrace(ctx, c.Recorder, c.cfg.GracePeriod, obj, fn)
}
