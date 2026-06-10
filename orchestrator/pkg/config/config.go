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
	"path/filepath"
	"strings"

	"github.com/blang/semver/v4"
	"github.com/spf13/viper"

	configApi "github.com/lburgazzoli/opendatahub-module-operator/orchestrator/api/config/v1alpha1"
	"github.com/opendatahub-io/opendatahub-operator/v2/api/common"
	ofVersion "github.com/operator-framework/api/pkg/lib/version"
)

const (
	AdminAcksConfigMapName = "opendatahub-admin"

	KeyChartsPath          = "charts-path"
	KeyDistributionName    = "distribution.name"
	KeyDistributionVersion = "distribution.version"

	KeyMetricsBindAddr    = "controller.metrics.bind-address"
	KeyHealthBindAddr     = "controller.health.bind-address"
	KeyLeaderElectEnabled = "controller.leader-election.enabled"
	KeyLeaderElectID      = "controller.leader-election.id"
	KeyZapLevel           = "controller.zap.level"
	KeyZapDevMode         = "controller.zap.dev-mode"
	KeyZapEncoder         = "controller.zap.encoder"
	KeyPprofEnabled       = "controller.pprof.enabled"
	KeyPprofBindAddr      = "controller.pprof.bind-address"

	DefaultNamespace           = "opendatahub"
	DefaultChartsPath          = "/charts"
	DefaultDistributionName    = "unknown"
	DefaultDistributionVersion = "unknown"

	NamespaceEnvVar = "ODH_OPERATOR_NAMESPACE"

	DefaultMetricsBindAddr    = ":8080"
	DefaultHealthBindAddr     = ":8081"
	DefaultLeaderElectEnabled = true
	DefaultLeaderElectID      = "opendatahub-orchestrator-lock"
	DefaultZapLevel           = "info"
	DefaultZapDevMode         = false
	DefaultZapEncoder         = ""
	DefaultPprofEnabled       = false

	ConfigPathEnvVar = "ODH_MODULE_OPERATOR_CONFIGURATION_PATH"
	EnvPrefix        = "ODH_MODULE_OPERATOR"
)

var structuredExtensions = map[string]bool{
	"yaml": true,
	"yml":  true,
	"json": true,
}

type Config struct {
	ChartsPath   string                 `mapstructure:"charts-path"`
	Distribution configApi.Distribution `mapstructure:"distribution"`
	Controller   ControllerConfig       `mapstructure:"controller"`
}

// Namespace returns the orchestrator's namespace, derived from
// ODH_OPERATOR_NAMESPACE (set via downward API from the pod namespace).
// Falls back to DefaultNamespace if unset.
func (c *Config) Namespace() string {
	if ns := os.Getenv(NamespaceEnvVar); ns != "" {
		return ns
	}
	return DefaultNamespace
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

func (c *Config) Release() common.Release {
	rel := common.Release{
		Name: common.Platform(c.Distribution.Name),
	}

	if c.Distribution.Version != "" {
		v, err := semver.ParseTolerant(c.Distribution.Version)
		if err == nil {
			rel.Version = ofVersion.OperatorVersion{Version: v}
		}
	}

	return rel
}

func Load() (*Config, error) {
	var configFS fs.FS

	if configPath := os.Getenv(ConfigPathEnvVar); configPath != "" {
		configFS = os.DirFS(configPath)
	}

	return LoadFromFS(configFS)
}

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
	if err := v.Unmarshal(cfg); err != nil {
		return nil, fmt.Errorf("unmarshaling config: %w", err)
	}

	return cfg, nil
}

func setDefaults(v *viper.Viper) {
	v.SetDefault(KeyChartsPath, DefaultChartsPath)
	v.SetDefault(KeyDistributionName, DefaultDistributionName)
	v.SetDefault(KeyDistributionVersion, DefaultDistributionVersion)

	v.SetDefault(KeyMetricsBindAddr, DefaultMetricsBindAddr)
	v.SetDefault(KeyHealthBindAddr, DefaultHealthBindAddr)
	v.SetDefault(KeyLeaderElectEnabled, DefaultLeaderElectEnabled)
	v.SetDefault(KeyLeaderElectID, DefaultLeaderElectID)
	v.SetDefault(KeyZapLevel, DefaultZapLevel)
	v.SetDefault(KeyZapDevMode, DefaultZapDevMode)
	v.SetDefault(KeyZapEncoder, DefaultZapEncoder)
	v.SetDefault(KeyPprofEnabled, DefaultPprofEnabled)
	v.SetDefault(KeyPprofBindAddr, "")
}

func bindEnv(v *viper.Viper) error {
	v.SetEnvPrefix(EnvPrefix)
	v.SetEnvKeyReplacer(strings.NewReplacer("-", "_", ".", "_"))
	v.AutomaticEnv()

	for _, key := range v.AllKeys() {
		if err := v.BindEnv(key); err != nil {
			return fmt.Errorf("binding env for key %s: %w", key, err)
		}
	}

	return nil
}

func loadFromFS(v *viper.Viper, fsys fs.FS) error {
	entries, err := fs.ReadDir(fsys, ".")
	if err != nil {
		return fmt.Errorf("reading config directory: %w", err)
	}

	tmp := viper.New()

	for _, entry := range entries {
		if entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}

		data, err := fs.ReadFile(fsys, entry.Name())
		if err != nil {
			continue
		}

		ext := strings.TrimPrefix(filepath.Ext(entry.Name()), ".")

		if structuredExtensions[ext] {
			if err := mergeStructuredFile(tmp, entry.Name(), ext, data); err != nil {
				return err
			}
		} else {
			tmp.Set(entry.Name(), strings.TrimSpace(string(data)))
		}
	}

	if err := v.MergeConfigMap(tmp.AllSettings()); err != nil {
		return fmt.Errorf("merging config from filesystem: %w", err)
	}

	return nil
}

func mergeStructuredFile(v *viper.Viper, name string, ext string, data []byte) error {
	fv := viper.New()
	fv.SetConfigType(ext)

	if err := fv.ReadConfig(strings.NewReader(string(data))); err != nil {
		return fmt.Errorf("parsing config file %s: %w", name, err)
	}

	if err := v.MergeConfigMap(fv.AllSettings()); err != nil {
		return fmt.Errorf("merging config from %s: %w", name, err)
	}

	return nil
}
