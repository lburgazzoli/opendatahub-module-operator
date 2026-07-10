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
	"fmt"
	"strconv"

	"github.com/jackc/pgx/v5/pgxpool"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	infraApi "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/api/infrastructure/v1alpha1"
	dbcontroller "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/pkg/controller"
	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/pkg/postgres"
	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/pkg/resources/gvk"
)

// SchemaProvisioner provisions schema-scoped credentials for a SchemaClaim.
type SchemaProvisioner struct {
	Client client.Client
	Claim  *infraApi.SchemaClaim
	Pool   *pgxpool.Pool
	Config postgres.Config
}

// Schema returns the resolved schema name for the claim.
func (p SchemaProvisioner) Schema() string {
	return resolveSchema(p.Claim.Namespace, p.Claim.Name, p.Claim.Spec.Schema)
}

// ConnectionStatus returns the desired connection status for the claim.
func (p SchemaProvisioner) ConnectionStatus(schema string) infraApi.SchemaConnectionStatus {
	return infraApi.SchemaConnectionStatus{
		ConnectionStatus: infraApi.ConnectionStatus{
			SecretRef: corev1.LocalObjectReference{Name: dbcontroller.SecretNameForSchemaClaim(p.Claim)},
			Host:      p.Config.Host,
			Port:      int32(p.Config.Port),
		},
		Database: p.Config.DBName,
		Schema:   schema,
	}
}

// Ensure validates the target schema, provisions credentials if needed, and
// returns the desired Secret.
func (p SchemaProvisioner) Ensure(
	ctx context.Context,
) (*corev1.Secret, error) {
	schema := p.Schema()
	schemaExists, err := ensureSchema(ctx, p.Pool, schema)
	if err != nil {
		return nil, err
	}

	role := roleNameFor(p.Claim)
	roleExists, err := postgres.RoleExists(ctx, p.Pool, role)
	if err != nil {
		return nil, wrapQuickRetry("checking role existence", err)
	}

	secret := &corev1.Secret{}
	secret.Name = dbcontroller.SecretNameForSchemaClaim(p.Claim)
	secret.Namespace = p.Claim.Namespace
	if err := p.Client.Get(ctx, client.ObjectKeyFromObject(secret), secret); client.IgnoreNotFound(err) != nil {
		return nil, fmt.Errorf("checking credentials Secret existence: %w", err)
	}
	secretExists := secret.ResourceVersion != ""

	if !schemaExists || !roleExists || !secretExists {
		pw, err := postgres.GeneratePassword(24)
		if err != nil {
			return nil, fmt.Errorf("generating password: %w", err)
		}
		if err := postgres.CreateRole(ctx, p.Pool, role, pw); err != nil {
			return nil, wrapQuickRetry("creating role", err)
		}

		p.buildCredentialsSecret(secret, role, pw, schema)
	}

	if err := postgres.RevokeSchemaPrivileges(ctx, p.Pool, schema, role); err != nil {
		return nil, wrapQuickRetry("revoking schema privileges", err)
	}
	if err := postgres.GrantSchemaPrivileges(ctx, p.Pool, schema, role, p.Claim.Spec.Access); err != nil {
		return nil, wrapQuickRetry("granting schema privileges", err)
	}
	secret.SetGroupVersionKind(gvk.Secret)

	return secret, nil
}

func (p SchemaProvisioner) buildCredentialsSecret(
	secret *corev1.Secret,
	role string,
	password string,
	schema string,
) {
	secret.SetGroupVersionKind(gvk.Secret)
	secret.Type = corev1.SecretTypeOpaque
	secret.Data = map[string][]byte{
		postgres.SecretKeyHost:     []byte(p.Config.Host),
		postgres.SecretKeyPort:     []byte(strconv.Itoa(p.Config.Port)),
		postgres.SecretKeyUser:     []byte(role),
		postgres.SecretKeyPassword: []byte(password),
		postgres.SecretKeyDatabase: []byte(p.Config.DBName),
		postgres.SecretKeySchema:   []byte(schema),
	}
}
