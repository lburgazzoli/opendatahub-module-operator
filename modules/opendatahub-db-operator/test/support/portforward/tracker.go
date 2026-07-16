package portforward

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"k8s.io/client-go/rest"
)

type Tracker struct {
	restCfg  *rest.Config
	mu       sync.Mutex
	forwards map[serviceKey]*Forward
}

type serviceKey struct {
	namespace  string
	name       string
	remotePort int
	addresses  string
	localPort  int
}

func NewTracker(restCfg *rest.Config) (*Tracker, error) {
	if restCfg == nil {
		return nil, fmt.Errorf("rest config is nil")
	}

	return &Tracker{
		restCfg:  rest.CopyConfig(restCfg),
		forwards: map[serviceKey]*Forward{},
	}, nil
}

func (t *Tracker) EnsureService(
	ctx context.Context,
	namespace string,
	serviceName string,
	remotePort int,
	opts ...Option,
) (*Forward, error) {
	if t == nil {
		return nil, fmt.Errorf("tracker is nil")
	}

	options := defaultOptions()
	for _, opt := range opts {
		if opt != nil {
			opt.ApplyTo(&options)
		}
	}
	if err := options.Validate(); err != nil {
		return nil, err
	}

	key := serviceKey{
		namespace:  namespace,
		name:       serviceName,
		remotePort: remotePort,
		addresses:  strings.Join(options.Addresses, ","),
		localPort:  options.LocalPort,
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	if existing := t.forwards[key]; existing != nil {
		return existing, nil
	}

	forward, err := StartService(ctx, t.restCfg, namespace, serviceName, remotePort, opts...)
	if err != nil {
		return nil, err
	}

	t.forwards[key] = forward
	return forward, nil
}

func (t *Tracker) Close(ctx context.Context) error {
	if t == nil {
		return nil
	}

	t.mu.Lock()
	forwards := make([]*Forward, 0, len(t.forwards))
	for _, forward := range t.forwards {
		if forward != nil {
			forwards = append(forwards, forward)
		}
	}
	t.forwards = map[serviceKey]*Forward{}
	t.mu.Unlock()

	var errs []error
	for _, forward := range forwards {
		if err := forward.Close(ctx); err != nil {
			errs = append(errs, err)
		}
	}

	return errorsJoin(errs)
}

func errorsJoin(errs []error) error {
	switch len(errs) {
	case 0:
		return nil
	case 1:
		return errs[0]
	default:
		return fmt.Errorf("closing port-forwards: %v", errs)
	}
}
