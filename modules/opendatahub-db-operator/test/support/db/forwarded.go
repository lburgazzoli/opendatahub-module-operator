package db

import (
	"fmt"

	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/pkg/postgres"
)

func ConfigFromForwardedSecret(
	data map[string][]byte,
	localHost string,
	localPort int,
) (postgres.Config, error) {
	if localHost == "" {
		return postgres.Config{}, fmt.Errorf("local host is empty")
	}
	if localPort <= 0 || localPort > 65535 {
		return postgres.Config{}, fmt.Errorf("local port must be between 1 and 65535, got %d", localPort)
	}

	cfg, err := postgres.ParseSecret(data)
	if err != nil {
		return postgres.Config{}, err
	}

	cfg.Host = localHost
	cfg.Port = localPort

	return cfg, nil
}
