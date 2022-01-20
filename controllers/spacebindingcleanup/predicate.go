package spacebindingcleanup

import (
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

var _ predicate.Predicate = OnlyDeleteAndGenericPredicate{}

type OnlyDeleteAndGenericPredicate struct {
}

func (p OnlyDeleteAndGenericPredicate) Create(e event.CreateEvent) bool {
	return false
}

func (p OnlyDeleteAndGenericPredicate) Update(e event.UpdateEvent) bool {
	return false
}

func (p OnlyDeleteAndGenericPredicate) Delete(e event.DeleteEvent) bool {
	return true
}

func (p OnlyDeleteAndGenericPredicate) Generic(e event.GenericEvent) bool {
	return true
}
