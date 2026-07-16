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

package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Client owns a PostgreSQL connection and is the main way to interact with the database.
type Client interface {
	Config() Config
	Close()
	Ping(ctx context.Context) error
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) (pgx.Row, error)
}

type pgxClient struct {
	config Config
	pool   *pgxpool.Pool
}

// NewClient opens a long-lived PostgreSQL client with runtime TLS configuration applied.
func NewClient(ctx context.Context, cfg Config) (Client, error) {
	pool, err := openPool(ctx, cfg)
	if err != nil {
		return nil, err
	}

	return &pgxClient{
		config: cfg,
		pool:   pool,
	}, nil
}

// Config returns the connection settings used to open the client.
func (c *pgxClient) Config() Config {
	if c == nil {
		return Config{}
	}

	return c.config
}

// Close closes the owned pool. It is safe to call more than once.
func (c *pgxClient) Close() {
	if c == nil || c.pool == nil {
		return
	}

	c.pool.Close()
	c.pool = nil
}

// Ping verifies that the open client can reach the server.
func (c *pgxClient) Ping(ctx context.Context) error {
	if c == nil || c.pool == nil {
		return fmt.Errorf("postgres client is not open")
	}

	return sanitize(c.pool.Ping(ctx), c.config.Password)
}

// Exec executes a statement through the owned pool.
func (c *pgxClient) Exec(
	ctx context.Context,
	sql string,
	args ...any,
) (pgconn.CommandTag, error) {
	if c == nil || c.pool == nil {
		return pgconn.CommandTag{}, fmt.Errorf("postgres client is not open")
	}

	return c.pool.Exec(ctx, sql, args...)
}

// Query executes a multi-row query through the owned pool.
func (c *pgxClient) Query(
	ctx context.Context,
	sql string,
	args ...any,
) (pgx.Rows, error) {
	if c == nil || c.pool == nil {
		return nil, fmt.Errorf("postgres client is not open")
	}

	return c.pool.Query(ctx, sql, args...)
}

// QueryRow executes a single-row query through the owned pool.
func (c *pgxClient) QueryRow(
	ctx context.Context,
	sql string,
	args ...any,
) (pgx.Row, error) {
	if c == nil || c.pool == nil {
		return nil, fmt.Errorf("postgres client is not open")
	}

	return c.pool.QueryRow(ctx, sql, args...), nil
}

func openPool(ctx context.Context, cfg Config) (*pgxpool.Pool, error) {
	poolConfig, err := poolConfigFor(cfg)
	if err != nil {
		return nil, sanitize(fmt.Errorf("building pool config: %w", err), cfg.Password)
	}

	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return nil, sanitize(fmt.Errorf("opening pool: %w", err), cfg.Password)
	}

	return pool, nil
}
