package logger

import (
	"fmt"
	"maps"
)

type LoggerFn func(format string, args ...any)

type Option interface {
	ApplyTo(target *Options)
}

type StreamOption interface {
	ApplyTo(target *StreamOptions)
}

type optionFunc[T any] func(*T)

func (f optionFunc[T]) ApplyTo(target *T) {
	if target == nil {
		return
	}

	f(target)
}

type Options struct {
	DefaultNamespace string
	DefaultPrefix    string
	LoggerFn         LoggerFn
}

type StreamOptions struct {
	Namespace   string
	Prefix      string
	Name        string
	MatchLabels map[string]string
	Container   string
	LoggerFn    LoggerFn
}

func defaultOptions() Options {
	return Options{
		LoggerFn: func(format string, args ...any) {
			fmt.Printf(format+"\n", args...)
		},
	}
}

func defaultStreamOptions(opts Options) StreamOptions {
	return StreamOptions{
		Namespace: opts.DefaultNamespace,
		Prefix:    opts.DefaultPrefix,
		LoggerFn:  opts.LoggerFn,
	}
}

func (opts StreamOptions) Validate() error {
	if opts.Namespace == "" {
		return fmt.Errorf("namespace is empty")
	}
	if opts.Name == "" && len(opts.MatchLabels) == 0 {
		return fmt.Errorf("pod name or selector is required")
	}

	return nil
}

func WithLoggerFn(fn LoggerFn) Option {
	return optionFunc[Options](func(opts *Options) {
		if opts == nil || fn == nil {
			return
		}

		opts.LoggerFn = fn
	})
}

func WithStreamLoggerFn(fn LoggerFn) StreamOption {
	return optionFunc[StreamOptions](func(opts *StreamOptions) {
		if opts == nil || fn == nil {
			return
		}

		opts.LoggerFn = fn
	})
}

func WithDefaultNamespace(namespace string) Option {
	return optionFunc[Options](func(opts *Options) {
		if opts == nil || namespace == "" {
			return
		}

		opts.DefaultNamespace = namespace
	})
}

func WithDefaultPrefix(prefix string) Option {
	return optionFunc[Options](func(opts *Options) {
		if opts == nil || prefix == "" {
			return
		}

		opts.DefaultPrefix = prefix
	})
}

func WithNamespace(namespace string) StreamOption {
	return optionFunc[StreamOptions](func(opts *StreamOptions) {
		if opts == nil || namespace == "" {
			return
		}

		opts.Namespace = namespace
	})
}

func WithPrefix(prefix string) StreamOption {
	return optionFunc[StreamOptions](func(opts *StreamOptions) {
		if opts == nil || prefix == "" {
			return
		}

		opts.Prefix = prefix
	})
}

func WithName(name string) StreamOption {
	return optionFunc[StreamOptions](func(opts *StreamOptions) {
		if opts == nil || name == "" {
			return
		}

		opts.Name = name
	})
}

func WithSelector(matchLabels map[string]string) StreamOption {
	return optionFunc[StreamOptions](func(opts *StreamOptions) {
		if opts == nil || len(matchLabels) == 0 {
			return
		}

		opts.MatchLabels = cloneLabels(matchLabels)
	})
}

func WithContainer(container string) StreamOption {
	return optionFunc[StreamOptions](func(opts *StreamOptions) {
		if opts == nil || container == "" {
			return
		}

		opts.Container = container
	})
}

func cloneLabels(src map[string]string) map[string]string {
	if len(src) == 0 {
		return nil
	}

	dst := make(map[string]string, len(src))
	maps.Copy(dst, src)

	return dst
}
