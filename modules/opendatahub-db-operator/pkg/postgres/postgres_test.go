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

package postgres_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	. "github.com/onsi/gomega"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"

	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/pkg/postgres"
)

// startPostgres starts an ephemeral Postgres container and returns the Config.
// The container uses the stock postgres:16 image.
func startPostgres(t *testing.T) postgres.Config {
	t.Helper()
	g := NewWithT(t)

	ctr, err := tcpostgres.Run(t.Context(), "postgres:16",
		tcpostgres.WithDatabase("testdb"),
		tcpostgres.WithUsername("testuser"),
		tcpostgres.WithPassword("testpassword"),
		tcpostgres.BasicWaitStrategies(), // waits for log "ready" twice + port up
	)
	g.Expect(err).NotTo(HaveOccurred())

	t.Cleanup(func() {
		_ = ctr.Terminate(context.Background())
	})

	connStr, err := ctr.ConnectionString(t.Context(), "sslmode=disable")
	g.Expect(err).NotTo(HaveOccurred())

	cfg, err := postgres.ConfigFromDSN(connStr)
	g.Expect(err).NotTo(HaveOccurred())

	return cfg
}

// TestParseSecret_MissingKeys verifies that each required key absence
// produces a clear error before any connection is attempted.
func TestParseSecret_MissingKeys(t *testing.T) {
	complete := map[string][]byte{
		postgres.SecretKeyHost:     []byte("localhost"),
		postgres.SecretKeyPort:     []byte("5432"),
		postgres.SecretKeyUser:     []byte("admin"),
		postgres.SecretKeyPassword: []byte("secret"),
		postgres.SecretKeyDatabase: []byte("postgres"),
	}

	for _, omit := range []string{
		postgres.SecretKeyHost,
		postgres.SecretKeyUser,
		postgres.SecretKeyPassword,
		postgres.SecretKeyDatabase,
	} {
		t.Run("missing-"+omit, func(t *testing.T) {
			g := NewWithT(t)
			data := make(map[string][]byte)
			for k, v := range complete {
				if k != omit {
					data[k] = v
				}
			}
			_, err := postgres.ParseSecret(data)
			g.Expect(err).To(MatchError(ContainSubstring(omit)))
		})
	}
}

func TestParseSecret_InvalidPort(t *testing.T) {
	g := NewWithT(t)
	_, err := postgres.ParseSecret(map[string][]byte{
		postgres.SecretKeyHost:     []byte("localhost"),
		postgres.SecretKeyPort:     []byte("notanumber"),
		postgres.SecretKeyUser:     []byte("u"),
		postgres.SecretKeyPassword: []byte("p"),
		postgres.SecretKeyDatabase: []byte("d"),
	})
	g.Expect(err).To(HaveOccurred())
	g.Expect(err).To(MatchError(ContainSubstring(postgres.SecretKeyPort)))
}

func TestParseSecret_DefaultPort(t *testing.T) {
	g := NewWithT(t)
	cfg, err := postgres.ParseSecret(map[string][]byte{
		postgres.SecretKeyHost:     []byte("localhost"),
		postgres.SecretKeyUser:     []byte("u"),
		postgres.SecretKeyPassword: []byte("p"),
		postgres.SecretKeyDatabase: []byte("d"),
	})
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(cfg.Port).To(Equal(postgres.DefaultPort))
}

func TestConfigDSN_EscapesSpecialCharacters(t *testing.T) {
	g := NewWithT(t)

	cfg := postgres.Config{
		Host:     "db.example.test",
		Port:     5432,
		User:     "user name",
		Password: "pa ss:wo/rd?&=#",
		DBName:   "app-db",
		SSLMode:  "disable",
	}

	parsed, err := pgxpool.ParseConfig(cfg.DSN())
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(parsed.ConnConfig.Host).To(Equal(cfg.Host))
	g.Expect(parsed.ConnConfig.Port).To(Equal(uint16(cfg.Port)))
	g.Expect(parsed.ConnConfig.User).To(Equal(cfg.User))
	g.Expect(parsed.ConnConfig.Password).To(Equal(cfg.Password))
	g.Expect(parsed.ConnConfig.Database).To(Equal(cfg.DBName))
}

// TestPing_Success tests a real connection against a testcontainers Postgres.
func TestPing_Success(t *testing.T) {
	g := NewWithT(t)
	cfg := startPostgres(t)

	err := postgres.Ping(t.Context(), cfg)
	g.Expect(err).NotTo(HaveOccurred())
}

// TestPing_WrongPassword verifies that a failed connection returns an error
// that does NOT contain the password literal.
func TestPing_WrongPassword(t *testing.T) {
	g := NewWithT(t)
	cfg := startPostgres(t)

	bad := cfg
	bad.Password = "totally-wrong-password-sentinel"

	err := postgres.Ping(t.Context(), bad)
	g.Expect(err).To(HaveOccurred())
	g.Expect(err).NotTo(MatchError(ContainSubstring(bad.Password)),
		"error message must not contain the password literal")
}
