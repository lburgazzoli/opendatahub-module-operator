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

package controller

import (
	"context"
	"crypto/sha256"
	"fmt"
	"regexp"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/events"
	"sigs.k8s.io/controller-runtime/pkg/client"

	infraApi "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/api/infrastructure/v1alpha1"
	moduleconfig "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/pkg/config"
	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/pkg/postgres"
)

const (
	// maxRoleNameLen is PostgreSQL's identifier length limit.
	maxRoleNameLen = 63
)

var nonIdentRe = regexp.MustCompile(`[^a-z0-9_]`)

// RoleNameFor derives a deterministic, PostgreSQL-safe role name from a claim's
// namespace and name. Extracted from the identical roleNameFor helpers that
// previously lived independently in the schemaclaim and databaseclaim packages.
func RoleNameFor(namespace, name string) string {
	raw := fmt.Sprintf("%s_%s", namespace, name)
	safe := nonIdentRe.ReplaceAllString(raw, "_")
	if len(safe) > maxRoleNameLen {
		h := fmt.Sprintf("%x", sha256.Sum256([]byte(safe)))[:8]
		safe = safe[:maxRoleNameLen-9] + "_" + h
	}
	return safe
}

// NewClient loads the provider's admin connection config and opens a postgres.Client.
// Extracted from the identical openPool helpers in schemaclaim and databaseclaim.
func NewClient(
	ctx context.Context,
	cli client.Client,
	provider *infraApi.DatabaseProvider,
	cfg *moduleconfig.Config,
	factory postgres.ClientFactory,
) (postgres.Client, postgres.Config, error) {
	if factory == nil {
		factory = postgres.DefaultClientFactory
	}

	providerCfg, err := LoadProviderConfig(ctx, cli, provider, cfg.OperatorNamespace)
	if err != nil {
		return nil, postgres.Config{}, err
	}
	dbClient, err := factory(ctx, providerCfg)
	if err != nil {
		return nil, postgres.Config{}, fmt.Errorf(
			"opening postgres client: %w",
			postgres.SanitizeError(err, providerCfg.Password),
		)
	}
	return dbClient, providerCfg, nil
}

// WrapQuickRetry wraps err with an operation label and upgrades it to a
// quick-retry signal when it's a connection-refused error. Extracted from the
// identical wrapQuickRetry helpers in schemaclaim and databaseclaim.
func WrapQuickRetry(op string, err error) error {
	if err == nil {
		return nil
	}
	wrapped := fmt.Errorf("%s: %w", op, err)
	if retryErr := StopWithQuickRetryIfConnectionRefused(wrapped); retryErr != nil {
		return retryErr
	}
	return wrapped
}

// WithGrace wraps fn in the controller's grace-period cleanup policy.
// If fn succeeds it returns nil. If fn fails and the object's deletion
// timestamp is older than gracePeriod, a Warning event is emitted and nil is
// returned (allowing the finalizer to be removed). Otherwise the error is
// returned so the cleanup is retried on the next reconcile.
//
// Extracted from the identical withGrace methods on each claim Controller.
func WithGrace(
	ctx context.Context,
	recorder events.EventRecorder,
	gracePeriod time.Duration,
	obj client.Object,
	fn func(context.Context) error,
) error {
	err := fn(ctx)
	switch {
	case err == nil:
		return nil
	case obj.GetDeletionTimestamp() != nil && time.Since(obj.GetDeletionTimestamp().Time) > gracePeriod:
		recorder.Eventf(
			obj,
			nil,
			corev1.EventTypeWarning,
			"CleanupGracePeriodExpired",
			"FinalizerCleanup",
			"Cleanup failed after %s grace period; allowing finalizer removal: %v",
			gracePeriod,
			err,
		)
		return nil
	default:
		return fmt.Errorf("cleanup failed (will retry within grace period): %w", err)
	}
}
