package deactivation

import (
	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

// CreateAndUpdateOnlyPredicate will filter out all events except Create and Update
type CreateAndUpdateOnlyPredicate struct {
}

// Update implements default UpdateEvent filter for validating no generation change
func (CreateAndUpdateOnlyPredicate) Update(_ event.UpdateEvent) bool {
	return true
}

// Create implements Predicate
func (CreateAndUpdateOnlyPredicate) Create(_ event.CreateEvent) bool {
	return true
}

// Delete implements Predicate
func (CreateAndUpdateOnlyPredicate) Delete(_ event.DeleteEvent) bool {
	return false
}

// Generic implements Predicate
func (CreateAndUpdateOnlyPredicate) Generic(_ event.GenericEvent) bool {
	return false
}

type GenerationOrConditionsChangedPredicate struct {
	predicate.GenerationChangedPredicate
}

// Update implements default UpdateEvent filter for validating no generation change
func (GenerationOrConditionsChangedPredicate) Update(e event.UpdateEvent) bool {
	if e.ObjectOld == nil || e.ObjectNew == nil {
		return false
	}

	if e.ObjectNew.GetGeneration() != e.ObjectOld.GetGeneration() {
		return true
	}

	switch objNew := e.ObjectNew.(type) {
	case *toolchainv1alpha1.UserSignup:
		switch objOld := e.ObjectOld.(type) {
		case *toolchainv1alpha1.UserSignup:
			if !test.ConditionsMatch(objOld.Status.Conditions, objNew.Status.Conditions...) {
				return true
			}
		}
	}

	return false
}

/*
func ConditionsMatch(first, second []toolchainv1alpha1.Condition) bool {
	if len(first) != len(second) {
		return false
	}
	for _, c := range first {
		if !ContainsCondition(second, c) {
			return false
		}
	}
	for _, c := range second {
		if !ContainsCondition(first, c) {
			return false
		}
	}
	return true
}

// ContainsCondition returns true if the specified list of conditions contains the specified condition and the statuses of the conditions match.
// LastTransitionTime is ignored.
func ContainsCondition(conditions []toolchainv1alpha1.Condition, contains toolchainv1alpha1.Condition) bool {
	for _, c := range conditions {
		if c.Type == contains.Type {
			return contains.Status == c.Status && contains.Reason == c.Reason && contains.Message == c.Message
		}
	}
	return false
}
*/
