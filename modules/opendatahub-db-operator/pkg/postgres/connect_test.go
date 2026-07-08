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
	"testing"
	"time"

	. "github.com/onsi/gomega"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
)

type stubPingPool struct {
	pingErr error
	closed  bool
}

func (p *stubPingPool) Ping(context.Context) error {
	return p.pingErr
}

func (p *stubPingPool) Close() {
	p.closed = true
}

func startPingPostgres(t *testing.T) Config {
	t.Helper()

	g := NewWithT(t)
	ctr, err := tcpostgres.Run(t.Context(), "postgres:16",
		tcpostgres.WithDatabase("testdb"),
		tcpostgres.WithUsername("testuser"),
		tcpostgres.WithPassword("testpassword"),
		tcpostgres.BasicWaitStrategies(),
	)
	g.Expect(err).NotTo(HaveOccurred())

	t.Cleanup(func() {
		_ = ctr.Terminate(context.Background())
	})

	connStr, err := ctr.ConnectionString(t.Context(), "sslmode=disable")
	g.Expect(err).NotTo(HaveOccurred())

	cfg, err := ConfigFromDSN(connStr)
	g.Expect(err).NotTo(HaveOccurred())
	return cfg
}

func TestPing_ClosesPoolOnPingError(t *testing.T) {
	g := NewWithT(t)

	pool := &stubPingPool{pingErr: fmt.Errorf("boom")}
	previous := openPingPool
	openPingPool = func(context.Context, string) (poolPinger, error) {
		return pool, nil
	}
	defer func() {
		openPingPool = previous
	}()

	err := Ping(t.Context(), Config{
		Host:     "localhost",
		Port:     5432,
		User:     "user",
		Password: "secret",
		DBName:   "db",
	})

	g.Expect(err).To(HaveOccurred())
	g.Expect(pool.closed).To(BeTrue())
}

func TestPing_ClosesPoolOnSuccess(t *testing.T) {
	g := NewWithT(t)

	pool := &stubPingPool{}
	previous := openPingPool
	openPingPool = func(context.Context, string) (poolPinger, error) {
		return pool, nil
	}
	defer func() {
		openPingPool = previous
	}()

	err := Ping(t.Context(), Config{
		Host:     "localhost",
		Port:     5432,
		User:     "user",
		Password: "secret",
		DBName:   "db",
	})

	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(pool.closed).To(BeTrue())
}

func TestPing_Success(t *testing.T) {
	g := NewWithT(t)
	cfg := startPingPostgres(t)

	err := Ping(t.Context(), cfg)
	g.Expect(err).NotTo(HaveOccurred())
}

func TestPing_WrongPassword(t *testing.T) {
	g := NewWithT(t)
	cfg := startPingPostgres(t)

	bad := cfg
	bad.Password = "totally-wrong-password-sentinel"

	err := Ping(t.Context(), bad)
	g.Expect(err).To(HaveOccurred())
	g.Expect(err).NotTo(MatchError(ContainSubstring(bad.Password)))
}

func TestPing_UnreachableHostTimesOut(t *testing.T) {
	g := NewWithT(t)

	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()

	err := Ping(ctx, Config{
		Host:     "203.0.113.1",
		Port:     5432,
		User:     "user",
		Password: "secret",
		DBName:   "db",
	})

	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(SatisfyAny(
		ContainSubstring("timeout"),
		ContainSubstring("deadline exceeded"),
	))
}
