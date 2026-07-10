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

// ErrDatabaseNotFound reports that the target database is missing.
type ErrDatabaseNotFound struct {
	Database string
}

func (e ErrDatabaseNotFound) Error() string {
	return fmt.Sprintf("database %q not found", e.Database)
}

// DatabaseProvisioner provisions database-scoped credentials for a DatabaseClaim.
type DatabaseProvisioner struct {
	Client client.Client
	Claim  *infraApi.DatabaseClaim
	Pool   *pgxpool.Pool
	Config postgres.Config
}

// ConnectionStatus returns the desired connection status for the claim.
func (p DatabaseProvisioner) ConnectionStatus() infraApi.DatabaseConnectionStatus {
	return infraApi.DatabaseConnectionStatus{
		SecretRef: corev1.LocalObjectReference{Name: dbcontroller.SecretNameForDatabaseClaim(p.Claim)},
		Host:      p.Config.Host,
		Port:      int32(p.Config.Port),
		Database:  p.Claim.Spec.Database,
	}
}

// Ensure validates the target database, provisions credentials if needed, and
// returns the desired Secret.
func (p DatabaseProvisioner) Ensure(
	ctx context.Context,
) (*corev1.Secret, error) {
	dbExists, err := postgres.DatabaseExists(ctx, p.Pool, p.Claim.Spec.Database)
	if err != nil {
		return nil, wrapQuickRetry("checking database existence", err)
	}
	if !dbExists {
		return nil, ErrDatabaseNotFound{Database: p.Claim.Spec.Database}
	}

	role := roleNameFor(p.Claim)
	roleExists, err := postgres.RoleExists(ctx, p.Pool, role)
	if err != nil {
		return nil, wrapQuickRetry("checking role existence", err)
	}

	secret := &corev1.Secret{}
	secret.Name = dbcontroller.SecretNameForDatabaseClaim(p.Claim)
	secret.Namespace = p.Claim.Namespace
	if err := p.Client.Get(ctx, client.ObjectKeyFromObject(secret), secret); client.IgnoreNotFound(err) != nil {
		return nil, fmt.Errorf("checking credentials Secret existence: %w", err)
	}

	secretExists := secret.ResourceVersion != ""

	if !roleExists || !secretExists {
		pw, err := postgres.GeneratePassword(24)
		if err != nil {
			return nil, fmt.Errorf("generating password: %w", err)
		}
		if err := postgres.CreateRole(ctx, p.Pool, role, pw); err != nil {
			return nil, wrapQuickRetry("creating role", err)
		}

		p.buildCredentialsSecret(secret, role, pw)
	}

	if err := postgres.RevokeDatabasePrivileges(ctx, p.Pool, p.Claim.Spec.Database, role); err != nil {
		return nil, wrapQuickRetry("revoking database privileges", err)
	}
	if err := postgres.GrantDatabasePrivileges(ctx, p.Pool, p.Claim.Spec.Database, role, p.Claim.Spec.Access); err != nil {
		return nil, wrapQuickRetry("granting database privileges", err)
	}
	secret.SetGroupVersionKind(gvk.Secret)

	return secret, nil
}

func (p DatabaseProvisioner) buildCredentialsSecret(
	secret *corev1.Secret,
	role string,
	password string,
) {
	secret.SetGroupVersionKind(gvk.Secret)
	secret.Type = corev1.SecretTypeOpaque
	secret.Data = map[string][]byte{
		postgres.SecretKeyHost:     []byte(p.Config.Host),
		postgres.SecretKeyPort:     []byte(strconv.Itoa(p.Config.Port)),
		postgres.SecretKeyUser:     []byte(role),
		postgres.SecretKeyPassword: []byte(password),
		postgres.SecretKeyDatabase: []byte(p.Claim.Spec.Database),
	}
}
