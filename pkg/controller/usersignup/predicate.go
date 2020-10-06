package usersignup

import (
	toolchainv1alpha1 "github.com/codeready-toolchain/api/pkg/apis/toolchain/v1alpha1"
	"github.com/codeready-toolchain/host-operator/pkg/metrics"
	"sigs.k8s.io/controller-runtime/pkg/event"
	controllerPredicate "sigs.k8s.io/controller-runtime/pkg/predicate"
)

type UserSignupChangedPredicate struct {
	controllerPredicate.Funcs
}

// Update implements default UpdateEvent filter for validating generation change
func (p UserSignupChangedPredicate) Update(e event.UpdateEvent) bool {
	if e.MetaOld == nil {
		log.Error(nil, "Update event has no old metadata", "event", e)
		return false
	}
	if e.ObjectOld == nil {
		log.Error(nil, "Update event has no old runtime object to update", "event", e)
		return false
	}
	if e.ObjectNew == nil {
		log.Error(nil, "Update event has no new runtime object for update", "event", e)
		return false
	}
	if e.MetaNew == nil {
		log.Error(nil, "Update event has no new metadata", "event", e)
		return false
	}
	if e.MetaNew.GetGeneration() == e.MetaOld.GetGeneration() &&
		!p.AnnotationChanged(e, toolchainv1alpha1.UserSignupUserEmailAnnotationKey) &&
		!p.LabelChanged(e, toolchainv1alpha1.UserSignupUserEmailHashLabelKey) {
		return false
	}
	return true
}

func (p UserSignupChangedPredicate) AnnotationChanged(e event.UpdateEvent, annotationName string) bool {
	return e.MetaOld.GetAnnotations()[annotationName] != e.MetaNew.GetAnnotations()[annotationName]
}

func (p UserSignupChangedPredicate) LabelChanged(e event.UpdateEvent, labelName string) bool {
	return e.MetaOld.GetLabels()[labelName] != e.MetaNew.GetLabels()[labelName]
}

type UserSignupCreatedPredicate struct {
	controllerPredicate.Funcs
}

// Create implements default CreateEvent filter, in this case we use it as a pass-through and only use it for metrics
func (p UserSignupCreatedPredicate) Create(e event.CreateEvent) bool {
	metrics.IncrementUserSignupTotal()
	return true
}
