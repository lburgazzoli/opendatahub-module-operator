package db

import (
	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/pkg/postgres"
)

type Option interface {
	applyOption(target *Options)
}

type Options struct {
	Client        client.Client
	ClientFactory postgres.ClientFactory
	Namespace     string
	Name          string
	Image         string
}

func (o Options) applyOption(target *Options) {
	if target == nil {
		return
	}
	if o.Client != nil {
		target.Client = o.Client
	}
	if o.ClientFactory != nil {
		target.ClientFactory = o.ClientFactory
	}
	if o.Namespace != "" {
		target.Namespace = o.Namespace
	}
	if o.Name != "" {
		target.Name = o.Name
	}
	if o.Image != "" {
		target.Image = o.Image
	}
}

func (o Options) Validate() error {
	if o.Client == nil {
		return fmt.Errorf("client is nil")
	}
	if o.ClientFactory == nil {
		return fmt.Errorf("client factory is nil")
	}
	if o.Namespace == "" {
		return fmt.Errorf("namespace is empty")
	}
	if o.Name == "" {
		return fmt.Errorf("name is empty")
	}
	if o.Image == "" {
		return fmt.Errorf("image is empty")
	}

	return nil
}

type optionFunc func(*Options)

func (fn optionFunc) applyOption(target *Options) {
	if fn == nil {
		return
	}
	fn(target)
}

func WithClient(cli client.Client) Option {
	return optionFunc(func(options *Options) {
		if options == nil || cli == nil {
			return
		}
		options.Client = cli
	})
}

func WithClientFactory(factory postgres.ClientFactory) Option {
	return optionFunc(func(options *Options) {
		if options == nil || factory == nil {
			return
		}
		options.ClientFactory = factory
	})
}

func WithNamespace(namespace string) Option {
	return optionFunc(func(options *Options) {
		if options == nil || namespace == "" {
			return
		}
		options.Namespace = namespace
	})
}

func WithName(name string) Option {
	return optionFunc(func(options *Options) {
		if options == nil || name == "" {
			return
		}
		options.Name = name
	})
}

func WithImage(image string) Option {
	return optionFunc(func(options *Options) {
		if options == nil || image == "" {
			return
		}
		options.Image = image
	})
}
