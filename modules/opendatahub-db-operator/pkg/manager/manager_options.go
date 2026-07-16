package manager

import (
	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/pkg/postgres"
)

type Option interface {
	applyOption(*Options)
}

type Options struct {
	PostgresClientFactory postgres.ClientFactory
}

func (o Options) applyOption(target *Options) {
	if target == nil {
		return
	}
	if o.PostgresClientFactory != nil {
		target.PostgresClientFactory = o.PostgresClientFactory
	}
}

type optionFunc func(*Options)

func (fn optionFunc) applyOption(target *Options) {
	if fn == nil {
		return
	}

	fn(target)
}

func WithPostgresClientFactory(factory postgres.ClientFactory) Option {
	return optionFunc(func(target *Options) {
		if target == nil || factory == nil {
			return
		}

		target.PostgresClientFactory = factory
	})
}
