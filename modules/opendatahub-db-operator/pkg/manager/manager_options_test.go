package manager

import (
	"context"
	"testing"

	. "github.com/onsi/gomega"

	infraApi "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/api/infrastructure/v1alpha1"
	dbcontroller "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/pkg/controller"
	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/pkg/postgres"
)

func TestOptionsApplyOption_MergesModuleOptions(t *testing.T) {
	g := NewWithT(t)

	var called bool
	resolver := dbcontroller.PostgresConnectionConfigResolveFunc(func(
		_ context.Context,
		_ *infraApi.DatabaseProvider,
		cfg postgres.Config,
	) (postgres.Config, error) {
		called = true
		return cfg, nil
	})

	target := Options{}
	Options{
		PostgresConnectionConfigResolver: resolver,
	}.applyOption(&target)

	g.Expect(target.PostgresConnectionConfigResolver).NotTo(BeNil())

	_, err := target.PostgresConnectionConfigResolver.Resolve(
		context.Background(),
		&infraApi.DatabaseProvider{},
		postgres.Config{},
	)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(called).To(BeTrue())
}

func TestDefaultPostgresConnectionConfigResolver_IsUsable(t *testing.T) {
	g := NewWithT(t)

	got, err := dbcontroller.DefaultPostgresConnectionConfigResolver().Resolve(
		context.Background(),
		&infraApi.DatabaseProvider{},
		postgres.Config{
			Host: "postgres.db-admin.svc",
			Port: 5432,
		},
	)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(got.Host).To(Equal("postgres.db-admin.svc"))
	g.Expect(got.Port).To(Equal(5432))
}
