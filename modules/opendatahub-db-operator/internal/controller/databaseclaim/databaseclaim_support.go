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
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	infraApi "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/api/infrastructure/v1alpha1"
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

// credentialsSecretExists returns true when the claim's credentials Secret exists.
func credentialsSecretExists(ctx context.Context, cli client.Client, obj *infraApi.DatabaseClaim) (bool, error) {
	secret := &corev1.Secret{}
	if err := cli.Get(ctx, client.ObjectKey{Namespace: obj.Namespace, Name: obj.Name}, secret); err != nil {
		if apierrors.IsNotFound(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// openPool reads the provider's admin Secret and opens a pgxpool.Pool.
func openPool(
	ctx context.Context,
	cli client.Client,
	provider *infraApi.DatabaseProvider,
) (postgres.Config, *pgxpool.Pool, error) {
	ref := adminSecretRef(provider)
	secret := &corev1.Secret{}
	if err := cli.Get(ctx, client.ObjectKey{Namespace: ref.Namespace, Name: ref.Name}, secret); err != nil {
		return postgres.Config{}, nil, fmt.Errorf("reading admin Secret %s/%s: %w", ref.Namespace, ref.Name, err)
	}
	cfg, err := postgres.ParseSecret(secret.Data)
	if err != nil {
		return postgres.Config{}, nil, fmt.Errorf("parsing admin Secret: %w", err)
	}
	pool, err := pgxpool.New(ctx, cfg.DSN())
	if err != nil {
		return postgres.Config{}, nil, fmt.Errorf("opening pool: %w", err)
	}
	return cfg, pool, nil
}

// adminSecretRef returns the reference to the provider's admin Secret.
func adminSecretRef(provider *infraApi.DatabaseProvider) corev1.SecretReference {
	if provider.Spec.Type == infraApi.ProviderTypeExternal {
		return provider.Spec.External.ConnectionSecretRef
	}
	return corev1.SecretReference{
		Namespace: provider.Annotations["db.infrastructure.opendatahub.io/operator-namespace"],
		Name:      provider.Name + "-admin",
	}
}

// claimConfig builds a postgres.Config for the claim user from the admin config.
func claimConfig(adminCfg postgres.Config, database, role, password string) postgres.Config {
	return postgres.Config{
		Host:     adminCfg.Host,
		Port:     adminCfg.Port,
		User:     role,
		Password: password,
		DBName:   database,
		// DatabaseClaim has no schema -- Schema field left empty
	}
}

// buildCredentialsSecret constructs the SSA-ready credentials Secret.
func buildCredentialsSecret(obj *infraApi.DatabaseClaim, cfg postgres.Config) *corev1.Secret {
	return &corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Secret",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      obj.Name,
			Namespace: obj.Namespace,
		},
		StringData: map[string]string{
			postgres.SecretKeyHost:     cfg.Host,
			postgres.SecretKeyPort:     fmt.Sprintf("%d", cfg.Port),
			postgres.SecretKeyUser:     cfg.User,
			postgres.SecretKeyPassword: cfg.Password,
			postgres.SecretKeyDatabase: cfg.DBName,
		},
	}
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
