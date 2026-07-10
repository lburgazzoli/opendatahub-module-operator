package support

import (
	"context"
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	defaultDeleteTimeout      = 30 * time.Second
	defaultDeletePollInterval = 200 * time.Millisecond
)

// ClearFinalizersAndDelete removes finalizers when present, then deletes the
// object using foreground propagation. Missing objects are ignored.
func ClearFinalizersAndDelete(
	ctx context.Context,
	cli client.Client,
	obj client.Object,
) error {
	if len(obj.GetFinalizers()) > 0 {
		patch := client.MergeFrom(obj.DeepCopyObject().(client.Object))
		obj.SetFinalizers(nil)

		err := cli.Patch(ctx, obj, patch)
		switch {
		case errors.IsNotFound(err):
			return nil
		case meta.IsNoMatchError(err):
			return nil
		case err != nil:
			return fmt.Errorf("clearing finalizers for %s: %w", client.ObjectKeyFromObject(obj), err)
		}
	}

	err := cli.Delete(ctx, obj, client.PropagationPolicy(metav1.DeletePropagationForeground))
	switch {
	case errors.IsNotFound(err):
		return nil
	case meta.IsNoMatchError(err):
		return nil
	case err != nil:
		return fmt.Errorf("deleting %s: %w", client.ObjectKeyFromObject(obj), err)
	default:
		return nil
	}
}

// DeleteAndWait reads the current object state, clears finalizers when needed,
// deletes the object with foreground propagation, then waits until it is gone.
// Missing objects are treated as already deleted. The passed object is reset so
// it can be recreated immediately after this call.
func DeleteAndWait(
	ctx context.Context,
	cli client.Client,
	obj client.Object,
) error {
	key := client.ObjectKeyFromObject(obj)

	switch err := cli.Get(ctx, key, obj); {
	case errors.IsNotFound(err):
		resetDeletedObject(obj)
		return nil
	case meta.IsNoMatchError(err):
		resetDeletedObject(obj)
		return nil
	case err != nil:
		return fmt.Errorf("reading %s before delete: %w", key, err)
	}
	uid := obj.GetUID()

	if err := ClearFinalizersAndDelete(ctx, cli, obj); err != nil {
		return err
	}

	if err := wait.PollUntilContextTimeout(
		ctx,
		defaultDeletePollInterval,
		defaultDeleteTimeout,
		true,
		func(ctx context.Context) (bool, error) {
			current := obj.DeepCopyObject().(client.Object)
			switch err := cli.Get(ctx, key, current); {
			case errors.IsNotFound(err):
				return true, nil
			case meta.IsNoMatchError(err):
				return true, nil
			case err != nil:
				return false, fmt.Errorf("getting %s: %w", key, err)
			case uid != "" && current.GetUID() != "" && current.GetUID() != uid:
				return true, nil
			default:
				return false, nil
			}
		},
	); err != nil {
		return fmt.Errorf("waiting for %s to be deleted: %w", key, err)
	}

	resetDeletedObject(obj)
	return nil
}

func resetDeletedObject(obj client.Object) {
	obj.SetResourceVersion("")
	obj.SetUID("")
	obj.SetDeletionTimestamp(nil)
	obj.SetFinalizers(nil)
}
