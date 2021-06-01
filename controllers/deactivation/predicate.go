package deactivation

import "sigs.k8s.io/controller-runtime/pkg/event"

// CreateAndUpdateOnlyPredicate will filter out all events except Create and Update
type CreateAndUpdateOnlyPredicate struct {
}

// Update implements default UpdateEvent filter for validating no generation change
func (CreateAndUpdateOnlyPredicate) Update(e event.UpdateEvent) bool {
	return true
}

// Create implements Predicate
func (CreateAndUpdateOnlyPredicate) Create(e event.CreateEvent) bool {
	return true
}

// Delete implements Predicate
func (CreateAndUpdateOnlyPredicate) Delete(e event.DeleteEvent) bool {
	return false
}

// Generic implements Predicate
func (CreateAndUpdateOnlyPredicate) Generic(e event.GenericEvent) bool {
	return false
}
