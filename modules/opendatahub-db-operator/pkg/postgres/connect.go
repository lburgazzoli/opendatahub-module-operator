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
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type poolPinger interface {
	Ping(ctx context.Context) error
	Close()
}

func OpenPool(ctx context.Context, cfg Config) (*pgxpool.Pool, error) {
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

// Ping opens a short-lived connection to verify the server is reachable, then
// closes it immediately. It is a liveness check, not a long-lived pool.
// The returned error message never contains the password.
func Ping(ctx context.Context, cfg Config) error {
	poolConfig, err := poolConfigFor(cfg)
	if err != nil {
		return sanitize(fmt.Errorf("building pool config: %w", err), cfg.Password)
	}

	pool, err := openPingPool(ctx, poolConfig)
	if err != nil {
		return sanitize(err, cfg.Password)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		return sanitize(err, cfg.Password)
	}

	return nil
}

var openPingPool = func(ctx context.Context, poolConfig *pgxpool.Config) (poolPinger, error) {
	return pgxpool.NewWithConfig(ctx, poolConfig)
}

func poolConfigFor(cfg Config) (*pgxpool.Config, error) {
	poolConfig, err := pgxpool.ParseConfig(cfg.DSN())
	if err != nil {
		return nil, fmt.Errorf("parsing DSN: %w", err)
	}
	if err := applyRuntimeTLSConfig(poolConfig.ConnConfig, cfg.SSLMode, cfg.SSLRootCert); err != nil {
		return nil, err
	}
	return poolConfig, nil
}

func applyRuntimeTLSConfig(
	connConfig *pgx.ConnConfig,
	sslMode string,
	sslRootCert string,
) error {
	if connConfig == nil {
		return nil
	}

	tlsConfig, err := runtimeTLSConfig(connConfig.TLSConfig, connConfig.Host, sslMode, sslRootCert)
	if err != nil {
		return err
	}
	connConfig.TLSConfig = tlsConfig

	for _, fallback := range connConfig.Fallbacks {
		tlsConfig, err := runtimeTLSConfig(fallback.TLSConfig, fallback.Host, sslMode, sslRootCert)
		if err != nil {
			return err
		}
		fallback.TLSConfig = tlsConfig
	}

	return nil
}

func runtimeTLSConfig(
	base *tls.Config,
	host string,
	sslMode string,
	sslRootCert string,
) (*tls.Config, error) {
	if sslRootCert == "" || base == nil {
		return base, nil
	}

	caCertPool := x509.NewCertPool()
	if !caCertPool.AppendCertsFromPEM([]byte(sslRootCert)) {
		return nil, errors.New("unable to add CA to cert pool")
	}

	tlsConfig := base.Clone()
	tlsConfig.RootCAs = caCertPool
	tlsConfig.ClientCAs = caCertPool

	mode := sslMode
	if mode == SSLModeRequire {
		mode = SSLModeVerifyCA
	}

	switch mode {
	case SSLModeVerifyCA:
		tlsConfig.InsecureSkipVerify = true
		tlsConfig.VerifyPeerCertificate = verifyCertificateChain(tlsConfig)
	case SSLModeVerifyFull:
		tlsConfig.InsecureSkipVerify = false
		tlsConfig.VerifyPeerCertificate = nil
		if tlsConfig.ServerName == "" {
			tlsConfig.ServerName = host
		}
	default:
		tlsConfig.VerifyPeerCertificate = nil
	}

	return tlsConfig, nil
}

func verifyCertificateChain(tlsConfig *tls.Config) func([][]byte, [][]*x509.Certificate) error {
	return func(certificates [][]byte, _ [][]*x509.Certificate) error {
		if len(certificates) == 0 {
			return errors.New("server sent no certificates")
		}

		certs := make([]*x509.Certificate, len(certificates))
		for i, asn1Data := range certificates {
			cert, err := x509.ParseCertificate(asn1Data)
			if err != nil {
				return fmt.Errorf("failed to parse certificate from server: %w", err)
			}
			certs[i] = cert
		}

		opts := x509.VerifyOptions{
			Roots:         tlsConfig.RootCAs,
			Intermediates: x509.NewCertPool(),
		}
		for _, cert := range certs[1:] {
			opts.Intermediates.AddCert(cert)
		}

		_, err := certs[0].Verify(opts)
		return err
	}
}
