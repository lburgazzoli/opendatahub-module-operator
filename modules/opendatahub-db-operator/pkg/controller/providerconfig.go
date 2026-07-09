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
	return providerName
}

func EmbeddedPVCName(providerName string) string {
	return providerName
}

func EmbeddedInitDBConfigMapName(providerName string) string {
	return providerName + "-initdb"
}

func EmbeddedNamespace(provider *infraApi.DatabaseProvider, operatorNamespace string) string {
	if provider != nil && provider.Spec.Embedded != nil && provider.Spec.Embedded.Namespace != "" {
		return provider.Spec.Embedded.Namespace
	}
	return operatorNamespace
}

func EmbeddedServiceHost(provider *infraApi.DatabaseProvider, operatorNamespace string) string {
	return fmt.Sprintf(
		"%s.%s.svc.cluster.local",
		EmbeddedServiceName(provider.Name),
		EmbeddedNamespace(provider, operatorNamespace),
	)
}

func OperatorNamespace(cfg *moduleconfig.Config) string {
	if cfg == nil {
		return ""
	}
	return cfg.OperatorNamespace
}

func LoadProviderConfig(
	ctx context.Context,
	cli client.Client,
	provider *infraApi.DatabaseProvider,
	operatorNamespace string,
) (postgres.Config, error) {
	ref := corev1.SecretReference{}
	if provider.Spec.Type == infraApi.ProviderTypeExternal {
		if provider.Spec.External == nil {
			return postgres.Config{}, fmt.Errorf("spec.external is required for External providers")
		}
		ref = provider.Spec.External.ConnectionSecretRef
	} else {
		ref = corev1.SecretReference{
			Namespace: EmbeddedNamespace(provider, operatorNamespace),
			Name:      EmbeddedAdminSecretName(provider.Name),
		}
	}

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

	parsed := postgres.Config{
		Host:     EmbeddedServiceHost(provider, operatorNamespace),
		Port:     postgres.DefaultPort,
		User:     string(secret.Data[EmbeddedAdminSecretUserKey]),
		Password: string(secret.Data[EmbeddedAdminSecretPasswordKey]),
		DBName:   string(secret.Data[EmbeddedAdminSecretDBKey]),
	}

	if err := parsed.Validate(); err != nil {
		return postgres.Config{}, fmt.Errorf("parsing embedded admin Secret: %w", err)
	}

	return parsed, nil
}
