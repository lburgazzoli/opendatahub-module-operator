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
	"context"
	"fmt"
	"io/fs"
	"os"

	"github.com/blang/semver/v4"
	"github.com/go-viper/mapstructure/v2"
	"github.com/spf13/viper"

	common "github.com/opendatahub-io/odh-platform-utilities/api/common"
	fwapi "github.com/opendatahub-io/odh-platform-utilities/framework/api"
	fwactions "github.com/opendatahub-io/odh-platform-utilities/framework/controller/actions"
	fwtypes "github.com/opendatahub-io/odh-platform-utilities/framework/controller/types"
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
	KeyManifestsPath   = "manifests-path"
	KeyApplicationsNS  = "applications-namespace"
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

	// PlatformType* are the identifier strings written to the platformType
	// ConfigMap key by the platform operator. They match the switch cases in
	// odhcluster.DetectPlatform and are distinct from the odhcluster.Platform
	// display-name constants ("Open Data Hub", "OpenShift AI Self-Managed", …).
	PlatformTypeOpenDataHub      = "OpenDataHub"
	PlatformTypeSelfManagedRhoai = "SelfManagedRhoai"
	PlatformTypeManagedRhoai     = "ManagedRhoai"

	DefaultApplicationsNS  = "opendatahub"
	DefaultPlatformType    = PlatformTypeOpenDataHub
	DefaultPlatformVersion = ""

	DefaultMetricsBindAddr    = ":8080"
	DefaultHealthBindAddr     = ":8081"
	DefaultLeaderElectEnabled = true
	DefaultLeaderElectID      = "odh-trustyai-lock"
	DefaultZapLevel           = "info"
	DefaultZapDevMode         = false
	DefaultZapEncoder         = ""
	DefaultPprofEnabled       = false
	ReleasePlatform           = "platform"

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
	ManifestsPath         string           `mapstructure:"manifests-path"`
	ApplicationsNamespace string           `mapstructure:"applications-namespace"`
	PlatformType          string           `mapstructure:"platformType"`
	PlatformVersion       PlatformVersion  `mapstructure:"platformVersion"`
	Controller            ControllerConfig `mapstructure:"controller"`
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
		Version: c.PlatformVersion.Version,
	}
}

func ApplicationsNamespaceGetter(cfg *Config) fwactions.Getter[string] {
	return func(_ context.Context, _ *fwtypes.ReconciliationRequest) (string, error) {
		return cfg.ApplicationsNamespace, nil
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
