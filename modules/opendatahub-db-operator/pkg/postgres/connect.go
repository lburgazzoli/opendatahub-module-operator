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

	"github.com/jackc/pgx/v5/pgxpool"
)

type poolPinger interface {
	Ping(ctx context.Context) error
	Close()
}

var openPingPool = func(ctx context.Context, dsn string) (poolPinger, error) {
	return pgxpool.New(ctx, dsn)
}

func OpenPool(ctx context.Context, cfg Config) (*pgxpool.Pool, error) {
	pool, err := pgxpool.New(ctx, cfg.DSN())
	if err != nil {
		return nil, sanitize(fmt.Errorf("opening pool: %w", err), cfg.Password)
	}

	return pool, nil
}

// Ping opens a short-lived connection to verify the server is reachable, then
// closes it immediately. It is a liveness check, not a long-lived pool.
// The returned error message never contains the password.
func Ping(ctx context.Context, cfg Config) error {
	pool, err := openPingPool(ctx, cfg.DSN())
	if err != nil {
		return sanitize(err, cfg.Password)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		return sanitize(err, cfg.Password)
	}

	return nil
}
