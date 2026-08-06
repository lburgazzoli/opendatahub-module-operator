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
	"testing"
	"testing/fstest"
	"time"

	. "github.com/onsi/gomega"
	"github.com/spf13/cobra"

	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/pkg/config"
)

func TestRegisterFlags_DerivesFlagNamesFromConfigStruct(t *testing.T) {
	g := NewWithT(t)

	cmd := &cobra.Command{Use: "test"}
	v := config.NewViper()

	g.Expect(config.RegisterFlags(cmd, v)).To(Succeed())

	for _, name := range []string{
		"operator-namespace",
		"platform-type",
		"platform-version",
		"controller-metrics-bind-address",
		"controller-leader-election-enabled",
		"internal-postgres-image",
		"databaseprovider-retry-interval",
	} {
		g.Expect(cmd.Flags().Lookup(name)).NotTo(BeNil(), name)
	}
}

func TestLoad_FlagsOverrideEnvAndConfigMap(t *testing.T) {
	g := NewWithT(t)

	t.Setenv("ODH_MODULE_OPERATOR_PLATFORM_TYPE", config.PlatformTypeSelfManagedRhoai)
	t.Setenv("ODH_MODULE_OPERATOR_GRACE_PERIOD", "7m")

	cmd := &cobra.Command{Use: "test"}
	v := config.NewViper()
	g.Expect(config.RegisterFlags(cmd, v)).To(Succeed())
	g.Expect(cmd.ParseFlags([]string{
		"--platform-type", config.PlatformTypeManagedRhoai,
		"--platform-version", "2.1.0",
		"--grace-period", "90s",
		"--controller-leader-election-enabled=false",
		"--databaseprovider-retry-interval", "45s",
	})).To(Succeed())

	cfg, err := config.Load(
		config.LoadOptions{
			Viper: v,
			FS: fstest.MapFS{
				config.KeyPlatformType:                  {Data: []byte(config.DefaultPlatformType)},
				config.KeyGracePeriod:                   {Data: []byte("15m")},
				config.KeyDatabaseProviderRetryInterval: {Data: []byte("3m")},
			},
		},
	)
	g.Expect(err).NotTo(HaveOccurred())

	g.Expect(cfg.PlatformType).To(Equal(config.PlatformTypeManagedRhoai))
	g.Expect(cfg.PlatformVersion.String()).To(Equal("2.1.0"))
	g.Expect(cfg.GracePeriod).To(Equal(90 * time.Second))
	g.Expect(cfg.Controller.LeaderElection.Enabled).To(BeFalse())
	g.Expect(cfg.DatabaseProvider.RetryInterval).To(Equal(45 * time.Second))
}
