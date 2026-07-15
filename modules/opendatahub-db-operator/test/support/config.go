package support

import (
	"os"
	"strings"
	"time"

	"github.com/spf13/viper"

	moduleconfig "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/pkg/config"
)

const (
	DefaultClusterType                 = ClusterTypeKind
	DefaultEventuallyTimeout           = 90 * time.Second
	DefaultEventuallyPollingInterval   = 2 * time.Second
	DefaultConsistentlyPollingInterval = 2 * time.Second
	DefaultOperatorInstall             = false
	DefaultOperatorLogs                = false
	DefaultOperatorNamespace           = "odh-db-operator-system"
	DefaultIntegrationTestNamespace    = "odh-db-operator-integration"
	DefaultPlatformType                = "OpenDataHub"
	DefaultPlatformVersion             = "0.1.0"

	testConfigEnvPrefix = "ODH_MODULE_OPERATOR_TEST"

	keyClusterType                 = "cluster.type"
	keyEventuallyTimeout           = "gomega.eventually-timeout"
	keyEventuallyPollingInterval   = "gomega.eventually-polling-interval"
	keyConsistentlyPollingInterval = "gomega.consistently-polling-interval"
	keyOperatorInstall             = "operator.install"
	keyOperatorLogs                = "operator.logs"
	keyOperatorImage               = "operator.image"
	keyOperatorNamespace           = "operator.namespace"
	keyOperatorPlatformType        = "operator.platform-type"
	keyOperatorPlatformVersion     = "operator.platform-version"
)

type ClusterType string

const (
	ClusterTypeExternal ClusterType = "external"
	ClusterTypeKind     ClusterType = "kind"
	ClusterTypeK3s      ClusterType = "k3s"
)

type Config struct {
	Cluster  ClusterConfig  `mapstructure:"cluster"`
	Gomega   GomegaConfig   `mapstructure:"gomega"`
	Operator OperatorConfig `mapstructure:"operator"`
}

type ClusterConfig struct {
	Type ClusterType `mapstructure:"type"`
}

type GomegaConfig struct {
	EventuallyTimeout           time.Duration `mapstructure:"eventually-timeout"`
	EventuallyPollingInterval   time.Duration `mapstructure:"eventually-polling-interval"`
	ConsistentlyPollingInterval time.Duration `mapstructure:"consistently-polling-interval"`
}

type OperatorConfig struct {
	Install         bool   `mapstructure:"install"`
	Logs            bool   `mapstructure:"logs"`
	Image           string `mapstructure:"image"`
	Namespace       string `mapstructure:"namespace"`
	PlatformType    string `mapstructure:"platform-type"`
	PlatformVersion string `mapstructure:"platform-version"`
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
	v.SetDefault(keyOperatorInstall, DefaultOperatorInstall)
	v.SetDefault(keyOperatorLogs, DefaultOperatorLogs)
	v.SetDefault(keyOperatorNamespace, DefaultOperatorNamespace)
	v.SetDefault(keyOperatorPlatformType, DefaultPlatformType)
	v.SetDefault(keyOperatorPlatformVersion, DefaultPlatformVersion)

	if err := moduleconfig.BindEnv(
		v,
		testConfigEnvPrefix,
		strings.NewReplacer(".", "_", "-", "_"),
		keyClusterType,
		keyEventuallyTimeout,
		keyEventuallyPollingInterval,
		keyConsistentlyPollingInterval,
		keyOperatorInstall,
		keyOperatorLogs,
		keyOperatorImage,
		keyOperatorNamespace,
		keyOperatorPlatformType,
		keyOperatorPlatformVersion,
	); err != nil {
		return nil, err
	}

	cfg := &Config{}
	if err := moduleconfig.Decode(v, cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}
