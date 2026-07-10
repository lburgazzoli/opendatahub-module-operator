package reaper

import (
	"context"
	"errors"
	"fmt"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/pkg/resources/gvk"
	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/test/support"
)

const (
	defaultTimeout         = 30 * time.Second
	defaultPollingInterval = 200 * time.Millisecond
)

type Reaper struct {
	cli client.Client

	timeout         time.Duration
	pollingInterval time.Duration
}

// Option is implemented by both the Options struct literal and the named
// With* constructor functions.
type Option interface {
	applyOption(o *Options)
}

type Options struct {
	Timeout         time.Duration
	PollingInterval time.Duration
}

func (o Options) applyOption(target *Options) {
	if o.Timeout > 0 {
		target.Timeout = o.Timeout
	}
	if o.PollingInterval > 0 {
		target.PollingInterval = o.PollingInterval
	}
}

type optionFunc func(*Options)

func (fn optionFunc) applyOption(target *Options) {
	if fn == nil {
		return
	}
	fn(target)
}

func WithTimeout(timeout time.Duration) Option {
	return optionFunc(func(r *Options) {
		if r == nil || timeout <= 0 {
			return
		}
		r.Timeout = timeout
	})
}

func WithPollingInterval(interval time.Duration) Option {
	return optionFunc(func(r *Options) {
		if r == nil || interval <= 0 {
			return
		}
		r.PollingInterval = interval
	})
}

func New(cli client.Client, opts ...Option) (*Reaper, error) {
	if cli == nil {
		return nil, fmt.Errorf("client is nil")
	}

	reaperOptions := &Options{
		Timeout:         defaultTimeout,
		PollingInterval: defaultPollingInterval,
	}

	for _, opt := range opts {
		if opt != nil {
			opt.applyOption(reaperOptions)
		}
	}

	return &Reaper{
		cli:             cli,
		timeout:         reaperOptions.Timeout,
		pollingInterval: reaperOptions.PollingInterval,
	}, nil
}

func (r *Reaper) Run(ctx context.Context) error {
	var errs []error
	if err := r.cleanupByGVK(ctx, gvk.SchemaClaim); err != nil {
		errs = append(errs, err)
	}
	if err := r.cleanupByGVK(ctx, gvk.DatabaseClaim); err != nil {
		errs = append(errs, err)
	}
	if err := r.cleanupByGVK(ctx, gvk.DatabaseProvider); err != nil {
		errs = append(errs, err)
	}

	return errors.Join(errs...)
}

func (r *Reaper) cleanupByGVK(ctx context.Context, resourceGVK schema.GroupVersionKind) error {
	list := &unstructured.UnstructuredList{}
	list.SetGroupVersionKind(resourceGVK)

	if err := r.cli.List(ctx, list); err != nil {
		return fmt.Errorf("listing %s resources: %w", resourceGVK.Kind, err)
	}

	for i := range list.Items {
		obj := &list.Items[i]

		if err := support.ClearFinalizersAndDelete(ctx, r.cli, obj); err != nil {
			return fmt.Errorf("deleting %s %q: %w", resourceGVK.Kind, list.Items[i].GetName(), err)
		}
	}

	if err := r.waitObjectsDeleted(ctx, list.Items); err != nil {
		return fmt.Errorf("waiting for %s resources to be deleted: %w", resourceGVK.Kind, err)
	}

	return nil
}

func (r *Reaper) waitObjectsDeleted(
	ctx context.Context,
	items []unstructured.Unstructured,
) error {
	return wait.PollUntilContextTimeout(
		ctx,
		r.pollingInterval,
		r.timeout,
		true,
		func(ctx context.Context) (bool, error) {
			for i := len(items) - 1; i >= 0; i-- {
				key := client.ObjectKeyFromObject(&items[i])
				switch err := r.cli.Get(ctx, key, &items[i]); {
				case apierrors.IsNotFound(err):
					items = append(items[:i], items[i+1:]...)
					continue
				case meta.IsNoMatchError(err):
					items = append(items[:i], items[i+1:]...)
					continue
				case err != nil:
					return false, fmt.Errorf("getting %q: %w", key, err)
				}
			}
			return len(items) == 0, nil
		},
	)
}
