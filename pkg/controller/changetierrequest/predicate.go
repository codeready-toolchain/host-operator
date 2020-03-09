package changetierrequest

import (
	"github.com/codeready-toolchain/api/pkg/apis/toolchain/v1alpha1"
	"github.com/codeready-toolchain/toolchain-common/pkg/condition"
	"github.com/operator-framework/operator-sdk/pkg/predicate"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/event"
)

// onlyGenerationChangedAndIsNotComplete allows only ChangeTierRequest resources whose
// generation has been changed and whose status conditions doesn't contain the complete one
type onlyGenerationChangedAndIsNotComplete struct {
	*predicate.GenerationChangedPredicate
}

// Update uses the one from predicate.GenerationChangedPredicate plus checks if the ChangeTierRequest is not complete
func (p onlyGenerationChangedAndIsNotComplete) Update(e event.UpdateEvent) bool {
	return p.GenerationChangedPredicate.Update(e) && onlyWhenNotComplete(e.ObjectNew)
}

// Create uses the one from predicate.GenerationChangedPredicate plus checks if the ChangeTierRequest is not complete
func (p onlyGenerationChangedAndIsNotComplete) Create(e event.CreateEvent) bool {
	return p.GenerationChangedPredicate.Create(e) && onlyWhenNotComplete(e.Object)
}

// Delete returns always false - we are not interested in deleted resources
func (onlyGenerationChangedAndIsNotComplete) Delete(e event.DeleteEvent) bool {
	return false
}

// Generic uses the one from predicate.GenerationChangedPredicate plus checks if the ChangeTierRequest is not complete
func (p onlyGenerationChangedAndIsNotComplete) Generic(e event.GenericEvent) bool {
	return p.GenerationChangedPredicate.Generic(e) && onlyWhenNotComplete(e.Object)
}

func onlyWhenNotComplete(obj runtime.Object) bool {
	changeTierRequest, ok := obj.(*v1alpha1.ChangeTierRequest)
	if !ok {
		return false
	}
	return !condition.IsTrue(changeTierRequest.Status.Conditions, v1alpha1.ChangeTierRequestComplete)
}
