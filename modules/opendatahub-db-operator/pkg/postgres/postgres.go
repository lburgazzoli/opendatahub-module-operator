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
	"fmt"
	"net"
	"net/url"
	"reflect"
	"strconv"

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
	// SecretKeySSLMode holds the libpq sslmode value (e.g. "disable", "require",
	// "verify-full"). Optional in the Secret; when absent, callers apply their own
	// default ("disable" for embedded, "require" for external).
	SecretKeySSLMode = "pg.sslmode"

	// DefaultPort is the standard PostgreSQL port, used when pg.port is absent.
	DefaultPort = 5432

	// SSLMode* are the libpq sslmode values accepted by pg.sslmode.
	SSLModeDisable    = "disable"
	SSLModeRequire    = "require"
	SSLModeVerifyCA   = "verify-ca"
	SSLModeVerifyFull = "verify-full"
)

// Config holds the connection parameters parsed from or written to a Kubernetes
// Secret (docs/plan.md §6). The pg.* key convention keeps the Secret schema
// clean and namespaced. The mapstructure tags match the SecretKey*
// constants above. Schema and SSLMode are optional.
type Config struct {
	Host     string `mapstructure:"pg.host"`
	Port     int    `mapstructure:"pg.port"`
	User     string `mapstructure:"pg.user"`
	Password string `mapstructure:"pg.password"`
	DBName   string `mapstructure:"pg.database"`
	Schema   string `mapstructure:"pg.schema"`
	SSLMode  string `mapstructure:"pg.sslmode"`
}

type secretField struct {
	value string
	key   string
}

// Validate checks that all required Config fields are set and the port is valid.
func (c Config) Validate() error {
	for _, field := range [...]secretField{
		{c.Host, SecretKeyHost},
		{c.User, SecretKeyUser},
		{c.Password, SecretKeyPassword},
		{c.DBName, SecretKeyDatabase},
	} {
		if field.value == "" {
			return fmt.Errorf("missing or empty key %q in Secret", field.key)
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
	// pgx does not expose the raw sslmode string after parsing, so extract it
	// directly from the URL query string.
	sslMode := ""
	if u, err := url.Parse(dsn); err == nil {
		sslMode = u.Query().Get("sslmode")
	}
	return Config{
		Host:     pcfg.ConnConfig.Host,
		Port:     int(pcfg.ConnConfig.Port),
		User:     pcfg.ConnConfig.User,
		Password: pcfg.ConnConfig.Password,
		DBName:   pcfg.ConnConfig.Database,
		SSLMode:  sslMode,
	}, nil
}

// DSN returns the libpq-style connection string for the config.
func (c Config) DSN() string {
	q := url.Values{}
	if c.SSLMode != "" {
		q.Set("sslmode", c.SSLMode)
	}
	return (&url.URL{
		Scheme:   "postgres",
		User:     url.UserPassword(c.User, c.Password),
		Host:     net.JoinHostPort(c.Host, strconv.Itoa(c.Port)),
		Path:     c.DBName,
		RawQuery: q.Encode(),
	}).String()
}
