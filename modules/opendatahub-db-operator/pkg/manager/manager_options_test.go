package manager

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/pkg/postgres"
	. "github.com/onsi/gomega"
)

func TestOptionsApplyOption_MergesModuleOptions(t *testing.T) {
	g := NewWithT(t)

	var called bool
	factory := postgres.ClientFactory(func(
		_ context.Context,
		cfg postgres.Config,
	) (postgres.Client, error) {
		called = true
		return stubClient{config: cfg}, nil
	})

	target := Options{}
	Options{
		PostgresClientFactory: factory,
	}.applyOption(&target)

	g.Expect(target.PostgresClientFactory).NotTo(BeNil())

	client, err := target.PostgresClientFactory(
		context.Background(),
		postgres.Config{Host: "db.example.test"},
	)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(client.Config().Host).To(Equal("db.example.test"))
	g.Expect(called).To(BeTrue())
}

func TestDefaultPostgresClientFactory_IsUsable(t *testing.T) {
	g := NewWithT(t)

	g.Expect(postgres.DefaultClientFactory).NotTo(BeNil())
	_, err := postgres.DefaultClientFactory(
		context.Background(),
		postgres.Config{
			Host:     "203.0.113.1",
			Port:     5432,
			User:     "user",
			Password: "secret",
			DBName:   "postgres",
		},
	)
	g.Expect(err).NotTo(HaveOccurred())
}

type stubClient struct {
	config postgres.Config
}

func (s stubClient) Config() postgres.Config  { return s.config }
func (stubClient) Close()                     {}
func (stubClient) Ping(context.Context) error { return nil }
func (stubClient) Exec(context.Context, string, ...any) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, nil
}
func (stubClient) Query(context.Context, string, ...any) (pgx.Rows, error) {
	return nil, nil
}
func (stubClient) QueryRow(context.Context, string, ...any) (pgx.Row, error) {
	return nil, nil
}
