package kind

import (
	"fmt"
	"time"

	kindcluster "sigs.k8s.io/kind/pkg/cluster"
)

type Option interface {
	applyOption(target *options)
}

type options struct {
	NamePrefix      string
	ProviderOptions []kindcluster.ProviderOption
	CreateOptions   []kindcluster.CreateOption
	LogsDir         string
	LogFn           func(format string, args ...any)
	Purge           *bool
}

func (o options) applyOption(target *options) {
	if target == nil {
		return
	}
	if o.NamePrefix != "" {
		target.NamePrefix = o.NamePrefix
	}
	if len(o.ProviderOptions) > 0 {
		target.ProviderOptions = append(target.ProviderOptions, o.ProviderOptions...)
	}
	if len(o.CreateOptions) > 0 {
		target.CreateOptions = append(target.CreateOptions, o.CreateOptions...)
	}
	if o.LogsDir != "" {
		target.LogsDir = o.LogsDir
	}
	if o.LogFn != nil {
		target.LogFn = o.LogFn
	}
	if o.Purge != nil {
		purge := *o.Purge
		target.Purge = &purge
	}
}

func (o options) Validate() error {
	if o.NamePrefix == "" {
		return fmt.Errorf("name prefix is empty")
	}

	return nil
}

type optionFunc func(*options)

func (fn optionFunc) applyOption(target *options) {
	if fn == nil {
		return
	}

	fn(target)
}

func WithCreateOption(option kindcluster.CreateOption) Option {
	return optionFunc(func(opts *options) {
		if opts == nil || option == nil {
			return
		}

		opts.CreateOptions = append(opts.CreateOptions, option)
	})
}

func WithLogsDir(dir string) Option {
	return optionFunc(func(opts *options) {
		if opts == nil || dir == "" {
			return
		}

		opts.LogsDir = dir
	})
}

func WithLogFn(logFn func(format string, args ...any)) Option {
	return optionFunc(func(opts *options) {
		if opts == nil || logFn == nil {
			return
		}

		opts.LogFn = logFn
	})
}

func WithNamePrefix(prefix string) Option {
	return optionFunc(func(opts *options) {
		if opts == nil || prefix == "" {
			return
		}

		opts.NamePrefix = prefix
	})
}

func WithNodeImage(image string) Option {
	return optionFunc(func(opts *options) {
		if opts == nil || image == "" {
			return
		}

		opts.CreateOptions = append(opts.CreateOptions, kindcluster.CreateWithNodeImage(image))
	})
}

func WithProviderOption(option kindcluster.ProviderOption) Option {
	return optionFunc(func(opts *options) {
		if opts == nil || option == nil {
			return
		}

		opts.ProviderOptions = append(opts.ProviderOptions, option)
	})
}

func WithRawConfig(rawConfig []byte) Option {
	return optionFunc(func(opts *options) {
		if opts == nil || len(rawConfig) == 0 {
			return
		}

		opts.CreateOptions = append(opts.CreateOptions, kindcluster.CreateWithRawConfig(rawConfig))
	})
}

func WithWaitForReady(wait time.Duration) Option {
	return optionFunc(func(opts *options) {
		if opts == nil || wait < 0 {
			return
		}

		opts.CreateOptions = append(opts.CreateOptions, kindcluster.CreateWithWaitForReady(wait))
	})
}

func WithPurge(enabled bool) Option {
	return optionFunc(func(opts *options) {
		if opts == nil {
			return
		}

		opts.Purge = &enabled
	})
}
