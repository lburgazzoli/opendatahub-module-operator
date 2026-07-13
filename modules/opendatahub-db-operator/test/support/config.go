package support

import (
	"os"
	"strings"
	"time"

	"github.com/spf13/viper"

	moduleconfig "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/pkg/config"
	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/test/support/cluster"
)

const (
	DefaultClusterType                 = cluster.TypeK3s
	DefaultEventuallyTimeout           = 90 * time.Second
	DefaultEventuallyPollingInterval   = 2 * time.Second
	DefaultConsistentlyPollingInterval = 2 * time.Second
	DefaultOperatorNamespace           = "odh-db-operator-system"
	DefaultIntegrationTestNamespace    = "odh-db-operator-integration"

	testConfigEnvPrefix = "ODH_MODULE_OPERATOR_TEST"

	keyClusterType                 = "cluster.type"
	keyEventuallyTimeout           = "gomega.eventually-timeout"
	keyEventuallyPollingInterval   = "gomega.eventually-polling-interval"
	keyConsistentlyPollingInterval = "gomega.consistently-polling-interval"
)

type Config struct {
	Cluster ClusterConfig `mapstructure:"cluster"`
	Gomega  GomegaConfig  `mapstructure:"gomega"`
}

type ClusterConfig struct {
	Type cluster.Type `mapstructure:"type"`
}

type GomegaConfig struct {
	EventuallyTimeout           time.Duration `mapstructure:"eventually-timeout"`
	EventuallyPollingInterval   time.Duration `mapstructure:"eventually-polling-interval"`
	ConsistentlyPollingInterval time.Duration `mapstructure:"consistently-polling-interval"`
}

func OperatorNamespace() string {
	if namespace := os.Getenv("OPERATOR_NAMESPACE"); namespace != "" {
		return namespace
	}

	return DefaultOperatorNamespace
}

func IntegrationTestNamespace() string {
	if namespace := os.Getenv("INTEGRATION_TEST_NAMESPACE"); namespace != "" {
		return namespace
	}

	return DefaultIntegrationTestNamespace
}

func LoadConfig() (*Config, error) {
	v := viper.New()

	v.SetDefault(keyClusterType, string(DefaultClusterType))
	v.SetDefault(keyEventuallyTimeout, DefaultEventuallyTimeout)
	v.SetDefault(keyEventuallyPollingInterval, DefaultEventuallyPollingInterval)
	v.SetDefault(keyConsistentlyPollingInterval, DefaultConsistentlyPollingInterval)

	if err := moduleconfig.BindEnv(
		v,
		testConfigEnvPrefix,
		strings.NewReplacer(".", "_", "-", "_"),
		keyClusterType,
		keyEventuallyTimeout,
		keyEventuallyPollingInterval,
		keyConsistentlyPollingInterval,
	); err != nil {
		return nil, err
	}

	cfg := &Config{}
	if err := moduleconfig.Decode(v, cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}
