package usersignupcleanup

import (
	toolchainv1alpha1 "github.com/codeready-toolchain/api/pkg/apis/toolchain/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/event"
	controllerPredicate "sigs.k8s.io/controller-runtime/pkg/predicate"
)

type UserSignupCleanupPredicate struct {
	controllerPredicate.Funcs
}

// Update filters update events and let the reconcile loop to be triggered when any of the following conditions is met:
//
// * label toolchain.dev.openshift.com/state is equal to not-ready or deactivated
//
func (p UserSignupCleanupPredicate) Update(e event.UpdateEvent) bool {

	if state, ok := e.MetaNew.GetLabels()[toolchainv1alpha1.UserSignupStateLabelKey]; ok {

		return state == toolchainv1alpha1.UserSignupStateLabelValueNotReady ||
			state == toolchainv1alpha1.UserSignupStateLabelValueDeactivated
	}

	return false
}
