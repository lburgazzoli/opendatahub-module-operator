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
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	infraApi "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/api/infrastructure/v1alpha1"
	moduleconfig "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/pkg/config"
	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/pkg/postgres"
)

const (
	OperatorNamespaceAnnotation = "db.infrastructure.opendatahub.io/operator-namespace"

	// EmbeddedInstanceHashAnnotation is the annotation key on the embedded admin
	// Secret that holds an opaque token identifying the credential generation.
	// The same token is written into the StatefulSet pod-template annotation so
	// that Secret recreation triggers a rolling restart.
	EmbeddedInstanceHashAnnotation = "db.infrastructure.opendatahub.io/instance-hash"
)

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
	switch provider.Spec.Type {
	case infraApi.ProviderTypeExternal:
		return loadExternalProviderConfig(ctx, cli, provider)
	case infraApi.ProviderTypeEmbedded:
		return loadEmbeddedProviderConfig(ctx, cli, provider, operatorNamespace)
	default:
		return postgres.Config{}, fmt.Errorf("unsupported provider type %q", provider.Spec.Type)
	}
}

func loadExternalProviderConfig(
	ctx context.Context,
	cli client.Client,
	provider *infraApi.DatabaseProvider,
) (postgres.Config, error) {
	switch ref := provider.Spec.External; {
	case ref == nil:
		return postgres.Config{}, fmt.Errorf("spec.external is required for External providers")
	case ref.ConnectionSecretRef.Namespace == "":
		return postgres.Config{}, fmt.Errorf("spec.external.connectionSecretRef.namespace is required for External providers")
	default:
		secret, err := readSecretRef(ctx, cli, ref.ConnectionSecretRef)
		if err != nil {
			return postgres.Config{}, err
		}

		cfg, err := postgres.ParseSecret(secret.Data)
		if err != nil {
			return postgres.Config{}, fmt.Errorf("parsing admin Secret: %w", err)
		}

		// Default to require when the Secret does not specify pg.sslmode.
		// External providers are assumed to be remote services that support TLS.
		if cfg.SSLMode == "" {
			cfg.SSLMode = postgres.SSLModeRequire
		}

		return cfg, nil
	}
}

func loadEmbeddedProviderConfig(
	ctx context.Context,
	cli client.Client,
	provider *infraApi.DatabaseProvider,
	operatorNamespace string,
) (postgres.Config, error) {
	ref := corev1.SecretReference{
		Namespace: EmbeddedNamespace(provider, operatorNamespace),
		Name:      EmbeddedAdminSecretName(provider.Name),
	}

	secret, err := readSecretRef(ctx, cli, ref)
	if err != nil {
		return postgres.Config{}, err
	}

	cfg, err := postgres.ParseSecret(secret.Data)
	if err != nil {
		return postgres.Config{}, fmt.Errorf("parsing embedded admin Secret: %w", err)
	}
	// Embedded PostgreSQL runs inside the cluster without TLS certificates.
	cfg.SSLMode = postgres.SSLModeDisable
	return cfg, nil
}

func readSecretRef(
	ctx context.Context,
	cli client.Client,
	ref corev1.SecretReference,
) (*corev1.Secret, error) {
	secret := &corev1.Secret{}
	err := cli.Get(ctx, client.ObjectKey{Namespace: ref.Namespace, Name: ref.Name}, secret)
	switch {
	case err == nil:
		return secret, nil
	case apierrors.IsNotFound(err):
		return nil, fmt.Errorf("admin Secret %s/%s not found", ref.Namespace, ref.Name)
	default:
		return nil, fmt.Errorf("reading admin Secret %s/%s: %w", ref.Namespace, ref.Name, err)
	}
}
