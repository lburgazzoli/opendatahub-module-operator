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

package config

import (
	"fmt"
	"io/fs"
	"os"
	"time"

	"github.com/blang/semver/v4"
	"github.com/go-viper/mapstructure/v2"
	"github.com/spf13/viper"

	common "github.com/opendatahub-io/odh-platform-utilities/api/common"
	fwapi "github.com/opendatahub-io/odh-platform-utilities/framework/api"
)

// PlatformVersion wraps semver.Version and implements encoding.TextUnmarshaler
// so mapstructure can decode the platformVersion ConfigMap key directly.
type PlatformVersion struct {
	semver.Version
}

// UnmarshalText implements encoding.TextUnmarshaler.
// An empty string decodes to the zero semver (0.0.0) without error.
func (v *PlatformVersion) UnmarshalText(text []byte) error {
	s := string(text)
	if s == "" {
		v.Version = semver.Version{}
		return nil
	}
	sv, err := semver.ParseTolerant(s)
	if err != nil {
		return fmt.Errorf("parsing platform version %q: %w", s, err)
	}
	v.Version = sv
	return nil
}

const (
	KeyOperatorNS      = "operator-namespace"
	KeyPlatformType    = "platformType"
	KeyPlatformVersion = "platformVersion"

	KeyMetricsBindAddr    = "controller.metrics.bind-address"
	KeyHealthBindAddr     = "controller.health.bind-address"
	KeyLeaderElectEnabled = "controller.leader-election.enabled"
	KeyLeaderElectID      = "controller.leader-election.id"
	KeyZapLevel           = "controller.zap.level"
	KeyZapDevMode         = "controller.zap.dev-mode"
	KeyZapEncoder         = "controller.zap.encoder"
	KeyPprofEnabled       = "controller.pprof.enabled"
	KeyPprofBindAddr      = "controller.pprof.bind-address"

	// Embedded provider images and timing (docs/plan.md §7.1/§7.7/§6). Never
	// referenced as literals in controller code -- always through Config.
	// Named "*-image", not "default-*-image": there is no CRD override field
	// for either image (spec.md is explicit: no image override field, ever),
	// so there's nothing for this config value to be a fallback *from* --
	// it's simply the image, and this key is the only way to change it.
	KeyPostgresImage = "embedded.postgres-image"
	KeyPgvectorImage = "embedded.pgvector-image"

	// KeyGracePeriod is a generic grace period used wherever the operator needs
	// to wait before taking a destructive action (Embedded provider idle teardown,
	// claim cleanup retry timeout, etc.).
	KeyGracePeriod = "grace-period"

	// Periodic-retry intervals (docs/plan.md §6), one per reconciler
	// (SchemaClaim, DatabaseClaim, DatabaseProvider, and the DatabaseService
	// module-enablement CR), passed to reconciler.WithDefaultRequeueAfter.
	// Each is independently configurable -- a slow-to-drift claim reconciler
	// and a provider that needs to notice recovered connectivity sooner
	// shouldn't be forced to share one value -- but all four use the same
	// "<type>.retry-interval" key shape and the same RetryConfig field name,
	// so the pattern reads identically across controllers.
	KeySchemaClaimRetryInterval      = "schemaclaim.retry-interval"
	KeyDatabaseClaimRetryInterval    = "databaseclaim.retry-interval"
	KeyDatabaseProviderRetryInterval = "databaseprovider.retry-interval"
	KeyDatabaseServiceRetryInterval  = "databaseservice.retry-interval"

	// PlatformType* are the identifier strings written to the platformType
	// ConfigMap key by the platform operator. They match the switch cases in
	// odhcluster.DetectPlatform and are distinct from the odhcluster.Platform
	// display-name constants ("Open Data Hub", "OpenShift AI Self-Managed", …).
	PlatformTypeOpenDataHub      = "OpenDataHub"
	PlatformTypeSelfManagedRhoai = "SelfManagedRhoai"
	PlatformTypeManagedRhoai     = "ManagedRhoai"

	DefaultOperatorNS      = "odh-db-operator-system"
	DefaultPlatformType    = PlatformTypeOpenDataHub
	DefaultPlatformVersion = ""

	DefaultMetricsBindAddr    = ":8080"
	DefaultHealthBindAddr     = ":8081"
	DefaultLeaderElectEnabled = true
	DefaultLeaderElectID      = "odh-db-operator-lock"
	DefaultZapLevel           = "info"
	DefaultZapDevMode         = false
	DefaultZapEncoder         = ""
	DefaultPprofEnabled       = false

	// DefaultPostgresImage/DefaultPgvectorImage are the community images, not
	// registry.redhat.io -- the latter needs an entitlement pull secret a
	// vanilla kind cluster doesn't have, which would break spec.md's
	// "testable on a plain kind cluster" constraint (docs/plan.md §7.1's
	// "Considered and rejected" note). Admins on entitled clusters (e.g. a
	// connected OpenShift cluster) can repoint either via the mounted
	// ConfigMap (KeyPostgresImage/KeyPgvectorImage above).
	DefaultPostgresImage = "postgres:16"
	DefaultPgvectorImage = "pgvector/pgvector:pg16"

	DefaultGracePeriod = 10 * time.Minute

	// DefaultRetryInterval is the shared compiled default for all four
	// retry-interval keys above. Each key is independently overridable, so an
	// admin can tune one reconciler's cadence without touching the others.
	DefaultRetryInterval = 3 * time.Minute

	// ReleasePlatform is the release name used in status.releases for the
	// platform version handshake.
	ReleasePlatform = "platform"

	// ConfigPathEnvVar is the environment variable that points to the mounted
	// ConfigMap directory (or a single config file).
	ConfigPathEnvVar = "ODH_MODULE_OPERATOR_CONFIGURATION_PATH"

	// EnvPrefix is the prefix for environment variables that override
	// configuration values (e.g. ODH_MODULE_OPERATOR_PLATFORM_TYPE).
	EnvPrefix = "ODH_MODULE_OPERATOR"
)

// Config holds the complete operator configuration.
//
// Values are loaded from (in order of precedence):
//  1. Struct field defaults
//  2. ConfigMap files (from ODH_MODULE_OPERATOR_CONFIGURATION_PATH)
//  3. Environment variables (ODH_MODULE_OPERATOR_ prefix)
//
// Controller-runtime fields use dot-separated ConfigMap keys under
// the "controller." prefix (e.g. "controller.leader-election.enabled").
type Config struct {
	OperatorNamespace string           `mapstructure:"operator-namespace"`
	PlatformType      string           `mapstructure:"platformType"`
	PlatformVersion   PlatformVersion  `mapstructure:"platformVersion"`
	Controller        ControllerConfig `mapstructure:"controller"`
	Embedded          EmbeddedConfig   `mapstructure:"embedded"`
	SchemaClaim       RetryConfig      `mapstructure:"schemaclaim"`
	DatabaseClaim     RetryConfig      `mapstructure:"databaseclaim"`
	DatabaseProvider  RetryConfig      `mapstructure:"databaseprovider"`
	DatabaseService   RetryConfig      `mapstructure:"databaseservice"`
	// GracePeriod is a generic operator-wide timeout used wherever a destructive
	// action should be deferred: Embedded provider idle teardown, claim cleanup
	// retry ceiling, etc. Defaults to DefaultGracePeriod.
	GracePeriod time.Duration `mapstructure:"grace-period"`
}

// EmbeddedConfig holds the Embedded DatabaseProvider's operator-wide image
// defaults (docs/plan.md §7.1). Never referenced as literals in controller code.
// The idle teardown grace period is now the top-level Config.GracePeriod.
type EmbeddedConfig struct {
	PostgresImage string `mapstructure:"postgres-image"`
	PgvectorImage string `mapstructure:"pgvector-image"`
}

// RetryConfig holds one reconciler's periodic-retry interval (docs/plan.md
// §6), passed to reconciler.WithDefaultRequeueAfter. The same field name is
// used across SchemaClaim/DatabaseClaim/DatabaseProvider/DatabaseService on
// Config above, even though each carries its own independently-configured
// value.
type RetryConfig struct {
	RetryInterval time.Duration `mapstructure:"retry-interval"`
}

type ControllerConfig struct {
	Metrics        MetricsConfig        `mapstructure:"metrics"`
	Health         HealthConfig         `mapstructure:"health"`
	LeaderElection LeaderElectionConfig `mapstructure:"leader-election"`
	Zap            ZapConfig            `mapstructure:"zap"`
	Pprof          PprofConfig          `mapstructure:"pprof"`
}

type MetricsConfig struct {
	BindAddress string `mapstructure:"bind-address"`
}

type HealthConfig struct {
	BindAddress string `mapstructure:"bind-address"`
}

type LeaderElectionConfig struct {
	Enabled bool   `mapstructure:"enabled"`
	ID      string `mapstructure:"id"`
}

type ZapConfig struct {
	Level   string `mapstructure:"level"`
	DevMode bool   `mapstructure:"dev-mode"`
	Encoder string `mapstructure:"encoder"`
}

type PprofConfig struct {
	Enabled     bool   `mapstructure:"enabled"`
	BindAddress string `mapstructure:"bind-address"`
}

// ComponentRelease returns the platform handshake entry for status.releases.
func (c *Config) ComponentRelease() common.ComponentRelease {
	return common.ComponentRelease{
		Name:    ReleasePlatform,
		Version: c.PlatformVersion.String(),
	}
}

// PlatformRelease returns the fwapi.Release used by the reconciler and stored
// in m.release. PlatformVersion is already parsed at load time — no error.
func (c *Config) PlatformRelease() fwapi.Release {
	return fwapi.Release{
		Name:    fwapi.Platform(c.PlatformType),
		Version: c.PlatformVersion.Version, // semver.Version embedded in PlatformVersion
	}
}

// Load reads operator configuration from all available sources.
//
// The loading sequence:
//  1. Set defaults
//  2. Read ConfigMap files from ODH_MODULE_OPERATOR_CONFIGURATION_PATH (if set)
//  3. Bind environment variables with the ODH_MODULE_OPERATOR_ prefix
//  4. Unmarshal into the Config struct
func Load() (*Config, error) {
	var configFS fs.FS

	if configPath := os.Getenv(ConfigPathEnvVar); configPath != "" {
		configFS = os.DirFS(configPath)
	}

	return LoadFromFS(configFS)
}

// LoadFromFS reads operator configuration from the given filesystem.
// If fsys is nil, only defaults and environment variables are used.
// This function is the primary entry point for testing.
func LoadFromFS(fsys fs.FS) (*Config, error) {
	v := viper.New()

	setDefaults(v)

	if fsys != nil {
		if err := loadFromFS(v, fsys); err != nil {
			return nil, fmt.Errorf("loading config from filesystem: %w", err)
		}
	}

	if err := bindEnv(v); err != nil {
		return nil, fmt.Errorf("binding env vars: %w", err)
	}

	cfg := &Config{}
	if err := v.Unmarshal(cfg, viper.DecodeHook(
		mapstructure.ComposeDecodeHookFunc(
			mapstructure.TextUnmarshallerHookFunc(),
			mapstructure.StringToTimeDurationHookFunc(),
			mapstructure.StringToSliceHookFunc(","),
		),
	)); err != nil {
		return nil, fmt.Errorf("unmarshaling config: %w", err)
	}

	return cfg, nil
}
