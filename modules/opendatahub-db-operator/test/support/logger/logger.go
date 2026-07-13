package logger

import (
	"context"
	"fmt"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

type Logger struct {
	clientset kubernetes.Interface
	opts      Options
}

type Handler struct {
	cancel context.CancelFunc
	done   <-chan error
}

func New(restCfg *rest.Config, opts ...Option) (*Logger, error) {
	if restCfg == nil {
		return nil, fmt.Errorf("rest config is nil")
	}

	options := defaultOptions()
	for _, opt := range opts {
		if opt != nil {
			opt.ApplyTo(&options)
		}
	}

	clientset, err := kubernetes.NewForConfig(restCfg)
	if err != nil {
		return nil, fmt.Errorf("creating kube clientset: %w", err)
	}

	return &Logger{
		clientset: clientset,
		opts:      options,
	}, nil
}

func (l *Logger) Stream(ctx context.Context, opts ...StreamOption) (*Handler, error) {
	if l == nil || l.clientset == nil {
		return nil, fmt.Errorf("logger is nil")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	options := defaultStreamOptions(l.opts)
	for _, opt := range opts {
		if opt != nil {
			opt.ApplyTo(&options)
		}
	}
	if err := options.Validate(); err != nil {
		return nil, err
	}

	streamCtx, cancel := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() {
		defer close(done)
		done <- streamPodLogs(streamCtx, l.clientset, options)
	}()

	return &Handler{
		cancel: cancel,
		done:   done,
	}, nil
}

func (h *Handler) Stop(ctx context.Context) error {
	if h == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if h.cancel != nil {
		h.cancel()
	}
	if h.done == nil {
		return nil
	}

	select {
	case err := <-h.done:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}
