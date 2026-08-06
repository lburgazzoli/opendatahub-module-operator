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

package config_test

import (
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"
	"time"

	. "github.com/onsi/gomega"

	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/pkg/config"
)

func TestLoad_Defaults(t *testing.T) {
	g := NewWithT(t)

	cfg, err := config.Load()
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(cfg.PlatformType).To(Equal(config.DefaultPlatformType))
	g.Expect(cfg.PlatformVersion.String()).To(Equal("0.0.0"))
	g.Expect(cfg.OperatorNamespace).To(Equal(config.DefaultOperatorNS))
}

func TestLoad_ParsesPlatformVersion(t *testing.T) {
	g := NewWithT(t)

	cfg, err := config.Load(config.WithFS(fstest.MapFS{
		config.KeyPlatformVersion: {Data: []byte("3.5.0")},
	}))
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(cfg.PlatformVersion.String()).To(Equal("3.5.0"))
}

func TestLoad_EmptyPlatformVersionIsZero(t *testing.T) {
	g := NewWithT(t)

	cfg, err := config.Load(config.WithFS(fstest.MapFS{
		config.KeyPlatformVersion: {Data: []byte("")},
	}))
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(cfg.PlatformVersion.String()).To(Equal("0.0.0"))
}

func TestLoad_InvalidPlatformVersionReturnsError(t *testing.T) {
	g := NewWithT(t)

	_, err := config.Load(config.WithFS(fstest.MapFS{
		config.KeyPlatformVersion: {Data: []byte("not-a-version")},
	}))
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("not-a-version"))
}

func TestLoad_PlatformType(t *testing.T) {
	g := NewWithT(t)

	cfg, err := config.Load(config.WithFS(fstest.MapFS{
		config.KeyPlatformType: {Data: []byte(config.PlatformTypeSelfManagedRhoai)},
	}))
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(cfg.PlatformType).To(Equal(config.PlatformTypeSelfManagedRhoai))
}

func TestComponentRelease(t *testing.T) {
	g := NewWithT(t)

	cfg, err := config.Load(config.WithFS(fstest.MapFS{
		config.KeyPlatformVersion: {Data: []byte("2.1.0")},
	}))
	g.Expect(err).NotTo(HaveOccurred())

	rel := cfg.ComponentRelease()
	g.Expect(rel.Name).To(Equal(config.ReleasePlatform))
	g.Expect(rel.Version).To(Equal("2.1.0"))
}

func TestPlatformRelease(t *testing.T) {
	g := NewWithT(t)

	cfg, err := config.Load(config.WithFS(fstest.MapFS{
		config.KeyPlatformType:    {Data: []byte(config.DefaultPlatformType)},
		config.KeyPlatformVersion: {Data: []byte("2.1.0")},
	}))
	g.Expect(err).NotTo(HaveOccurred())

	rel := cfg.PlatformRelease()
	g.Expect(string(rel.Name)).To(Equal(config.DefaultPlatformType))
	g.Expect(rel.Version.String()).To(Equal("2.1.0"))
}

func TestComponentRelease_EmptyVersion(t *testing.T) {
	g := NewWithT(t)

	cfg, err := config.Load()
	g.Expect(err).NotTo(HaveOccurred())

	// Zero OperatorVersion serialises as "0.0.0"
	rel := cfg.ComponentRelease()
	g.Expect(rel.Name).To(Equal(config.ReleasePlatform))
	g.Expect(rel.Version).To(Equal("0.0.0"))
}

// Internal image / retry-interval config keys (docs/plan.md §6, §7.1, §7.7) --
// these must always be config-driven, never hardcoded literals in controller
// code, so their three-layer precedence (compiled default -> ConfigMap ->
// env var) is exercised explicitly.

func TestLoad_InternalAndRetryDefaults(t *testing.T) {
	g := NewWithT(t)

	cfg, err := config.Load()
	g.Expect(err).NotTo(HaveOccurred())

	g.Expect(cfg.Internal.PostgresImage).To(Equal(config.DefaultPostgresImage))
	g.Expect(cfg.Internal.PgvectorImage).To(Equal(config.DefaultPgvectorImage))
	g.Expect(cfg.GracePeriod).To(Equal(config.DefaultGracePeriod))

	// All four reconcilers share the same compiled default, but each is its
	// own independently-configurable key (docs/plan.md §6).
	g.Expect(cfg.SchemaClaim.RetryInterval).To(Equal(config.DefaultRetryInterval))
	g.Expect(cfg.DatabaseClaim.RetryInterval).To(Equal(config.DefaultRetryInterval))
	g.Expect(cfg.DatabaseProvider.RetryInterval).To(Equal(config.DefaultRetryInterval))
	g.Expect(cfg.DatabaseService.RetryInterval).To(Equal(config.DefaultRetryInterval))
}

func TestLoad_InternalImages_ConfigMapOverride(t *testing.T) {
	g := NewWithT(t)

	cfg, err := config.Load(config.WithFS(fstest.MapFS{
		config.KeyPostgresImage: {Data: []byte("registry.redhat.io/rhel9/postgresql-16")},
		config.KeyGracePeriod:   {Data: []byte("15m")},
	}))
	g.Expect(err).NotTo(HaveOccurred())

	g.Expect(cfg.Internal.PostgresImage).To(Equal("registry.redhat.io/rhel9/postgresql-16"))
	g.Expect(cfg.GracePeriod).To(Equal(15 * time.Minute))
	// Untouched keys keep their compiled defaults.
	g.Expect(cfg.Internal.PgvectorImage).To(Equal(config.DefaultPgvectorImage))
}

func TestLoad_RetryIntervals_AreIndependentAndEnvOverridesConfigMap(t *testing.T) {
	g := NewWithT(t)

	t.Setenv("ODH_MODULE_OPERATOR_DATABASEPROVIDER_RETRY_INTERVAL", "90s")
	t.Setenv("ODH_MODULE_OPERATOR_DATABASESERVICE_RETRY_INTERVAL", "1h")

	cfg, err := config.Load(config.WithFS(fstest.MapFS{
		// ConfigMap sets a different value than the env var above for
		// databaseprovider -- env must win. schemaclaim is ConfigMap-only.
		config.KeyDatabaseProviderRetryInterval: {Data: []byte("3m")},
		config.KeySchemaClaimRetryInterval:      {Data: []byte("7m")},
	}))
	g.Expect(err).NotTo(HaveOccurred())

	g.Expect(cfg.DatabaseProvider.RetryInterval).To(Equal(90 * time.Second))
	g.Expect(cfg.DatabaseService.RetryInterval).To(Equal(time.Hour))
	g.Expect(cfg.SchemaClaim.RetryInterval).To(Equal(7 * time.Minute))
	// Untouched by either ConfigMap or env -- stays at the compiled default,
	// proving the four keys are genuinely independent of one another.
	g.Expect(cfg.DatabaseClaim.RetryInterval).To(Equal(config.DefaultRetryInterval))
}

func TestLoad_UsesExplicitConfigPathFromStructOption(t *testing.T) {
	g := NewWithT(t)

	dir := t.TempDir()
	path := filepath.Join(dir, config.KeyPlatformType)
	g.Expect(os.WriteFile(path, []byte(config.PlatformTypeManagedRhoai), 0o600)).To(Succeed())

	cfg, err := config.Load(config.LoadOptions{ConfigPath: dir})
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(cfg.PlatformType).To(Equal(config.PlatformTypeManagedRhoai))
}

func TestLoad_UsesEnvConfigPathByDefault(t *testing.T) {
	g := NewWithT(t)

	dir := t.TempDir()
	path := filepath.Join(dir, config.KeyPlatformVersion)
	g.Expect(os.WriteFile(path, []byte("4.2.0"), 0o600)).To(Succeed())
	t.Setenv(config.ConfigPathEnvVar, dir)

	cfg, err := config.Load()
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(cfg.PlatformVersion.String()).To(Equal("4.2.0"))
}
