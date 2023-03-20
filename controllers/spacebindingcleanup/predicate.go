package spacebindingcleanup

import (
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

var _ predicate.Predicate = OnlyDeleteAndGenericPredicate{}

type OnlyDeleteAndGenericPredicate struct {
}

func (p OnlyDeleteAndGenericPredicate) Create(_ event.CreateEvent) bool {
	return false
}

func (p OnlyDeleteAndGenericPredicate) Update(_ event.UpdateEvent) bool {
	return false
}

func (p OnlyDeleteAndGenericPredicate) Delete(_ event.DeleteEvent) bool {
	return true
}

func (p OnlyDeleteAndGenericPredicate) Generic(_ event.GenericEvent) bool {
	return true
}
