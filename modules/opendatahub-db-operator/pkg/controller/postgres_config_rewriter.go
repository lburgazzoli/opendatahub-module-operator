package controller

import (
	"context"

	"sigs.k8s.io/controller-runtime/pkg/client"

	infraApi "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/api/infrastructure/v1alpha1"
	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/pkg/postgres"
)

type PostgresConnectionConfigResolver interface {
	Resolve(
		ctx context.Context,
		provider *infraApi.DatabaseProvider,
		cfg postgres.Config,
	) (postgres.Config, error)
}

type PostgresConnectionConfigResolveFunc func(
	ctx context.Context,
	provider *infraApi.DatabaseProvider,
	cfg postgres.Config,
) (postgres.Config, error)

func DefaultPostgresConnectionConfigResolver() PostgresConnectionConfigResolver {
	return PostgresConnectionConfigResolveFunc(func(
		_ context.Context,
		_ *infraApi.DatabaseProvider,
		cfg postgres.Config,
	) (postgres.Config, error) {
		return cfg, nil
	})
}

func (fn PostgresConnectionConfigResolveFunc) Resolve(
	ctx context.Context,
	provider *infraApi.DatabaseProvider,
	cfg postgres.Config,
) (postgres.Config, error) {
	if fn == nil {
		return cfg, nil
	}

	return fn(ctx, provider, cfg)
}

type ResolvedProviderConfig struct {
	Published  postgres.Config
	Connection postgres.Config
}

func ResolveProviderConfig(
	ctx context.Context,
	cli client.Client,
	provider *infraApi.DatabaseProvider,
	operatorNamespace string,
	resolver PostgresConnectionConfigResolver,
) (ResolvedProviderConfig, error) {
	cfg, err := LoadProviderConfig(ctx, cli, provider, operatorNamespace)
	if err != nil {
		return ResolvedProviderConfig{}, err
	}

	resolved := ResolvedProviderConfig{
		Published:  cfg,
		Connection: cfg,
	}
	if resolver == nil {
		return resolved, nil
	}

	connectionCfg, err := resolver.Resolve(ctx, provider, cfg)
	if err != nil {
		return ResolvedProviderConfig{}, err
	}

	resolved.Connection = connectionCfg
	return resolved, nil
}
