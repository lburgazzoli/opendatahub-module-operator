package portforward

import "fmt"

type Option interface {
	ApplyTo(target *Options)
}

type optionFunc func(*Options)

func (fn optionFunc) ApplyTo(target *Options) {
	if target == nil || fn == nil {
		return
	}

	fn(target)
}

type Options struct {
	Addresses []string
	LocalPort int
}

func defaultOptions() Options {
	return Options{
		Addresses: []string{"127.0.0.1"},
	}
}

func (opts Options) Validate() error {
	if len(opts.Addresses) == 0 {
		return fmt.Errorf("at least one listen address is required")
	}
	if opts.LocalPort < 0 || opts.LocalPort > 65535 {
		return fmt.Errorf("local port must be between 0 and 65535, got %d", opts.LocalPort)
	}

	return nil
}

func WithAddress(address string) Option {
	return optionFunc(func(target *Options) {
		if target == nil || address == "" {
			return
		}

		target.Addresses = []string{address}
	})
}

func WithAddresses(addresses ...string) Option {
	return optionFunc(func(target *Options) {
		if target == nil {
			return
		}

		filtered := make([]string, 0, len(addresses))
		for _, address := range addresses {
			if address != "" {
				filtered = append(filtered, address)
			}
		}
		if len(filtered) == 0 {
			return
		}

		target.Addresses = filtered
	})
}

func WithLocalPort(port int) Option {
	return optionFunc(func(target *Options) {
		if target == nil {
			return
		}

		target.LocalPort = port
	})
}
