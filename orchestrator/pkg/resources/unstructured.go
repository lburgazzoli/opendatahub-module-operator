package resources

import (
	"fmt"
	"reflect"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Decode returns obj as the requested typed object. Typed inputs of the same
// Go type are returned directly; unstructured inputs are converted through the
// default unstructured converter.
func Decode[T client.Object](
	obj client.Object,
) (T, error) {
	var zero T

	if obj == nil {
		return zero, fmt.Errorf("unexpected object type %T", obj)
	}

	if typed, ok := obj.(T); ok {
		return typed, nil
	}

	u, ok := obj.(*unstructured.Unstructured)
	if !ok {
		return zero, fmt.Errorf("unexpected object type %T", obj)
	}

	targetType := reflect.TypeOf(zero)
	if targetType == nil || targetType.Kind() != reflect.Pointer {
		return zero, fmt.Errorf("decode target must be a pointer type")
	}

	destValue := reflect.New(targetType.Elem())
	dest, ok := destValue.Interface().(T)
	if !ok {
		return zero, fmt.Errorf("decode target must implement client.Object")
	}

	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(u.Object, dest); err != nil {
		return zero, err
	}

	return dest, nil
}
