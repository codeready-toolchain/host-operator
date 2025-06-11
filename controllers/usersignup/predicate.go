package usersignup

import (
	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/host-operator/controllers/toolchainconfig"

	"github.com/go-logr/logr"
	ctrl "sigs.k8s.io/controller-runtime"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	runtimeevent "sigs.k8s.io/controller-runtime/pkg/event"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	controllerpredicate "sigs.k8s.io/controller-runtime/pkg/predicate"
)

var changedLog = logf.Log.WithName("user_signup_changed_predicate")

type UserSignupChangedPredicate struct { // nolint: revive
	controllerpredicate.Funcs
}

var _ controllerpredicate.Predicate = UserSignupChangedPredicate{}
var predicateLog = ctrl.Log.WithName("UserSignupChangedPredicate")

// Update filters update events and let the reconcile loop to be triggered when any of the following conditions is met:
// * generation number has changed
// * annotation toolchain.dev.openshift.com/user-email has changed
// * annotation toolchain.dev.openshift.com/migration-in-progress was removed
// * label toolchain.dev.openshift.com/email-hash has changed
func (p UserSignupChangedPredicate) Update(e runtimeevent.UpdateEvent) bool {
	if !checkMetaObjects(changedLog, e) {
		return false
	}
	return e.ObjectNew.GetGeneration() != e.ObjectOld.GetGeneration() ||
		p.labelChanged(e, toolchainv1alpha1.UserSignupUserEmailHashLabelKey)
}

func (p UserSignupChangedPredicate) labelChanged(e runtimeevent.UpdateEvent, labelName string) bool {
	return e.ObjectOld.GetLabels()[labelName] != e.ObjectNew.GetLabels()[labelName]
}

var configLog = logf.Log.WithName("automatic_approval_predicate")

// OnlyWhenAutomaticApprovalIsEnabled let the reconcile to be triggered only when the automatic approval is enabled
type OnlyWhenAutomaticApprovalIsEnabled struct {
	client runtimeclient.Client
}

// Update implements default UpdateEvent filter for validating no generation change
func (p OnlyWhenAutomaticApprovalIsEnabled) Update(e runtimeevent.UpdateEvent) bool {
	if !checkMetaObjects(configLog, e) {
		return false
	}
	return p.checkIfAutomaticApprovalIsEnabled(e.ObjectNew.GetNamespace())
}

// Create implements Predicate
func (OnlyWhenAutomaticApprovalIsEnabled) Create(_ runtimeevent.CreateEvent) bool {
	return false
}

// Delete implements Predicate
func (OnlyWhenAutomaticApprovalIsEnabled) Delete(_ runtimeevent.DeleteEvent) bool {
	return false
}

// Generic implements Predicate
func (p OnlyWhenAutomaticApprovalIsEnabled) Generic(e runtimeevent.GenericEvent) bool {
	if e.Object == nil {
		predicateLog.Error(nil, "Generic event has no object", "event", e)
		return false
	}
	return p.checkIfAutomaticApprovalIsEnabled(e.Object.GetNamespace())
}

func (p OnlyWhenAutomaticApprovalIsEnabled) checkIfAutomaticApprovalIsEnabled(namespace string) bool {
	config, err := toolchainconfig.GetToolchainConfig(p.client)
	if err != nil {
		configLog.Error(err, "unable to get ToolchainConfig", "namespace", namespace)
		return false
	}
	return config.AutomaticApproval().IsEnabled()
}

func checkMetaObjects(log logr.Logger, e runtimeevent.UpdateEvent) bool {
	if e.ObjectOld == nil {
		log.Error(nil, "Update event has no old runtime object to update", "event", e)
		return false
	}
	if e.ObjectNew == nil {
		log.Error(nil, "Update event has no new runtime object for update", "event", e)
		return false
	}
	return true
}
