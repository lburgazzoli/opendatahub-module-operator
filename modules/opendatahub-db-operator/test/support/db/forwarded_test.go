package db

import (
	"testing"

	. "github.com/onsi/gomega"

	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/pkg/postgres"
)

func TestConfigWithForwardTarget(t *testing.T) {
	g := NewWithT(t)

	cfg, err := ConfigWithForwardTarget(postgres.Config{
		Host:        "provider.ns.svc",
		Port:        5432,
		User:        "user",
		Password:    "password",
		DBName:      "postgres",
		Schema:      "app",
		SSLMode:     postgres.SSLModeRequire,
		SSLRootCert: "ca",
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

func TestConfigWithForwardTarget_ValidatesForwardTarget(t *testing.T) {
	t.Run("requires local host", func(t *testing.T) {
		g := NewWithT(t)
		_, err := ConfigWithForwardTarget(postgres.Config{}, "", 15432)
		g.Expect(err).To(MatchError("local host is empty"))
	})

	t.Run("requires valid local port", func(t *testing.T) {
		g := NewWithT(t)
		_, err := ConfigWithForwardTarget(postgres.Config{}, "127.0.0.1", 0)
		g.Expect(err).To(MatchError("local port must be between 1 and 65535, got 0"))
	})
}
