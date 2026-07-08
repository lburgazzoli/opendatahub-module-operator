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
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	infraApi "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/api/infrastructure/v1alpha1"
	moduleconfig "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/pkg/config"
	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/pkg/postgres"
)

const (
	OperatorNamespaceAnnotation = "db.infrastructure.opendatahub.io/operator-namespace"

	EmbeddedAdminSecretUserKey     = "POSTGRES_USER"
	EmbeddedAdminSecretPasswordKey = "POSTGRES_PASSWORD"
	EmbeddedAdminSecretDBKey       = "POSTGRES_DB"
)

func EmbeddedAdminSecretName(providerName string) string {
	return providerName + "-admin"
}

func EmbeddedServiceName(providerName string) string {
	return providerName + "-postgres"
}

func EmbeddedPVCName(providerName string) string {
	return providerName + "-postgres"
}

func EmbeddedInitDBConfigMapName(providerName string) string {
	return providerName + "-postgres-initdb"
}

func EmbeddedServiceHost(providerName string, cfg *moduleconfig.Config) string {
	return fmt.Sprintf("%s.%s.svc", EmbeddedServiceName(providerName), OperatorNamespace(cfg))
}

func OperatorNamespace(cfg *moduleconfig.Config) string {
	if cfg == nil {
		return ""
	}
	if cfg.OperatorNamespace != "" {
		return cfg.OperatorNamespace
	}
	return cfg.ApplicationsNamespace
}

func ProviderAdminSecretRef(provider *infraApi.DatabaseProvider, cfg *moduleconfig.Config) corev1.SecretReference {
	if provider.Spec.Type == infraApi.ProviderTypeExternal {
		return provider.Spec.External.ConnectionSecretRef
	}
	return corev1.SecretReference{
		Namespace: OperatorNamespace(cfg),
		Name:      EmbeddedAdminSecretName(provider.Name),
	}
}

func LoadProviderConfig(
	ctx context.Context,
	cli client.Client,
	provider *infraApi.DatabaseProvider,
	cfg *moduleconfig.Config,
) (postgres.Config, error) {
	ref := ProviderAdminSecretRef(provider, cfg)
	secret := &corev1.Secret{}
	if err := cli.Get(ctx, client.ObjectKey{Namespace: ref.Namespace, Name: ref.Name}, secret); err != nil {
		return postgres.Config{}, fmt.Errorf("reading admin Secret %s/%s: %w", ref.Namespace, ref.Name, err)
	}

	if provider.Spec.Type == infraApi.ProviderTypeExternal {
		parsed, err := postgres.ParseSecret(secret.Data)
		if err != nil {
			return postgres.Config{}, fmt.Errorf("parsing admin Secret: %w", err)
		}
		return parsed, nil
	}

	user := string(secret.Data[EmbeddedAdminSecretUserKey])
	password := string(secret.Data[EmbeddedAdminSecretPasswordKey])
	database := string(secret.Data[EmbeddedAdminSecretDBKey])
	parsed := postgres.Config{
		Host:     EmbeddedServiceHost(provider.Name, cfg),
		Port:     postgres.DefaultPort,
		User:     user,
		Password: password,
		DBName:   database,
	}
	if err := parsed.Validate(); err != nil {
		return postgres.Config{}, fmt.Errorf("parsing embedded admin Secret: %w", err)
	}

	return parsed, nil
}
