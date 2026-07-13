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

type Option func(*Reaper)

func WithTimeout(timeout time.Duration) Option {
	return func(r *Reaper) {
		if r == nil || timeout <= 0 {
			return
		}

		r.timeout = timeout
	}
}

func WithPollingInterval(interval time.Duration) Option {
	return func(r *Reaper) {
		if r == nil || interval <= 0 {
			return
		}

		r.pollingInterval = interval
	}
}

func New(cli client.Client, opts ...Option) (*Reaper, error) {
	if cli == nil {
		return nil, fmt.Errorf("client is nil")
	}

	r := &Reaper{
		cli:             cli,
		timeout:         defaultTimeout,
		pollingInterval: defaultPollingInterval,
	}

	for _, opt := range opts {
		if opt != nil {
			opt(r)
		}
	}

	return r, nil
}

func (r *Reaper) Run(ctx context.Context) error {
	var errs []error

	for _, resourceGVK := range []schema.GroupVersionKind{
		gvk.SchemaClaim,
		gvk.DatabaseClaim,
		gvk.DatabaseProvider,
	} {
		if err := r.cleanupByGVK(ctx, resourceGVK); err != nil {
			errs = append(errs, err)
		}
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
