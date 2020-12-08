package usersignup

import (
	"github.com/codeready-toolchain/host-operator/pkg/controller/hostoperatorconfig"
	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/pkg/apis/toolchain/v1alpha1"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	controllerPredicate "sigs.k8s.io/controller-runtime/pkg/predicate"
)

var changedLog = logf.Log.WithName("user_signup_changed_predicate")

type UserSignupChangedPredicate struct {
	controllerPredicate.Funcs
}

// Update filters update events and let the reconcile loop to be triggered when any of the following conditions is met:
//
// * generation number has changed
//
// * annotation toolchain.dev.openshift.com/user-email has changed
//
// * label toolchain.dev.openshift.com/email-hash has changed
func (p UserSignupChangedPredicate) Update(e event.UpdateEvent) bool {
	if !checkMetaObjects(changedLog, e) {
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

var configLog = logf.Log.WithName("automatic_approval_predicate")

// OnlyWhenAutomaticApprovalIsEnabled let the reconcile to be triggered only when the automatic approval is enabled
type OnlyWhenAutomaticApprovalIsEnabled struct {
	client client.Client
}

// Update implements default UpdateEvent filter for validating no generation change
func (p OnlyWhenAutomaticApprovalIsEnabled) Update(e event.UpdateEvent) bool {
	if !checkMetaObjects(configLog, e) {
		return false
	}
	return p.checkIfAutomaticApprovalIsEnabled(e.MetaNew.GetNamespace())
}

// Create implements Predicate
func (OnlyWhenAutomaticApprovalIsEnabled) Create(e event.CreateEvent) bool {
	return false
}

// Delete implements Predicate
func (OnlyWhenAutomaticApprovalIsEnabled) Delete(e event.DeleteEvent) bool {
	return false
}

// Generic implements Predicate
func (p OnlyWhenAutomaticApprovalIsEnabled) Generic(e event.GenericEvent) bool {
	if e.Meta == nil {
		log.Error(nil, "Generic event has no object metadata", "event", e)
		return false
	}
	return p.checkIfAutomaticApprovalIsEnabled(e.Meta.GetNamespace())
}

func (p OnlyWhenAutomaticApprovalIsEnabled) checkIfAutomaticApprovalIsEnabled(namespace string) bool {
	config, err := hostoperatorconfig.GetConfig(p.client, namespace)
	if err != nil {
		configLog.Error(nil, "unable to get HostOperatorConfig resource", "namespace", namespace)
		return false
	}
	return config.AutomaticApproval.Enabled
}

func checkMetaObjects(log logr.Logger, e event.UpdateEvent) bool {
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
	return true
}
