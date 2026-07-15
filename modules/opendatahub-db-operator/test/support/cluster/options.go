package cluster

import (
	"time"

	kindcluster "sigs.k8s.io/kind/pkg/cluster"
)

type Option interface {
	applyOption(target *options)
}

type options struct {
	LogFn         func(format string, args ...any)
	CreateOptions []kindcluster.CreateOption
	Purge         *bool
}

func (o options) applyOption(target *options) {
	if target == nil {
		return
	}
	if o.LogFn != nil {
		target.LogFn = o.LogFn
	}
	if len(o.CreateOptions) > 0 {
		target.CreateOptions = append(target.CreateOptions, o.CreateOptions...)
	}
	if o.Purge != nil {
		purge := *o.Purge
		target.Purge = &purge
	}
}

type optionFunc func(*options)

func (fn optionFunc) applyOption(target *options) {
	if fn == nil {
		return
	}

	fn(target)
}

func WithLogFn(logFn func(format string, args ...any)) Option {
	return optionFunc(func(opts *options) {
		if opts == nil || logFn == nil {
			return
		}

		opts.LogFn = logFn
	})
}

func WithCreateOption(option kindcluster.CreateOption) Option {
	return optionFunc(func(opts *options) {
		if opts == nil || option == nil {
			return
		}

		opts.CreateOptions = append(opts.CreateOptions, option)
	})
}

func WithNodeImage(image string) Option {
	if image == "" {
		return nil
	}

	return WithCreateOption(kindcluster.CreateWithNodeImage(image))
}

func WithRawConfig(rawConfig []byte) Option {
	if len(rawConfig) == 0 {
		return nil
	}

	return WithCreateOption(kindcluster.CreateWithRawConfig(rawConfig))
}

func WithWaitForReady(wait time.Duration) Option {
	if wait < 0 {
		return nil
	}

	return WithCreateOption(kindcluster.CreateWithWaitForReady(wait))
}

func WithPurge(enabled bool) Option {
	return optionFunc(func(opts *options) {
		if opts == nil {
			return
		}

		opts.Purge = &enabled
	})
}
