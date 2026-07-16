package db

import (
	"context"
	"fmt"

	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"

	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/pkg/postgres"
)

type Instance struct {
	cfg       postgres.Config
	client    postgres.Client
	terminate func(context.Context) error
}

func Start(ctx context.Context) (*Instance, error) {
	ctr, err := tcpostgres.Run(ctx, "postgres:16",
		tcpostgres.WithDatabase("testdb"),
		tcpostgres.WithUsername("admin"),
		tcpostgres.WithPassword("adminpass"),
		tcpostgres.BasicWaitStrategies(),
	)
	if err != nil {
		return nil, fmt.Errorf("starting postgres container: %w", err)
	}

	connStr, err := ctr.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		if ctr != nil {
			_ = ctr.Terminate(ctx)
		}

		return nil, fmt.Errorf("getting postgres connection string: %w", err)
	}

	cfg, err := postgres.ConfigFromDSN(connStr)
	if err != nil {
		if ctr != nil {
			_ = ctr.Terminate(ctx)
		}

		return nil, fmt.Errorf("parsing postgres DSN: %w", err)
	}

	client, err := postgres.NewClient(ctx, cfg)
	if err != nil {
		if ctr != nil {
			_ = ctr.Terminate(ctx)
		}

		return nil, fmt.Errorf("opening postgres client: %w", err)
	}

	return &Instance{
		cfg:    cfg,
		client: client,
		terminate: func(ctx context.Context) error {
			if ctr == nil {
				return nil
			}

			return ctr.Terminate(ctx)
		},
	}, nil
}

func (db *Instance) Config() postgres.Config {
	if db == nil {
		return postgres.Config{}
	}

	return db.cfg
}

func (db *Instance) Client() postgres.Client {
	if db == nil {
		return nil
	}

	return db.client
}

func (db *Instance) Close(ctx context.Context) error {
	if db == nil {
		return nil
	}

	if db.client != nil {
		db.client.Close()
		db.client = nil
	}

	if db.terminate == nil {
		return nil
	}

	return db.terminate(ctx)
}
