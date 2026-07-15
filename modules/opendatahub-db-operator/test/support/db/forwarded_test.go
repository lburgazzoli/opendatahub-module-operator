package db

import (
	"testing"

	. "github.com/onsi/gomega"

	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/pkg/postgres"
)

func TestConfigFromForwardedSecret(t *testing.T) {
	g := NewWithT(t)

	cfg, err := ConfigFromForwardedSecret(map[string][]byte{
		postgres.SecretKeyHost:     []byte("provider.ns.svc"),
		postgres.SecretKeyPort:     []byte("5432"),
		postgres.SecretKeyUser:     []byte("user"),
		postgres.SecretKeyPassword: []byte("password"),
		postgres.SecretKeyDatabase: []byte("postgres"),
		postgres.SecretKeySchema:   []byte("app"),
		postgres.SecretKeySSLMode:  []byte(postgres.SSLModeRequire),
		postgres.SecretKeyCA:       []byte("ca"),
	}, "127.0.0.1", 15432)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(cfg.Host).To(Equal("127.0.0.1"))
	g.Expect(cfg.Port).To(Equal(15432))
	g.Expect(cfg.User).To(Equal("user"))
	g.Expect(cfg.Password).To(Equal("password"))
	g.Expect(cfg.DBName).To(Equal("postgres"))
	g.Expect(cfg.Schema).To(Equal("app"))
	g.Expect(cfg.SSLMode).To(Equal(postgres.SSLModeRequire))
	g.Expect(cfg.SSLRootCert).To(Equal("ca"))
}

func TestConfigFromForwardedSecret_ValidatesForwardTarget(t *testing.T) {
	t.Run("requires local host", func(t *testing.T) {
		g := NewWithT(t)
		_, err := ConfigFromForwardedSecret(map[string][]byte{}, "", 15432)
		g.Expect(err).To(MatchError("local host is empty"))
	})

	t.Run("requires valid local port", func(t *testing.T) {
		g := NewWithT(t)
		_, err := ConfigFromForwardedSecret(map[string][]byte{}, "127.0.0.1", 0)
		g.Expect(err).To(MatchError("local port must be between 1 and 65535, got 0"))
	})
}
