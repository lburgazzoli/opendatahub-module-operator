package manager

import (
	dbcontroller "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/pkg/controller"
)

type Option interface {
	applyOption(*Options)
}

type Options struct {
	PostgresConnectionConfigResolver dbcontroller.PostgresConnectionConfigResolver
}

func (o Options) applyOption(target *Options) {
	if target == nil {
		return
	}
	if o.PostgresConnectionConfigResolver != nil {
		target.PostgresConnectionConfigResolver = o.PostgresConnectionConfigResolver
	}
}

type optionFunc func(*Options)

func (fn optionFunc) applyOption(target *Options) {
	if fn == nil {
		return
	}

	fn(target)
}

func WithPostgresConnectionConfigResolver(resolver dbcontroller.PostgresConnectionConfigResolver) Option {
	return optionFunc(func(target *Options) {
		if target == nil || resolver == nil {
			return
		}

		target.PostgresConnectionConfigResolver = resolver
	})
}
