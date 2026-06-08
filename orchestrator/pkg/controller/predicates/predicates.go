// Package predicates provides controller-runtime predicate helpers for the orchestrator.
package predicates

import (
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

// CreateOrResourceVersionChanged lets create events through and filters updates
// down to real object changes, including status updates.
func CreateOrResourceVersionChanged() predicate.Predicate {
	return predicate.Funcs{
		CreateFunc: func(_ event.CreateEvent) bool {
			return true
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			if e.ObjectOld == nil || e.ObjectNew == nil {
				return false
			}

			return e.ObjectNew.GetResourceVersion() != e.ObjectOld.GetResourceVersion()
		},
		DeleteFunc: func(_ event.DeleteEvent) bool {
			return false
		},
		GenericFunc: func(_ event.GenericEvent) bool {
			return false
		},
	}
}

// LogAllEvents logs each controller event with the given message and lets it pass through.
func LogAllEvents(msg string) predicate.Predicate {
	return predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			ctrl.Log.Info(
				msg,
				"event", "CREATE",
				"name", objectRef(e.Object),
				"gvk", e.Object.GetObjectKind().GroupVersionKind(),
			)

			return true
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			ctrl.Log.Info(
				msg,
				"event", "UPDATE",
				"name", objectRef(e.ObjectNew),
				"gvk", e.ObjectNew.GetObjectKind().GroupVersionKind(),
			)

			return true
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			ctrl.Log.Info(
				msg,
				"event", "DELETE",
				"name", objectRef(e.Object),
				"gvk", e.Object.GetObjectKind().GroupVersionKind(),
			)

			return true
		},
		GenericFunc: func(e event.GenericEvent) bool {
			ctrl.Log.Info(
				msg,
				"event", "GENERIC",
				"name", objectRef(e.Object),
				"gvk", e.Object.GetObjectKind().GroupVersionKind(),
			)

			return true
		},
	}
}

func objectRef(obj client.Object) string {
	if obj.GetNamespace() == "" {
		return obj.GetName()
	}

	return obj.GetNamespace() + "/" + obj.GetName()
}
