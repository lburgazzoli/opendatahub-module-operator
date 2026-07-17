package instance

import (
	"testing"

	. "github.com/onsi/gomega"

	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/pkg/postgres"
)

func TestAdminSecret(t *testing.T) {
	t.Run("without tls", func(t *testing.T) {
		g := NewWithT(t)

		secret := AdminSecret(
			Data{
				Namespace: "test-ns",
				Service: Service{
					Name: "test-db",
				},
				Postgres: Postgres{
					AdminSecretName: "test-db-admin",
				},
			},
			[]byte("test-password"),
			nil,
		)

		g.Expect(secret.Name).To(Equal("test-db-admin"))
		g.Expect(secret.Namespace).To(Equal("test-ns"))
		g.Expect(secret.Data).To(HaveKeyWithValue(postgres.SecretKeyHost, []byte("test-db.test-ns.svc")))
		g.Expect(secret.Data).To(HaveKeyWithValue(postgres.SecretKeyUser, []byte(DefaultAdminUser)))
		g.Expect(secret.Data).To(HaveKeyWithValue(postgres.SecretKeyDatabase, []byte(DefaultAdminDatabase)))
		g.Expect(secret.Data).To(HaveKeyWithValue(postgres.SecretKeyPassword, []byte("test-password")))
		g.Expect(secret.Data).NotTo(HaveKey(postgres.SecretKeySSLMode))
		g.Expect(secret.Data).NotTo(HaveKey(postgres.SecretKeyCA))
	})

	t.Run("with tls", func(t *testing.T) {
		g := NewWithT(t)

		secret := AdminSecret(
			Data{
				Namespace: "test-ns",
				Service: Service{
					Name: "test-db",
				},
				Postgres: Postgres{
					AdminSecretName: "test-db-admin",
				},
				TLS: TLS{
					Enabled: true,
				},
			},
			[]byte("test-password"),
			[]byte("test-ca"),
		)

		g.Expect(secret.Data).To(HaveKeyWithValue(postgres.SecretKeySSLMode, []byte(postgres.SSLModeVerifyFull)))
		g.Expect(secret.Data).To(HaveKeyWithValue(postgres.SecretKeyCA, []byte("test-ca")))
	})
}

func TestAdminConfig(t *testing.T) {
	t.Run("without tls", func(t *testing.T) {
		g := NewWithT(t)

		cfg := AdminConfig(
			Data{
				Namespace: "test-ns",
				Service: Service{
					Name: "test-db",
				},
			},
			"test-password",
			nil,
		)

		g.Expect(cfg.Host).To(Equal("test-db.test-ns.svc"))
		g.Expect(cfg.Port).To(Equal(postgres.DefaultPort))
		g.Expect(cfg.User).To(Equal(DefaultAdminUser))
		g.Expect(cfg.Password).To(Equal("test-password"))
		g.Expect(cfg.DBName).To(Equal(DefaultAdminDatabase))
		g.Expect(cfg.SSLMode).To(Equal(postgres.SSLModeDisable))
		g.Expect(cfg.SSLRootCert).To(BeEmpty())
	})

	t.Run("with tls", func(t *testing.T) {
		g := NewWithT(t)

		cfg := AdminConfig(
			Data{
				Namespace: "test-ns",
				Service: Service{
					Name: "test-db",
				},
				TLS: TLS{
					Enabled: true,
				},
			},
			"test-password",
			[]byte("test-ca"),
		)

		g.Expect(cfg.SSLMode).To(Equal(postgres.SSLModeVerifyFull))
		g.Expect(cfg.SSLRootCert).To(Equal("test-ca"))
	})
}
