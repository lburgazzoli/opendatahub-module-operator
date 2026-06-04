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
	. "github.com/onsi/gomega/gstruct"
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

	g.Expect(cfg).To(PointTo(MatchFields(IgnoreExtras, Fields{
		"PlatformName":          Equal(config.DefaultPlatformName),
		"PlatformVersion":       Equal(config.DefaultPlatformVersion),
		"ApplicationsNamespace": Equal(config.DefaultApplicationsNS),
		"ManifestsPath":         BeEmpty(),
	})))

	g.Expect(cfg.Controller.Metrics).To(MatchAllFields(Fields{
		"BindAddress": Equal(config.DefaultMetricsBindAddr),
	}))

	g.Expect(cfg.Controller.Health).To(MatchAllFields(Fields{
		"BindAddress": Equal(config.DefaultHealthBindAddr),
	}))

	g.Expect(cfg.Controller.LeaderElection).To(MatchAllFields(Fields{
		"Enabled": BeTrue(),
		"ID":      Equal(config.DefaultLeaderElectID),
	}))

	g.Expect(cfg.Controller.Webhook).To(MatchAllFields(Fields{
		"Enabled": Equal(config.DefaultWebhookEnabled),
		"Port":    Equal(config.DefaultWebhookPort),
		"CertDir": Equal(config.DefaultWebhookCertDir),
	}))

	g.Expect(cfg.Controller.Zap).To(MatchAllFields(Fields{
		"Level": Equal(config.DefaultZapLevel),
	}))

	g.Expect(cfg.Controller.Pprof).To(MatchAllFields(Fields{
		"Enabled":     Equal(config.DefaultPprofEnabled),
		"BindAddress": BeEmpty(),
	}))
}

func TestLoadFromFS_FlatFiles(t *testing.T) {
	g := NewWithT(t)

	fsys := fstest.MapFS{
		config.KeyPlatformName:    {Data: []byte(platformTypeSelfManaged)},
		config.KeyPlatformVersion: {Data: []byte(platformVersion225)},
	}

	cfg, err := config.LoadFromFS(fsys)
	g.Expect(err).NotTo(HaveOccurred())

	g.Expect(cfg.PlatformName).To(Equal(platformTypeSelfManaged))
	g.Expect(cfg.PlatformVersion).To(Equal(platformVersion225))
	g.Expect(cfg.Controller.Metrics.BindAddress).To(Equal(config.DefaultMetricsBindAddr))
	g.Expect(cfg.Controller.LeaderElection.Enabled).To(BeTrue())
}

func TestLoadFromFS_FlatFilesWithWhitespace(t *testing.T) {
	g := NewWithT(t)

	fsys := fstest.MapFS{
		config.KeyPlatformName:    {Data: []byte("  " + platformTypeODH + "  \n")},
		config.KeyPlatformVersion: {Data: []byte("\t" + platformVersion300 + "\n")},
	}

	cfg, err := config.LoadFromFS(fsys)
	g.Expect(err).NotTo(HaveOccurred())

	g.Expect(cfg.PlatformName).To(Equal(platformTypeODH))
	g.Expect(cfg.PlatformVersion).To(Equal(platformVersion300))
}

func TestLoadFromFS_YAMLFile(t *testing.T) {
	g := NewWithT(t)

	fsys := fstest.MapFS{
		fileConfigYAML: {Data: toYAML(map[string]string{
			config.KeyPlatformName:    platformTypeManagedRhai,
			config.KeyPlatformVersion: platformVersion220,
			config.KeyApplicationsNS:  namespaceRHAI,
		})},
	}

	cfg, err := config.LoadFromFS(fsys)
	g.Expect(err).NotTo(HaveOccurred())

	g.Expect(cfg.PlatformName).To(Equal(platformTypeManagedRhai))
	g.Expect(cfg.PlatformVersion).To(Equal(platformVersion220))
	g.Expect(cfg.ApplicationsNamespace).To(Equal(namespaceRHAI))
}

func TestLoadFromFS_MixedFiles(t *testing.T) {
	g := NewWithT(t)

	fsys := fstest.MapFS{
		config.KeyPlatformName:    {Data: []byte(platformTypeODH)},
		config.KeyPlatformVersion: {Data: []byte(platformVersion225)},
		fileConfigYAML: {Data: toYAML(map[string]string{
			config.KeyApplicationsNS: namespaceCustom,
			config.KeyManifestsPath:  manifestsPathOpt,
		})},
	}

	cfg, err := config.LoadFromFS(fsys)
	g.Expect(err).NotTo(HaveOccurred())

	g.Expect(cfg).To(PointTo(MatchFields(IgnoreExtras, Fields{
		"PlatformName":          Equal(platformTypeODH),
		"PlatformVersion":       Equal(platformVersion225),
		"ApplicationsNamespace": Equal(namespaceCustom),
		"ManifestsPath":         Equal(manifestsPathOpt),
	})))
}

func TestLoadFromFS_SkipsDotFiles(t *testing.T) {
	g := NewWithT(t)

	fsys := fstest.MapFS{
		".hidden":              {Data: []byte("should-be-ignored")},
		"..data":               {Data: []byte("k8s-projected-volume-symlink")},
		config.KeyPlatformName: {Data: []byte(platformTypeODH)},
	}

	cfg, err := config.LoadFromFS(fsys)
	g.Expect(err).NotTo(HaveOccurred())

	g.Expect(cfg.PlatformName).To(Equal(platformTypeODH))
}

func TestLoadFromFS_EnvOverride(t *testing.T) {
	g := NewWithT(t)

	t.Setenv(config.EnvPrefix+"_PLATFORM_NAME", platformTypeXKS)

	fsys := fstest.MapFS{
		config.KeyPlatformName: {Data: []byte(platformTypeODH)},
	}

	cfg, err := config.LoadFromFS(fsys)
	g.Expect(err).NotTo(HaveOccurred())

	g.Expect(cfg.PlatformName).To(Equal(platformTypeXKS))
}

func TestLoadFromFS_EmptyFS(t *testing.T) {
	g := NewWithT(t)

	cfg, err := config.LoadFromFS(fstest.MapFS{})
	g.Expect(err).NotTo(HaveOccurred())

	g.Expect(cfg.PlatformName).To(Equal(config.DefaultPlatformName))
	g.Expect(cfg.PlatformVersion).To(Equal(config.DefaultPlatformVersion))
	g.Expect(cfg.Controller.Metrics.BindAddress).To(Equal(config.DefaultMetricsBindAddr))
}

func TestLoadFromFS_StructuredYAMLOverridesDefaults(t *testing.T) {
	g := NewWithT(t)

	fsys := fstest.MapFS{
		fileConfigYAML: {Data: []byte(
			"controller:\n" +
				"  leader-election:\n" +
				"    enabled: false\n" +
				"    id: " + leaderElectionIDCustom + "\n" +
				"  metrics:\n" +
				"    bind-address: \"" + metricsAddr9090 + "\"\n",
		)},
	}

	cfg, err := config.LoadFromFS(fsys)
	g.Expect(err).NotTo(HaveOccurred())

	g.Expect(cfg.Controller.LeaderElection).To(MatchAllFields(Fields{
		"Enabled": BeFalse(),
		"ID":      Equal(leaderElectionIDCustom),
	}))
	g.Expect(cfg.Controller.Metrics.BindAddress).To(Equal(metricsAddr9090))
	g.Expect(cfg.PlatformName).To(Equal(config.DefaultPlatformName))
}

func TestLoadFromFS_StructuredJSONFile(t *testing.T) {
	g := NewWithT(t)

	fsys := fstest.MapFS{
		fileConfigJSON: {Data: toJSON(map[string]string{
			config.KeyPlatformName:    platformTypeXKS,
			config.KeyPlatformVersion: platformVersion100,
		})},
	}

	cfg, err := config.LoadFromFS(fsys)
	g.Expect(err).NotTo(HaveOccurred())

	g.Expect(cfg.PlatformName).To(Equal(platformTypeXKS))
	g.Expect(cfg.PlatformVersion).To(Equal(platformVersion100))
}

func TestLoadFromFS_StructuredYMLExtension(t *testing.T) {
	g := NewWithT(t)

	fsys := fstest.MapFS{
		fileSettingsYML: {Data: toYAML(map[string]string{
			config.KeyPlatformName:   platformTypeSelfManaged,
			config.KeyApplicationsNS: namespaceCustom,
		})},
	}

	cfg, err := config.LoadFromFS(fsys)
	g.Expect(err).NotTo(HaveOccurred())

	g.Expect(cfg.PlatformName).To(Equal(platformTypeSelfManaged))
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
		config.KeyPlatformName: {Data: []byte(platformTypeODH)},
		"some-setting.txt":     {Data: []byte("some-value")},
	}

	cfg, err := config.LoadFromFS(fsys)
	g.Expect(err).NotTo(HaveOccurred())

	g.Expect(cfg.PlatformName).To(Equal(platformTypeODH))
}

func TestLoadFromFS_FlatFileOverridesStructuredFile(t *testing.T) {
	g := NewWithT(t)

	fsys := fstest.MapFS{
		fileConfigYAML: {Data: toYAML(map[string]string{
			config.KeyPlatformName: "FromYAML",
		})},
		config.KeyPlatformName: {Data: []byte(platformTypeSelfManaged)},
	}

	cfg, err := config.LoadFromFS(fsys)
	g.Expect(err).NotTo(HaveOccurred())

	g.Expect(cfg.PlatformName).To(Equal(platformTypeSelfManaged))
}

func TestLoadFromFS_MultipleStructuredFilesMerge(t *testing.T) {
	g := NewWithT(t)

	fsys := fstest.MapFS{
		fileBaseYAML: {Data: toYAML(map[string]string{
			config.KeyPlatformName:    platformTypeODH,
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

	g.Expect(cfg).To(PointTo(MatchFields(IgnoreExtras, Fields{
		"PlatformName":          Equal(platformTypeODH),
		"PlatformVersion":       Equal(platformVersion200),
		"ApplicationsNamespace": Equal(namespaceOverride),
	})))
}

func TestLoadFromFS_EmptyFlatFileKeepsDefault(t *testing.T) {
	g := NewWithT(t)

	fsys := fstest.MapFS{
		config.KeyPlatformName: {Data: []byte("")},
	}

	cfg, err := config.LoadFromFS(fsys)
	g.Expect(err).NotTo(HaveOccurred())

	g.Expect(cfg.PlatformName).To(BeEmpty())
}

func TestLoadFromFS_EmptyStructuredFileKeepsDefaults(t *testing.T) {
	g := NewWithT(t)

	fsys := fstest.MapFS{
		fileConfigYAML: {Data: []byte("")},
	}

	cfg, err := config.LoadFromFS(fsys)
	g.Expect(err).NotTo(HaveOccurred())

	g.Expect(cfg.PlatformName).To(Equal(config.DefaultPlatformName))
	g.Expect(cfg.Controller.Metrics.BindAddress).To(Equal(config.DefaultMetricsBindAddr))
}

func TestLoadFromFS_EnvOverridesStructuredFile(t *testing.T) {
	g := NewWithT(t)

	t.Setenv(config.EnvPrefix+"_PLATFORM_VERSION", platformVersion300)

	fsys := fstest.MapFS{
		fileConfigYAML: {Data: toYAML(map[string]string{
			config.KeyPlatformName:    platformTypeODH,
			config.KeyPlatformVersion: platformVersion100,
		})},
	}

	cfg, err := config.LoadFromFS(fsys)
	g.Expect(err).NotTo(HaveOccurred())

	g.Expect(cfg.PlatformName).To(Equal(platformTypeODH))
	g.Expect(cfg.PlatformVersion).To(Equal(platformVersion300))
}

func TestLoadFromFS_DottedFlatFiles(t *testing.T) {
	g := NewWithT(t)

	fsys := fstest.MapFS{
		"controller.zap.level":     {Data: []byte("warn")},
		"controller.pprof.enabled": {Data: []byte("true")},
	}

	cfg, err := config.LoadFromFS(fsys)
	g.Expect(err).NotTo(HaveOccurred())

	g.Expect(cfg.Controller.Zap).To(MatchAllFields(Fields{
		"Level": Equal("warn"),
	}))
	g.Expect(cfg.Controller.Pprof).To(MatchFields(IgnoreExtras, Fields{
		"Enabled": BeTrue(),
	}))
	g.Expect(cfg.PlatformName).To(Equal(config.DefaultPlatformName))
}

func TestLoadFromFS_DottedKeyWithWhitespace(t *testing.T) {
	g := NewWithT(t)

	fsys := fstest.MapFS{
		"controller.zap.level":     {Data: []byte("  debug \n")},
		"controller.pprof.enabled": {Data: []byte("\ttrue\n")},
	}

	cfg, err := config.LoadFromFS(fsys)
	g.Expect(err).NotTo(HaveOccurred())

	g.Expect(cfg.Controller.Zap.Level).To(Equal("debug"))
	g.Expect(cfg.Controller.Pprof.Enabled).To(BeTrue())
}

func TestLoadFromFS_MixedFlatAndDottedFiles(t *testing.T) {
	g := NewWithT(t)

	fsys := fstest.MapFS{
		config.KeyPlatformName:     {Data: []byte(platformTypeODH)},
		"controller.zap.level":     {Data: []byte("info")},
		"controller.pprof.enabled": {Data: []byte("true")},
	}

	cfg, err := config.LoadFromFS(fsys)
	g.Expect(err).NotTo(HaveOccurred())

	g.Expect(cfg.PlatformName).To(Equal(platformTypeODH))
	g.Expect(cfg.Controller.Zap.Level).To(Equal("info"))
	g.Expect(cfg.Controller.Pprof.Enabled).To(BeTrue())
}

func TestLoadFromFS_DottedFlatFileOverridesStructuredFile(t *testing.T) {
	g := NewWithT(t)

	fsys := fstest.MapFS{
		fileConfigYAML:         {Data: []byte("controller:\n  zap:\n    level: info\n")},
		"controller.zap.level": {Data: []byte("debug")},
	}

	cfg, err := config.LoadFromFS(fsys)
	g.Expect(err).NotTo(HaveOccurred())

	g.Expect(cfg.Controller.Zap.Level).To(Equal("debug"))
}

func TestLoadFromFS_NestedYAMLEquivalentToDottedFile(t *testing.T) {
	g := NewWithT(t)

	cfgYAML, err := config.LoadFromFS(fstest.MapFS{
		fileConfigYAML: {Data: []byte("controller:\n  zap:\n    level: warn\n")},
	})
	g.Expect(err).NotTo(HaveOccurred())

	cfgDotted, err := config.LoadFromFS(fstest.MapFS{
		"controller.zap.level": {Data: []byte("warn")},
	})
	g.Expect(err).NotTo(HaveOccurred())

	g.Expect(cfgDotted.Controller.Zap).To(Equal(cfgYAML.Controller.Zap))
}

func TestLoadFromFS_NestedYAMLPartiallyOverriddenByDottedFile(t *testing.T) {
	g := NewWithT(t)

	fsys := fstest.MapFS{
		fileConfigYAML: {Data: []byte(
			"controller:\n  zap:\n    level: info\n  pprof:\n    enabled: false\n",
		)},
		"controller.zap.level": {Data: []byte("error")},
	}

	cfg, err := config.LoadFromFS(fsys)
	g.Expect(err).NotTo(HaveOccurred())

	g.Expect(cfg.Controller.Zap.Level).To(Equal("error"))
	g.Expect(cfg.Controller.Pprof.Enabled).To(BeFalse())
}

func TestLoadFromFS_NestedYAMLWithFlatKeysAndDottedFiles(t *testing.T) {
	g := NewWithT(t)

	fsys := fstest.MapFS{
		fileConfigYAML: {Data: toYAML(map[string]string{
			config.KeyPlatformName:   platformTypeSelfManaged,
			config.KeyApplicationsNS: namespaceRHAI,
		})},
		"controller.zap.level":     {Data: []byte("warn")},
		"controller.pprof.enabled": {Data: []byte("true")},
	}

	cfg, err := config.LoadFromFS(fsys)
	g.Expect(err).NotTo(HaveOccurred())

	g.Expect(cfg.PlatformName).To(Equal(platformTypeSelfManaged))
	g.Expect(cfg.ApplicationsNamespace).To(Equal(namespaceRHAI))
	g.Expect(cfg.Controller.Zap.Level).To(Equal("warn"))
	g.Expect(cfg.Controller.Pprof.Enabled).To(BeTrue())
}

func TestLoadFromFS_EnvOverridesDottedKey(t *testing.T) {
	g := NewWithT(t)

	t.Setenv(config.EnvPrefix+"_CONTROLLER_ZAP_LEVEL", "error")

	cfg, err := config.LoadFromFS(fstest.MapFS{
		"controller.zap.level": {Data: []byte("info")},
	})
	g.Expect(err).NotTo(HaveOccurred())

	g.Expect(cfg.Controller.Zap.Level).To(Equal("error"))
}

func TestLoadFromFS_EnvOverridesDottedKeyNoFile(t *testing.T) {
	g := NewWithT(t)

	t.Setenv(config.EnvPrefix+"_CONTROLLER_ZAP_LEVEL", "debug")

	cfg, err := config.LoadFromFS(nil)
	g.Expect(err).NotTo(HaveOccurred())

	g.Expect(cfg.Controller.Zap.Level).To(Equal("debug"))
}

func TestLoadFromFS_MixedDashDotAndEnvOverride(t *testing.T) {
	g := NewWithT(t)

	t.Setenv(config.EnvPrefix+"_CONTROLLER_PPROF_ENABLED", "true")
	t.Setenv(config.EnvPrefix+"_PLATFORM_NAME", platformTypeXKS)
	t.Setenv(config.EnvPrefix+"_CONTROLLER_ZAP_LEVEL", "error")

	fsys := fstest.MapFS{
		config.KeyPlatformName:     {Data: []byte(platformTypeODH)},
		config.KeyPlatformVersion:  {Data: []byte(platformVersion225)},
		"controller.zap.level":     {Data: []byte("warn")},
		"controller.pprof.enabled": {Data: []byte("false")},
	}

	cfg, err := config.LoadFromFS(fsys)
	g.Expect(err).NotTo(HaveOccurred())

	g.Expect(cfg).To(PointTo(MatchFields(IgnoreExtras, Fields{
		"PlatformName":    Equal(platformTypeXKS),
		"PlatformVersion": Equal(platformVersion225),
	})))
	g.Expect(cfg.Controller.Zap.Level).To(Equal("error"))
	g.Expect(cfg.Controller.Pprof.Enabled).To(BeTrue())
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
