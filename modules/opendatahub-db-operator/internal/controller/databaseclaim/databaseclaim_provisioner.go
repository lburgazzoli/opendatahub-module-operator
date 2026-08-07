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

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	infraApi "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/api/infrastructure/v1alpha1"
	dbcontroller "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/pkg/controller"
	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/pkg/controller/claimerrors"
	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/pkg/postgres"
	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/pkg/resources/gvk"
)

// DatabaseProvisioner provisions database-scoped credentials for a DatabaseClaim.
type DatabaseProvisioner struct {
	Client         client.Client
	Claim          *infraApi.DatabaseClaim
	Provider       *infraApi.DatabaseProvider
	Postgres       postgres.Client
	ProviderConfig postgres.Config
}

func (p DatabaseProvisioner) Database() string {
	return dbcontroller.ResolveDatabase(p.Claim.Spec.Database, p.ProviderConfig)
}

// ConnectionStatus returns the desired connection status for the claim.
func (p DatabaseProvisioner) ConnectionStatus() infraApi.DatabaseConnectionStatus {
	return infraApi.DatabaseConnectionStatus{
		SecretRef: corev1.LocalObjectReference{Name: dbcontroller.SecretNameForDatabaseClaim(p.Claim)},
		Host:      p.ProviderConfig.Host,
		Port:      int32(p.ProviderConfig.Port),
		Database:  p.Database(),
	}
}

// Ensure validates the target database, provisions credentials if needed, and
// returns the desired Secret.
func (p DatabaseProvisioner) Ensure(
	ctx context.Context,
) (*corev1.Secret, error) {
	database := p.Database()
	dbExists, err := postgres.DatabaseExists(ctx, p.Postgres, database)
	if err != nil {
		return nil, dbcontroller.WrapQuickRetry("checking database existence", err)
	}
	if !dbExists {
		if p.Claim.Spec.Database == "" {
			return nil, claimerrors.DatabaseNotFound{Database: database}
		}
		if !dbcontroller.ExternalProviderAllows(p.Provider, infraApi.ExternalCapabilityCreateDatabase) {
			return nil, claimerrors.DatabaseCreateNotAllowed{Database: database}
		}
		if err := postgres.CreateDatabase(ctx, p.Postgres, database); err != nil {
			return nil, dbcontroller.WrapQuickRetry("creating database", err)
		}
	}

	role := dbcontroller.RoleNameFor(p.Claim.Namespace, p.Claim.Name)
	roleExists, err := postgres.RoleExists(ctx, p.Postgres, role)
	if err != nil {
		return nil, dbcontroller.WrapQuickRetry("checking role existence", err)
	}

	secret := &corev1.Secret{}
	secret.Name = dbcontroller.SecretNameForDatabaseClaim(p.Claim)
	secret.Namespace = p.Claim.Namespace
	if err := p.Client.Get(ctx, client.ObjectKeyFromObject(secret), secret); client.IgnoreNotFound(err) != nil {
		return nil, fmt.Errorf("checking credentials Secret existence: %w", err)
	}

	secretExists := secret.ResourceVersion != ""

	switch {
	case !roleExists:
		// First provisioning: create the role with a fresh password.
		pw, err := postgres.GeneratePassword(24)
		if err != nil {
			return nil, fmt.Errorf("generating password: %w", err)
		}
		if err := postgres.EnsureRole(ctx, p.Postgres, role, pw); err != nil {
			return nil, dbcontroller.WrapQuickRetry("creating role", err)
		}
		p.buildCredentialsSecret(secret, role, pw)

	case !secretExists:
		// The credentials Secret was lost while the role still exists.
		// Explicitly rotate the password so Secret and role stay in sync.
		// Active connections using the old credentials will break — this is the
		// accepted trade-off; a vault integration would replace this path.
		pw, err := postgres.GeneratePassword(24)
		if err != nil {
			return nil, fmt.Errorf("generating password: %w", err)
		}
		if err := postgres.SetRolePassword(ctx, p.Postgres, role, pw); err != nil {
			return nil, dbcontroller.WrapQuickRetry("rotating role password", err)
		}
		p.buildCredentialsSecret(secret, role, pw)
	}

	switch ok, err := postgres.HasDatabasePrivileges(ctx, p.Postgres, database, role, p.Claim.Spec.Access); {
	case err != nil:
		return nil, dbcontroller.WrapQuickRetry("checking database privileges", err)
	case !ok:
		if err := postgres.GrantDatabasePrivileges(ctx, p.Postgres, database, role, p.Claim.Spec.Access); err != nil {
			return nil, dbcontroller.WrapQuickRetry("granting database privileges", err)
		}
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
		postgres.SecretKeyHost:     []byte(p.ProviderConfig.Host),
		postgres.SecretKeyPort:     []byte(strconv.Itoa(p.ProviderConfig.Port)),
		postgres.SecretKeyUser:     []byte(role),
		postgres.SecretKeyPassword: []byte(password),
		postgres.SecretKeyDatabase: []byte(p.Database()),
	}
	if p.ProviderConfig.SSLMode != "" {
		secret.Data[postgres.SecretKeySSLMode] = []byte(p.ProviderConfig.SSLMode)
	}
	if p.ProviderConfig.SSLRootCert != "" {
		secret.Data[postgres.SecretKeyCA] = []byte(p.ProviderConfig.SSLRootCert)
	}
}
