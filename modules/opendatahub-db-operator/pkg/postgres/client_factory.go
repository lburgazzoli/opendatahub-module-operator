package postgres

import "context"

type ClientFactory func(ctx context.Context, cfg Config) (Client, error)

func DefaultClientFactory(ctx context.Context, cfg Config) (Client, error) {
	return NewClient(ctx, cfg)
}
