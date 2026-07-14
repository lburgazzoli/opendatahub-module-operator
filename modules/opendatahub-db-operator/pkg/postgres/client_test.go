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
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	. "github.com/onsi/gomega"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
)

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

func TestClient_Ping_Success(t *testing.T) {
	g := NewWithT(t)
	cfg := startPingPostgres(t)

	cli, err := NewClient(t.Context(), cfg)
	g.Expect(err).NotTo(HaveOccurred())
	t.Cleanup(cli.Close)

	err = cli.Ping(t.Context())
	g.Expect(err).NotTo(HaveOccurred())
}

func TestClient_Ping_WrongPassword(t *testing.T) {
	g := NewWithT(t)
	cfg := startPingPostgres(t)

	bad := cfg
	bad.Password = "totally-wrong-password-sentinel"

	cli, err := NewClient(t.Context(), bad)
	g.Expect(err).NotTo(HaveOccurred())
	t.Cleanup(cli.Close)

	err = cli.Ping(t.Context())
	g.Expect(err).To(HaveOccurred())
	g.Expect(err).NotTo(MatchError(ContainSubstring(bad.Password)))
}

func TestNewClient_UnreachableHostTimesOut(t *testing.T) {
	g := NewWithT(t)

	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()

	cli, err := NewClient(ctx, Config{
		Host:     "203.0.113.1",
		Port:     5432,
		User:     "user",
		Password: "secret",
		DBName:   "db",
	})
	g.Expect(err).NotTo(HaveOccurred())
	t.Cleanup(cli.Close)

	err = cli.Ping(ctx)

	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(SatisfyAny(
		ContainSubstring("timeout"),
		ContainSubstring("deadline exceeded"),
	))
}

func TestNewClient_ExecQueryAndQueryRow(t *testing.T) {
	g := NewWithT(t)
	cfg := startPingPostgres(t)

	cli, err := NewClient(t.Context(), cfg)
	g.Expect(err).NotTo(HaveOccurred())
	t.Cleanup(cli.Close)

	_, err = cli.Exec(t.Context(), "CREATE TABLE IF NOT EXISTS client_api_test (id int)")
	g.Expect(err).NotTo(HaveOccurred())

	_, err = cli.Exec(t.Context(), "TRUNCATE client_api_test")
	g.Expect(err).NotTo(HaveOccurred())

	tag, err := cli.Exec(t.Context(), "INSERT INTO client_api_test (id) VALUES ($1)", 7)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(tag).To(Equal(pgconn.NewCommandTag("INSERT 0 1")))

	var count int
	row, err := cli.QueryRow(t.Context(), "SELECT count(*) FROM client_api_test")
	g.Expect(err).NotTo(HaveOccurred())
	err = row.Scan(&count)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(count).To(Equal(1))

	rows, err := cli.Query(t.Context(), "SELECT id FROM client_api_test")
	g.Expect(err).NotTo(HaveOccurred())
	defer rows.Close()

	g.Expect(rows.Next()).To(BeTrue())

	var id int
	err = rows.Scan(&id)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(id).To(Equal(7))
	g.Expect(rows.Next()).To(BeFalse())
	g.Expect(rows.Err()).NotTo(HaveOccurred())
}

func TestClient_CloseMakesFurtherOperationsFail(t *testing.T) {
	g := NewWithT(t)
	cfg := startPingPostgres(t)

	cli, err := NewClient(t.Context(), cfg)
	g.Expect(err).NotTo(HaveOccurred())
	cli.Close()
	cli.Close()

	_, err = cli.Exec(t.Context(), "SELECT 1")
	g.Expect(err).To(MatchError(ContainSubstring("not open")))

	row, err := cli.QueryRow(t.Context(), "SELECT 1")
	g.Expect(row).To(BeNil())
	g.Expect(err).To(MatchError(ContainSubstring("not open")))
}

func TestPoolConfigFor_ConfiguresVerifyFullFromCAData(t *testing.T) {
	g := NewWithT(t)

	poolConfig, err := poolConfigFor(Config{
		Host:        "db.example.test",
		Port:        5432,
		User:        "user",
		Password:    "secret",
		DBName:      "db",
		SSLMode:     SSLModeVerifyFull,
		SSLRootCert: string(testRootCA(t)),
	})

	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(poolConfig.ConnConfig.TLSConfig).NotTo(BeNil())
	g.Expect(poolConfig.ConnConfig.TLSConfig.RootCAs).NotTo(BeNil())
	g.Expect(poolConfig.ConnConfig.TLSConfig.InsecureSkipVerify).To(BeFalse())
	g.Expect(poolConfig.ConnConfig.TLSConfig.VerifyPeerCertificate).To(BeNil())
	g.Expect(poolConfig.ConnConfig.TLSConfig.ServerName).To(Equal("db.example.test"))
}

func TestPoolConfigFor_ConfiguresVerifyCAFromCAData(t *testing.T) {
	g := NewWithT(t)

	poolConfig, err := poolConfigFor(Config{
		Host:        "db.example.test",
		Port:        5432,
		User:        "user",
		Password:    "secret",
		DBName:      "db",
		SSLMode:     SSLModeVerifyCA,
		SSLRootCert: string(testRootCA(t)),
	})

	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(poolConfig.ConnConfig.TLSConfig).NotTo(BeNil())
	g.Expect(poolConfig.ConnConfig.TLSConfig.RootCAs).NotTo(BeNil())
	g.Expect(poolConfig.ConnConfig.TLSConfig.InsecureSkipVerify).To(BeTrue())
	g.Expect(poolConfig.ConnConfig.TLSConfig.VerifyPeerCertificate).NotTo(BeNil())
}

func TestPoolConfigFor_RejectsInvalidCAData(t *testing.T) {
	g := NewWithT(t)

	_, err := poolConfigFor(Config{
		Host:        "db.example.test",
		Port:        5432,
		User:        "user",
		Password:    "secret",
		DBName:      "db",
		SSLMode:     SSLModeVerifyFull,
		SSLRootCert: "not a pem bundle",
	})

	g.Expect(err).To(MatchError(ContainSubstring("unable to add CA to cert pool")))
}

func testRootCA(t *testing.T) []byte {
	t.Helper()

	g := NewWithT(t)

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	g.Expect(err).NotTo(HaveOccurred())

	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			CommonName: "odh-db-operator-test-ca",
		},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
	}

	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	g.Expect(err).NotTo(HaveOccurred())

	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
}
