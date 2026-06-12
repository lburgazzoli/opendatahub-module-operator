// Package predicates provides controller-runtime predicates for the workbenches module,
// including local helpers and re-exports from the shared framework predicate packages.
package predicates

import (
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	fwpredicates "github.com/opendatahub-io/odh-platform-utilities/framework/controller/predicates"
	labelpred "github.com/opendatahub-io/odh-platform-utilities/framework/controller/predicates/label"
)

// Or composes predicates with logical OR — an event passes if any predicate accepts it.
var Or = predicate.Or[client.Object]

// And composes predicates with logical AND — an event passes only if all predicates accept it.
var And = predicate.And[client.Object]

// Aliases from the shared framework predicate packages.
var (
	DefaultDeploymentPredicate = fwpredicates.DefaultDeploymentPredicate
)

// ForComponentLabel returns a predicate that passes only when the object carries
// the given label with the given value.
var ForComponentLabel = labelpred.ForLabel

// CreatedOrDeletedNamed returns a predicate that fires only on Create and Delete
// events for the object with the given name. Update and Generic events are
// explicitly suppressed so that routine status updates do not trigger unnecessary reconciles.
func CreatedOrDeletedNamed(name string) predicate.Predicate {
	return predicate.Funcs{
		CreateFunc:  func(e event.CreateEvent) bool { return e.Object.GetName() == name },
		UpdateFunc:  func(event.UpdateEvent) bool { return false },
		DeleteFunc:  func(e event.DeleteEvent) bool { return e.Object.GetName() == name },
		GenericFunc: func(event.GenericEvent) bool { return false },
	}
}
