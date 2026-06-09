package resources

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Get fetches a single object from the cache. The object key is derived from
// dest's name/namespace. When dest is a typed (structured) object the lookup
// goes through an unstructured Get — matching the informer registration used
// by the action framework — and the result is converted back into the typed
// object. If dest is already unstructured the call is passed through directly.
func Get(
	ctx context.Context,
	cli client.Client,
	dest client.Object,
) error {
	key := client.ObjectKeyFromObject(dest)

	if _, ok := dest.(*unstructured.Unstructured); ok {
		return cli.Get(ctx, key, dest)
	}

	gvk, err := cli.GroupVersionKindFor(dest)
	if err != nil {
		return fmt.Errorf("unable to determine GVK: %w", err)
	}

	u := &unstructured.Unstructured{}
	u.SetGroupVersionKind(gvk)

	if err := cli.Get(ctx, key, u); err != nil {
		return err
	}

	return runtime.DefaultUnstructuredConverter.FromUnstructured(u.Object, dest)
}

// List fetches a list of objects from the cache. When dest is a typed list the
// lookup goes through an unstructured list — matching the informer registration
// — and the result is converted back. If dest is already an UnstructuredList
// the call is passed through directly.
func List(
	ctx context.Context,
	cli client.Client,
	dest client.ObjectList,
	opts ...client.ListOption,
) error {
	if _, ok := dest.(*unstructured.UnstructuredList); ok {
		return cli.List(ctx, dest, opts...)
	}

	gvk, err := cli.GroupVersionKindFor(dest)
	if err != nil {
		return fmt.Errorf("unable to determine GVK: %w", err)
	}

	ul := &unstructured.UnstructuredList{}
	ul.SetGroupVersionKind(gvk)

	if err := cli.List(ctx, ul, opts...); err != nil {
		return err
	}

	return runtime.DefaultUnstructuredConverter.FromUnstructured(ul.UnstructuredContent(), dest)
}
