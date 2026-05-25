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
	"encoding/json"
	"testing"
	"testing/fstest"

	. "github.com/onsi/gomega"
	"sigs.k8s.io/yaml"

	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-mymodule-operator/pkg/config"
)

const (
	platformTypeODH         = "OpenDataHub"
	platformTypeSelfManaged = "SelfManagedRHOAI"
	platformTypeManagedRhai = "ManagedRhoai"
	platformTypeXKS         = "XKS"

	platformVersion100 = "1.0.0"
	platformVersion200 = "2.0.0"
	platformVersion220 = "2.20.0"
	platformVersion225 = "2.25.0"
	platformVersion300 = "3.0.0"

	namespaceRHAI     = "redhat-ods-applications"
	namespaceBase     = "base-ns"
	namespaceOverride = "override-ns"
	namespaceCustom   = "custom"

	metricsAddr9090 = ":9090"

	leaderElectionIDCustom = "custom-lock"

	manifestsPathOpt = "/opt/manifests"

	fileConfigYAML   = "config.yaml"
	fileConfigJSON   = "config.json"
	fileSettingsYML  = "settings.yml"
	fileBaseYAML     = "01-base.yaml"
	fileOverrideYAML = "02-override.yaml"
)

func TestLoadFromFS_Defaults(t *testing.T) {
	g := NewWithT(t)

	cfg, err := config.LoadFromFS(nil)
	g.Expect(err).NotTo(HaveOccurred())

	g.Expect(cfg.PlatformType).To(Equal(config.DefaultPlatformType))
	g.Expect(cfg.PlatformVersion).To(Equal(config.DefaultPlatformVersion))
	g.Expect(cfg.MetricsAddr).To(Equal(config.DefaultMetricsAddr))
	g.Expect(cfg.HealthProbeAddr).To(Equal(config.DefaultHealthProbeAddr))
	g.Expect(cfg.LeaderElect).To(BeTrue())
	g.Expect(cfg.LeaderElectionID).To(Equal(config.DefaultLeaderElectionID))
	g.Expect(cfg.ApplicationsNamespace).To(Equal(config.DefaultApplicationsNS))
	g.Expect(cfg.ManifestsPath).To(BeEmpty())
	g.Expect(cfg.PprofAddr).To(BeEmpty())
}

func TestLoadFromFS_FlatFiles(t *testing.T) {
	g := NewWithT(t)

	fsys := fstest.MapFS{
		config.KeyPlatformType:    {Data: []byte(platformTypeSelfManaged)},
		config.KeyPlatformVersion: {Data: []byte(platformVersion225)},
	}

	cfg, err := config.LoadFromFS(fsys)
	g.Expect(err).NotTo(HaveOccurred())

	g.Expect(cfg.PlatformType).To(Equal(platformTypeSelfManaged))
	g.Expect(cfg.PlatformVersion).To(Equal(platformVersion225))
	g.Expect(cfg.MetricsAddr).To(Equal(config.DefaultMetricsAddr))
	g.Expect(cfg.LeaderElect).To(BeTrue())
}

func TestLoadFromFS_FlatFilesWithWhitespace(t *testing.T) {
	g := NewWithT(t)

	fsys := fstest.MapFS{
		config.KeyPlatformType:    {Data: []byte("  " + platformTypeODH + "  \n")},
		config.KeyPlatformVersion: {Data: []byte("\t" + platformVersion300 + "\n")},
	}

	cfg, err := config.LoadFromFS(fsys)
	g.Expect(err).NotTo(HaveOccurred())

	g.Expect(cfg.PlatformType).To(Equal(platformTypeODH))
	g.Expect(cfg.PlatformVersion).To(Equal(platformVersion300))
}

func TestLoadFromFS_YAMLFile(t *testing.T) {
	g := NewWithT(t)

	fsys := fstest.MapFS{
		fileConfigYAML: {Data: toYAML(map[string]string{
			config.KeyPlatformType:    platformTypeManagedRhai,
			config.KeyPlatformVersion: platformVersion220,
			config.KeyApplicationsNS:  namespaceRHAI,
		})},
	}

	cfg, err := config.LoadFromFS(fsys)
	g.Expect(err).NotTo(HaveOccurred())

	g.Expect(cfg.PlatformType).To(Equal(platformTypeManagedRhai))
	g.Expect(cfg.PlatformVersion).To(Equal(platformVersion220))
	g.Expect(cfg.ApplicationsNamespace).To(Equal(namespaceRHAI))
}

func TestLoadFromFS_MixedFiles(t *testing.T) {
	g := NewWithT(t)

	fsys := fstest.MapFS{
		config.KeyPlatformType:    {Data: []byte(platformTypeODH)},
		config.KeyPlatformVersion: {Data: []byte(platformVersion225)},
		fileConfigYAML: {Data: toYAML(map[string]string{
			config.KeyApplicationsNS: namespaceCustom,
			config.KeyManifestsPath:  manifestsPathOpt,
		})},
	}

	cfg, err := config.LoadFromFS(fsys)
	g.Expect(err).NotTo(HaveOccurred())

	g.Expect(cfg.PlatformType).To(Equal(platformTypeODH))
	g.Expect(cfg.PlatformVersion).To(Equal(platformVersion225))
	g.Expect(cfg.ApplicationsNamespace).To(Equal(namespaceCustom))
	g.Expect(cfg.ManifestsPath).To(Equal(manifestsPathOpt))
}

func TestLoadFromFS_SkipsDotFiles(t *testing.T) {
	g := NewWithT(t)

	fsys := fstest.MapFS{
		".hidden":              {Data: []byte("should-be-ignored")},
		"..data":               {Data: []byte("k8s-projected-volume-symlink")},
		config.KeyPlatformType: {Data: []byte(platformTypeODH)},
	}

	cfg, err := config.LoadFromFS(fsys)
	g.Expect(err).NotTo(HaveOccurred())

	g.Expect(cfg.PlatformType).To(Equal(platformTypeODH))
}

func TestLoadFromFS_EnvOverride(t *testing.T) {
	g := NewWithT(t)

	t.Setenv(config.EnvPrefix+"_PLATFORM_TYPE", platformTypeXKS)

	fsys := fstest.MapFS{
		config.KeyPlatformType: {Data: []byte(platformTypeODH)},
	}

	cfg, err := config.LoadFromFS(fsys)
	g.Expect(err).NotTo(HaveOccurred())

	g.Expect(cfg.PlatformType).To(Equal(platformTypeXKS))
}

func TestLoadFromFS_EmptyFS(t *testing.T) {
	g := NewWithT(t)

	cfg, err := config.LoadFromFS(fstest.MapFS{})
	g.Expect(err).NotTo(HaveOccurred())

	g.Expect(cfg.PlatformType).To(Equal(config.DefaultPlatformType))
	g.Expect(cfg.PlatformVersion).To(Equal(config.DefaultPlatformVersion))
	g.Expect(cfg.MetricsAddr).To(Equal(config.DefaultMetricsAddr))
}

func TestLoadFromFS_StructuredYAMLOverridesDefaults(t *testing.T) {
	g := NewWithT(t)

	fsys := fstest.MapFS{
		fileConfigYAML: {Data: toYAML(map[string]string{
			config.KeyLeaderElect:      "false",
			config.KeyLeaderElectionID: leaderElectionIDCustom,
			config.KeyMetricsAddr:      metricsAddr9090,
		})},
	}

	cfg, err := config.LoadFromFS(fsys)
	g.Expect(err).NotTo(HaveOccurred())

	g.Expect(cfg.LeaderElect).To(BeFalse())
	g.Expect(cfg.LeaderElectionID).To(Equal(leaderElectionIDCustom))
	g.Expect(cfg.MetricsAddr).To(Equal(metricsAddr9090))
	g.Expect(cfg.PlatformType).To(Equal(config.DefaultPlatformType))
}

func TestLoadFromFS_StructuredJSONFile(t *testing.T) {
	g := NewWithT(t)

	fsys := fstest.MapFS{
		fileConfigJSON: {Data: toJSON(map[string]string{
			config.KeyPlatformType:    platformTypeXKS,
			config.KeyPlatformVersion: platformVersion100,
		})},
	}

	cfg, err := config.LoadFromFS(fsys)
	g.Expect(err).NotTo(HaveOccurred())

	g.Expect(cfg.PlatformType).To(Equal(platformTypeXKS))
	g.Expect(cfg.PlatformVersion).To(Equal(platformVersion100))
}

func TestLoadFromFS_StructuredYMLExtension(t *testing.T) {
	g := NewWithT(t)

	fsys := fstest.MapFS{
		fileSettingsYML: {Data: toYAML(map[string]string{
			config.KeyPlatformType:   platformTypeSelfManaged,
			config.KeyApplicationsNS: namespaceCustom,
		})},
	}

	cfg, err := config.LoadFromFS(fsys)
	g.Expect(err).NotTo(HaveOccurred())

	g.Expect(cfg.PlatformType).To(Equal(platformTypeSelfManaged))
	g.Expect(cfg.ApplicationsNamespace).To(Equal(namespaceCustom))
}

func TestLoadFromFS_InvalidYAMLReturnsError(t *testing.T) {
	g := NewWithT(t)

	fsys := fstest.MapFS{
		fileConfigYAML: {Data: []byte("{{invalid yaml")},
	}

	_, err := config.LoadFromFS(fsys)
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("parsing config file config.yaml"))
}

func TestLoadFromFS_InvalidJSONReturnsError(t *testing.T) {
	g := NewWithT(t)

	fsys := fstest.MapFS{
		fileConfigJSON: {Data: []byte("{not json")},
	}

	_, err := config.LoadFromFS(fsys)
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("parsing config file config.json"))
}

func TestLoadFromFS_NonStructuredExtensionTreatedAsKeyValue(t *testing.T) {
	g := NewWithT(t)

	fsys := fstest.MapFS{
		config.KeyPlatformType: {Data: []byte(platformTypeODH)},
		"some-setting.txt":     {Data: []byte("some-value")},
	}

	cfg, err := config.LoadFromFS(fsys)
	g.Expect(err).NotTo(HaveOccurred())

	g.Expect(cfg.PlatformType).To(Equal(platformTypeODH))
}

func TestLoadFromFS_FlatFileOverridesStructuredFile(t *testing.T) {
	g := NewWithT(t)

	// Both set platform-type. Directory entries are in lexical order:
	// fileConfigYAML < "platform-type", so the flat file wins (processed last).
	fsys := fstest.MapFS{
		fileConfigYAML: {Data: toYAML(map[string]string{
			config.KeyPlatformType: "FromYAML",
		})},
		config.KeyPlatformType: {Data: []byte(platformTypeSelfManaged)},
	}

	cfg, err := config.LoadFromFS(fsys)
	g.Expect(err).NotTo(HaveOccurred())

	g.Expect(cfg.PlatformType).To(Equal(platformTypeSelfManaged))
}

func TestLoadFromFS_MultipleStructuredFilesMerge(t *testing.T) {
	g := NewWithT(t)

	fsys := fstest.MapFS{
		fileBaseYAML: {Data: toYAML(map[string]string{
			config.KeyPlatformType:    platformTypeODH,
			config.KeyPlatformVersion: platformVersion100,
			config.KeyApplicationsNS:  namespaceBase,
		})},
		fileOverrideYAML: {Data: toYAML(map[string]string{
			config.KeyPlatformVersion: platformVersion200,
			config.KeyApplicationsNS:  namespaceOverride,
		})},
	}

	cfg, err := config.LoadFromFS(fsys)
	g.Expect(err).NotTo(HaveOccurred())

	g.Expect(cfg.PlatformType).To(Equal(platformTypeODH))
	g.Expect(cfg.PlatformVersion).To(Equal(platformVersion200))
	g.Expect(cfg.ApplicationsNamespace).To(Equal(namespaceOverride))
}

func TestLoadFromFS_EmptyFlatFileKeepsDefault(t *testing.T) {
	g := NewWithT(t)

	fsys := fstest.MapFS{
		config.KeyPlatformType: {Data: []byte("")},
	}

	cfg, err := config.LoadFromFS(fsys)
	g.Expect(err).NotTo(HaveOccurred())

	g.Expect(cfg.PlatformType).To(BeEmpty())
}

func TestLoadFromFS_EmptyStructuredFileKeepsDefaults(t *testing.T) {
	g := NewWithT(t)

	fsys := fstest.MapFS{
		fileConfigYAML: {Data: []byte("")},
	}

	cfg, err := config.LoadFromFS(fsys)
	g.Expect(err).NotTo(HaveOccurred())

	g.Expect(cfg.PlatformType).To(Equal(config.DefaultPlatformType))
	g.Expect(cfg.MetricsAddr).To(Equal(config.DefaultMetricsAddr))
}

func TestLoadFromFS_EnvOverridesStructuredFile(t *testing.T) {
	g := NewWithT(t)

	t.Setenv(config.EnvPrefix+"_PLATFORM_VERSION", platformVersion300)

	fsys := fstest.MapFS{
		fileConfigYAML: {Data: toYAML(map[string]string{
			config.KeyPlatformType:    platformTypeODH,
			config.KeyPlatformVersion: platformVersion100,
		})},
	}

	cfg, err := config.LoadFromFS(fsys)
	g.Expect(err).NotTo(HaveOccurred())

	g.Expect(cfg.PlatformType).To(Equal(platformTypeODH))
	g.Expect(cfg.PlatformVersion).To(Equal(platformVersion300))
}

func toYAML(m map[string]string) []byte {
	data, err := yaml.Marshal(m)
	if err != nil {
		panic(err)
	}

	return data
}

func toJSON(m map[string]string) []byte {
	data, err := json.Marshal(m)
	if err != nil {
		panic(err)
	}

	return data
}
