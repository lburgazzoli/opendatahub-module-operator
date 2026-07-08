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
	"reflect"

	"github.com/go-viper/mapstructure/v2"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Secret key names for connection parameters. Uses a "pg.<field>" properties
// convention rather than PostgreSQL's PGXXX env-var names to keep the Secret
// schema clean and namespaced.
const (
	SecretKeyHost     = "pg.host"
	SecretKeyPort     = "pg.port"
	SecretKeyUser     = "pg.user"
	SecretKeyPassword = "pg.password"
	SecretKeyDatabase = "pg.database"
	SecretKeySchema   = "pg.schema"

	// DefaultPort is the standard PostgreSQL port, used when pg.port is absent.
	DefaultPort = 5432
)

// Config holds the connection parameters parsed from or written to a Kubernetes
// Secret (docs/plan.md §6). The pg.* key convention keeps the Secret schema
// clean and namespaced. The mapstructure tags match the SecretKey*
// constants above. Schema is optional -- set only for SchemaClaim credentials.
type Config struct {
	Host     string `mapstructure:"pg.host"`
	Port     int    `mapstructure:"pg.port"`
	User     string `mapstructure:"pg.user"`
	Password string `mapstructure:"pg.password"`
	DBName   string `mapstructure:"pg.database"`
	Schema   string `mapstructure:"pg.schema"`
}

// Validate checks that all required Config fields are set and the port is valid.
func (c Config) Validate() error {
	for _, pair := range [...]struct {
		val string
		key string
	}{
		{c.Host, SecretKeyHost},
		{c.User, SecretKeyUser},
		{c.Password, SecretKeyPassword},
		{c.DBName, SecretKeyDatabase},
	} {
		if pair.val == "" {
			return fmt.Errorf("missing or empty key %q in Secret", pair.key)
		}
	}
	if c.Port <= 0 || c.Port > 65535 {
		return fmt.Errorf("invalid %s: must be a port number 1-65535, got %d", SecretKeyPort, c.Port)
	}
	return nil
}

// ParseSecret decodes a Secret's data map ([]byte values) into a Config using
// mapstructure. The inline hook promotes []byte → string first so that
// WeaklyTypedInput can then handle string → int for PGPORT. PGPORT defaults to
// DefaultPort when absent.
func ParseSecret(data map[string][]byte) (Config, error) {
	cfg := Config{Port: DefaultPort}

	dec, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		Result:           &cfg,
		WeaklyTypedInput: true, // converts string "5432" → int
		TagName:          "mapstructure",
		DecodeHook: mapstructure.ComposeDecodeHookFunc(
			func(f reflect.Type, _ reflect.Type, v any) (any, error) {
				if f == reflect.TypeFor[[]byte]() {
					return string(v.([]byte)), nil
				}
				return v, nil
			},
		),
	})
	if err != nil {
		return cfg, fmt.Errorf("building decoder: %w", err)
	}
	if err := dec.Decode(data); err != nil {
		return cfg, fmt.Errorf("decoding Secret fields: %w", err)
	}

	return cfg, cfg.Validate()
}

// ConfigFromDSN parses a libpq or postgres:// DSN into a Config using
// pgxpool's own parser -- no manual URL decomposition needed. Useful in tests
// where the DSN comes from a testcontainers ConnectionString() call.
func ConfigFromDSN(dsn string) (Config, error) {
	pcfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return Config{}, fmt.Errorf("parsing DSN: %w", err)
	}
	return Config{
		Host:     pcfg.ConnConfig.Config.Host,
		Port:     int(pcfg.ConnConfig.Config.Port),
		User:     pcfg.ConnConfig.Config.User,
		Password: pcfg.ConnConfig.Config.Password,
		DBName:   pcfg.ConnConfig.Config.Database,
	}, nil
}

// DSN returns the libpq-style connection string for the config.
func (c Config) DSN() string {
	return fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=disable",
		c.Host, c.Port, c.User, c.Password, c.DBName)
}

// Ping opens a single connection to verify the server is reachable, then
// closes it immediately. It is a liveness check, not a long-lived pool.
// The returned error message never contains the password.
func Ping(ctx context.Context, cfg Config) error {
	pool, err := pgxpool.New(ctx, cfg.DSN())
	if err != nil {
		return sanitize(err, cfg.Password)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		return sanitize(err, cfg.Password)
	}
	return nil
}
