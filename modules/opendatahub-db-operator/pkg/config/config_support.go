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
	"path/filepath"
	"strings"

	"github.com/spf13/viper"
)

// structuredExtensions is the set of file extensions that are parsed as
// structured config (YAML, JSON) rather than simple key-value pairs.
var structuredExtensions = map[string]bool{
	"yaml": true,
	"yml":  true,
	"json": true,
}

func setDefaults(v *viper.Viper) {
	v.SetDefault(KeyOperatorNS, DefaultOperatorNS)
	v.SetDefault(KeyPlatformType, DefaultPlatformType)
	v.SetDefault(KeyPlatformVersion, DefaultPlatformVersion)

	v.SetDefault(KeyMetricsBindAddr, DefaultMetricsBindAddr)
	v.SetDefault(KeyHealthBindAddr, DefaultHealthBindAddr)
	v.SetDefault(KeyLeaderElectEnabled, DefaultLeaderElectEnabled)
	v.SetDefault(KeyLeaderElectID, DefaultLeaderElectID)
	v.SetDefault(KeyZapLevel, DefaultZapLevel)
	v.SetDefault(KeyZapDevMode, DefaultZapDevMode)
	v.SetDefault(KeyZapEncoder, DefaultZapEncoder)
	v.SetDefault(KeyPprofEnabled, DefaultPprofEnabled)
	v.SetDefault(KeyPprofBindAddr, "")

	v.SetDefault(KeyPostgresImage, DefaultPostgresImage)
	v.SetDefault(KeyPgvectorImage, DefaultPgvectorImage)
	v.SetDefault(KeyGracePeriod, DefaultGracePeriod)

	v.SetDefault(KeySchemaClaimRetryInterval, DefaultRetryInterval)
	v.SetDefault(KeyDatabaseClaimRetryInterval, DefaultRetryInterval)
	v.SetDefault(KeyDatabaseProviderRetryInterval, DefaultRetryInterval)
	v.SetDefault(KeyDatabaseServiceRetryInterval, DefaultRetryInterval)
}

func bindEnv(v *viper.Viper) error {
	v.SetEnvPrefix(EnvPrefix)
	v.SetEnvKeyReplacer(strings.NewReplacer("-", "_", ".", "_"))
	v.AutomaticEnv()

	// Explicit BindEnv so Unmarshal picks up env vars.
	// AutomaticEnv only works with Get(), not Unmarshal().
	for _, key := range v.AllKeys() {
		if err := v.BindEnv(key); err != nil {
			return fmt.Errorf("binding env for key %s: %w", key, err)
		}
	}

	return nil
}

// loadFromFS reads all files from the given fs.FS into a temporary viper
// instance, then merges the result into v. Structured files (YAML/JSON)
// are parsed normally. Plain files use the filename as a dot-separated
// key path (e.g. "controller.zap.level" expands to a nested map).
// The single MergeConfigMap at the end writes to viper's config layer,
// so environment variables still take precedence.
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

// mergeStructuredFile parses a YAML/JSON file and merges its keys into viper.
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
