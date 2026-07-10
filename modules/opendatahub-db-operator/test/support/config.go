package support

import (
	"fmt"
	"strings"
	"time"

	"github.com/go-viper/mapstructure/v2"
	"github.com/spf13/viper"

	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/test/support/cluster"
)

const (
	DefaultEventuallyTimeout           = 90 * time.Second
	DefaultEventuallyPollingInterval   = 2 * time.Second
	DefaultConsistentlyPollingInterval = 2 * time.Second

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

func LoadConfig(defaultClusterType cluster.Type) (*Config, error) {
	v := viper.New()
	v.SetEnvPrefix(testConfigEnvPrefix)
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_", "-", "_"))
	v.AutomaticEnv()

	v.SetDefault(keyClusterType, string(defaultClusterType))
	v.SetDefault(keyEventuallyTimeout, DefaultEventuallyTimeout)
	v.SetDefault(keyEventuallyPollingInterval, DefaultEventuallyPollingInterval)
	v.SetDefault(keyConsistentlyPollingInterval, DefaultConsistentlyPollingInterval)

	for _, key := range []string{
		keyClusterType,
		keyEventuallyTimeout,
		keyEventuallyPollingInterval,
		keyConsistentlyPollingInterval,
	} {
		if err := v.BindEnv(key); err != nil {
			return nil, fmt.Errorf("binding env for %s: %w", key, err)
		}
	}

	cfg := &Config{}
	decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		Result:           cfg,
		WeaklyTypedInput: true,
		TagName:          "mapstructure",
		DecodeHook: mapstructure.ComposeDecodeHookFunc(
			mapstructure.StringToTimeDurationHookFunc(),
		),
	})
	if err != nil {
		return nil, fmt.Errorf("building config decoder: %w", err)
	}
	if err := decoder.Decode(v.AllSettings()); err != nil {
		return nil, fmt.Errorf("decoding test config: %w", err)
	}

	return cfg, nil
}

func LoadGomegaConfig() (*GomegaConfig, error) {
	cfg, err := LoadConfig(cluster.TypeExternal)
	if err != nil {
		return nil, err
	}

	return &cfg.Gomega, nil
}
