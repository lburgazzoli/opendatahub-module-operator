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
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	infraApi "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/api/infrastructure/v1alpha1"
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

// credentialsSecretExists returns true when the claim's credentials Secret
// already exists in the cluster.
func credentialsSecretExists(ctx context.Context, cli client.Client, obj *infraApi.SchemaClaim) (bool, error) {
	existing := &corev1.Secret{}
	if err := cli.Get(ctx, client.ObjectKey{Namespace: obj.Namespace, Name: obj.Name}, existing); err != nil {
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

// adminSecretRef returns the reference to the provider's admin Secret.
func adminSecretRef(provider *infraApi.DatabaseProvider) corev1.SecretReference {
	if provider.Spec.Type == infraApi.ProviderTypeExternal {
		return provider.Spec.External.ConnectionSecretRef
	}
	// Embedded: admin secret is named "<providerName>-admin" in the operator's namespace
	// (task-08). We read the namespace from an annotation set by the databaseprovider
	// reconciler when it creates the admin secret.
	return corev1.SecretReference{
		Namespace: provider.Annotations["db.infrastructure.opendatahub.io/operator-namespace"],
		Name:      provider.Name + "-admin",
	}
}

// claimConfig builds a postgres.Config for the claim user from the admin
// connection config, replacing the admin credentials with the claim role, its
// generated password, and the resolved schema. This is the self-contained
// credentials struct passed to buildCredentialsSecret.
func claimConfig(adminCfg postgres.Config, role, password, schema string) postgres.Config {
	return postgres.Config{
		Host:     adminCfg.Host,
		Port:     adminCfg.Port,
		User:     role,
		Password: password,
		DBName:   adminCfg.DBName,
		Schema:   schema,
	}
}

// buildCredentialsSecret constructs the SSA-ready credentials Secret from the
// claim-scoped Config (which already carries host/port/user/password/dbname/schema).
func buildCredentialsSecret(
	obj *infraApi.SchemaClaim,
	cfg postgres.Config,
) *corev1.Secret {
	data := map[string]string{
		postgres.SecretKeyHost:     cfg.Host,
		postgres.SecretKeyPort:     fmt.Sprintf("%d", cfg.Port),
		postgres.SecretKeyUser:     cfg.User,
		postgres.SecretKeyPassword: cfg.Password,
		postgres.SecretKeyDatabase: cfg.DBName,
	}
	if cfg.Schema != "" {
		data[postgres.SecretKeySchema] = cfg.Schema
	}
	return &corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Secret",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      obj.Name,
			Namespace: obj.Namespace,
		},
		StringData: data,
	}
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
