package socialevent

import "sigs.k8s.io/controller-runtime/pkg/event"

// CreateOnlyPredicate will filter out all events except `Create`
type CreateOnlyPredicate struct {
}

// Create implements Predicate
func (CreateOnlyPredicate) Create(e event.CreateEvent) bool {
	return true
}

// Update implements default UpdateEvent filter for validating no generation change
func (CreateOnlyPredicate) Update(e event.UpdateEvent) bool {
	return false
}

// Delete implements Predicate
func (CreateOnlyPredicate) Delete(e event.DeleteEvent) bool {
	return false
}

// Generic implements Predicate
func (CreateOnlyPredicate) Generic(e event.GenericEvent) bool {
	return false
}
