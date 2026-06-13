package support

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/viper"
)

const (
	DefaultEventuallyTimeout           = 90 * time.Second
	DefaultEventuallyPollingInterval   = 2 * time.Second
	DefaultConsistentlyPollingInterval = 2 * time.Second

	testConfigEnvPrefix = "ODH_MODULE_OPERATOR_TEST"

	keyEventuallyTimeout           = "eventually-timeout"
	keyEventuallyPollingInterval   = "eventually-polling-interval"
	keyConsistentlyPollingInterval = "consistently-polling-interval"
)

type GomegaConfig struct {
	EventuallyTimeout           time.Duration
	EventuallyPollingInterval   time.Duration
	ConsistentlyPollingInterval time.Duration
}

func LoadGomegaConfig() (*GomegaConfig, error) {
	v := viper.New()
	v.SetEnvPrefix(testConfigEnvPrefix)
	v.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))
	v.AutomaticEnv()

	v.SetDefault(keyEventuallyTimeout, DefaultEventuallyTimeout)
	v.SetDefault(keyEventuallyPollingInterval, DefaultEventuallyPollingInterval)
	v.SetDefault(keyConsistentlyPollingInterval, DefaultConsistentlyPollingInterval)

	for _, key := range []string{
		keyEventuallyTimeout,
		keyEventuallyPollingInterval,
		keyConsistentlyPollingInterval,
	} {
		if err := v.BindEnv(key); err != nil {
			return nil, fmt.Errorf("binding env for %s: %w", key, err)
		}
	}

	return &GomegaConfig{
		EventuallyTimeout:           v.GetDuration(keyEventuallyTimeout),
		EventuallyPollingInterval:   v.GetDuration(keyEventuallyPollingInterval),
		ConsistentlyPollingInterval: v.GetDuration(keyConsistentlyPollingInterval),
	}, nil
}
