package predicate

import (
	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/toolchain-common/pkg/condition"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

var _ predicate.Predicate = (*ToolchainConditionChanged)(nil)

type (
	GetToolchainConditions func(client.Object) []toolchainv1alpha1.Condition

	// ToolchainConditionChanged is a predicate to check that some conditions changed in a toolchain object.
	ToolchainConditionChanged struct {
		// GetConditions is function used to obtain the conditions from an object. There is no generic way of
		// doing that using the standard k8s api.
		GetConditions GetToolchainConditions

		// Type is the type of the condition to look at
		Type toolchainv1alpha1.ConditionType
	}
)

// Create implements predicate.Predicate.
func (t *ToolchainConditionChanged) Create(event.CreateEvent) bool {
	return true
}

// Delete implements predicate.Predicate.
func (t *ToolchainConditionChanged) Delete(event.DeleteEvent) bool {
	return true
}

// Generic implements predicate.Predicate.
func (t *ToolchainConditionChanged) Generic(event.GenericEvent) bool {
	return true
}

func (t *ToolchainConditionChanged) Update(evt event.UpdateEvent) bool {
	oldConds := t.GetConditions(evt.ObjectOld)
	newConds := t.GetConditions(evt.ObjectNew)

	oldCond, oldOk := condition.FindConditionByType(oldConds, t.Type)
	newCond, newOk := condition.FindConditionByType(newConds, t.Type)

	if oldOk != newOk {
		// the condition appeared or disappeared
		return true
	}

	if !oldOk {
		// neither the old nor the new version contains a condition of the required type
		return false
	}

	// we're intentionally ignoring changes to *Time fields of the conditions. If nothing else changed, those do not represent a factual
	// change in the condition.
	return oldCond.Status != newCond.Status || oldCond.Reason != newCond.Reason || oldCond.Message != newCond.Message
}
