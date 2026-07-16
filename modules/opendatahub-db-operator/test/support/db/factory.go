package db

import (
	"context"
	"fmt"
	"strings"

	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/pkg/postgres"
	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/test/support/portforward"
)

func NewForwardingClientFactory(tracker *portforward.Tracker) postgres.ClientFactory {
	return func(ctx context.Context, cfg postgres.Config) (postgres.Client, error) {
		if tracker == nil {
			return postgres.NewClient(ctx, cfg)
		}

		serviceName, namespace, ok := serviceRefForHost(cfg.Host)
		if !ok {
			return postgres.NewClient(ctx, cfg)
		}

		forward, err := tracker.EnsureService(ctx, namespace, serviceName, cfg.Port)
		if err != nil {
			return nil, fmt.Errorf(
				"starting port-forward for service %s/%s: %w",
				namespace,
				serviceName,
				err,
			)
		}

		forwardedCfg, err := ConfigWithForwardTarget(cfg, forward.Host(), forward.Port())
		if err != nil {
			return nil, err
		}

		return postgres.NewClient(ctx, forwardedCfg)
	}
}

func serviceRefForHost(host string) (serviceName string, namespace string, ok bool) {
	parts := strings.Split(host, ".")
	if len(parts) < 3 {
		return "", "", false
	}
	if parts[0] == "" || parts[1] == "" {
		return "", "", false
	}
	if parts[2] != "svc" {
		return "", "", false
	}

	return parts[0], parts[1], true
}
