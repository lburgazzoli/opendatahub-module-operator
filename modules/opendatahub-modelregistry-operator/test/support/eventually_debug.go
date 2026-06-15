package support

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type PollFunc func() error
type ContextPollFunc[T any] func(context.Context) (T, error)

// WrapPoll logs the rendered poll value when it changes or when the poll fails.
func WrapPoll(
	t *testing.T,
	label string,
	poll PollFunc,
	render func() any,
) PollFunc {
	t.Helper()

	if poll == nil {
		panic("WrapPoll: poll must not be nil")
	}

	var last string

	return func() error {
		err := poll()

		var value any = map[string]any{
			"ok": err == nil,
		}
		if render != nil {
			value = render()
		}

		current := stringifyDebugValue(value)
		if current != last || err != nil {
			last = current
			if err != nil {
				t.Logf("[EventuallyDebug] %s err=%v value=%s", label, err, current)
			} else {
				t.Logf("[EventuallyDebug] %s value=%s", label, current)
			}
		}

		return err
	}
}

func WrapGet(
	t *testing.T,
	label string,
	poll PollFunc,
	object client.Object,
) PollFunc {
	t.Helper()

	return WrapPoll(t, label, poll, func() any {
		return SnapshotObject(object)
	})
}

func WrapEventually[T any](
	t *testing.T,
	label string,
	poll ContextPollFunc[T],
	render func(T) any,
) ContextPollFunc[T] {
	t.Helper()

	if poll == nil {
		panic("WrapEventually: poll must not be nil")
	}

	var last string

	return func(ctx context.Context) (T, error) {
		value, err := poll(ctx)

		var rendered any = map[string]any{
			"ok": err == nil,
		}
		if render != nil {
			rendered = render(value)
		}

		current := stringifyDebugValue(rendered)
		if current != last || err != nil {
			last = current
			if err != nil {
				t.Logf("[EventuallyDebug] %s err=%v value=%s", label, err, current)
			} else {
				t.Logf("[EventuallyDebug] %s value=%s", label, current)
			}
		}

		return value, err
	}
}

func WrapGetEventually[T client.Object](
	t *testing.T,
	label string,
	poll ContextPollFunc[T],
) ContextPollFunc[T] {
	t.Helper()

	return WrapEventually(t, label, poll, func(object T) any {
		return SnapshotObject(object)
	})
}

func WrapDeploymentStatusEventually(
	t *testing.T,
	label string,
	poll ContextPollFunc[*appsv1.Deployment],
) ContextPollFunc[*appsv1.Deployment] {
	t.Helper()

	return WrapEventually(t, label, poll, SnapshotDeploymentStatus)
}

func SnapshotObject(object client.Object) any {
	if object == nil {
		return nil
	}

	raw, err := json.Marshal(object)
	if err != nil {
		return map[string]any{
			"name":      object.GetName(),
			"namespace": object.GetNamespace(),
			"error":     fmt.Sprintf("marshal object: %v", err),
		}
	}

	return map[string]any{
		"name":      object.GetName(),
		"namespace": object.GetNamespace(),
		"object":    json.RawMessage(raw),
	}
}

func SnapshotDeploymentStatus(deployment *appsv1.Deployment) any {
	if deployment == nil {
		return nil
	}

	raw, err := json.Marshal(deployment.Status)
	if err != nil {
		return map[string]any{
			"name":      deployment.GetName(),
			"namespace": deployment.GetNamespace(),
			"error":     fmt.Sprintf("marshal deployment status: %v", err),
		}
	}

	return map[string]any{
		"name":      deployment.GetName(),
		"namespace": deployment.GetNamespace(),
		"status":    json.RawMessage(raw),
	}
}

func stringifyDebugValue(value any) string {
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Sprintf("%+v", value)
	}

	return string(data)
}
