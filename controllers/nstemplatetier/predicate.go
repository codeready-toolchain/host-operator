package nstemplatetier

import (
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

type IsBeingDeletedPredicate struct{}

// Create implements predicate.TypedPredicate.
func (i IsBeingDeletedPredicate) Create(event.TypedCreateEvent[client.Object]) bool {
	return false
}

// Delete implements predicate.TypedPredicate.
func (i IsBeingDeletedPredicate) Delete(event.TypedDeleteEvent[client.Object]) bool {
	return false
}

// Generic implements predicate.TypedPredicate.
func (i IsBeingDeletedPredicate) Generic(e event.TypedGenericEvent[client.Object]) bool {
	if e.Object == nil {
		return false
	}
	return !e.Object.GetDeletionTimestamp().IsZero()
}

// Update implements predicate.TypedPredicate.
func (i IsBeingDeletedPredicate) Update(e event.TypedUpdateEvent[client.Object]) bool {
	if e.ObjectNew == nil {
		return false
	}
	return !e.ObjectNew.GetDeletionTimestamp().IsZero()
}

var _ predicate.Predicate = IsBeingDeletedPredicate{}
