// Package predicates provides controller-runtime predicate helpers for the orchestrator.
package predicates

import (
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

// Named matches only the object with the exact namespace/name pair.
func Named(target types.NamespacedName) predicate.Predicate {
	return predicate.NewPredicateFuncs(func(obj client.Object) bool {
		return obj.GetNamespace() == target.Namespace && obj.GetName() == target.Name
	})
}
