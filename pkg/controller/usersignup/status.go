package usersignup

import (
	"context"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/pkg/apis/toolchain/v1alpha1"
	commonCondition "github.com/codeready-toolchain/toolchain-common/pkg/condition"
	"github.com/go-logr/logr"
	errs "github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type statusUpdater struct {
	client client.Client
}

func (u *statusUpdater) setStatusApprovedAutomatically(userSignup *toolchainv1alpha1.UserSignup, message string) error {
	return u.updateStatusConditions(
		userSignup,
		toolchainv1alpha1.Condition{
			Type:    toolchainv1alpha1.UserSignupApproved,
			Status:  corev1.ConditionTrue,
			Reason:  toolchainv1alpha1.UserSignupApprovedAutomaticallyReason,
			Message: message,
		})
}

var statusApprovedByAdmin = func(_ string) toolchainv1alpha1.Condition {
	return toolchainv1alpha1.Condition{
		Type:   toolchainv1alpha1.UserSignupApproved,
		Status: corev1.ConditionTrue,
		Reason: toolchainv1alpha1.UserSignupApprovedByAdminReason,
	}
}

var statusPendingApproval = func(message string) toolchainv1alpha1.Condition {
	return toolchainv1alpha1.Condition{
		Type:    toolchainv1alpha1.UserSignupApproved,
		Status:  corev1.ConditionFalse,
		Reason:  toolchainv1alpha1.UserSignupPendingApprovalReason,
		Message: message,
	}
}

var statusIncompletePendingApproval = func(message string) toolchainv1alpha1.Condition {
	return toolchainv1alpha1.Condition{
		Type:    toolchainv1alpha1.UserSignupComplete,
		Status:  corev1.ConditionFalse,
		Reason:  toolchainv1alpha1.UserSignupPendingApprovalReason,
		Message: message,
	}
}

func (u *statusUpdater) setStatusFailedToReadUserApprovalPolicy(userSignup *toolchainv1alpha1.UserSignup, message string) error {
	return u.updateStatusConditions(
		userSignup,
		toolchainv1alpha1.Condition{
			Type:    toolchainv1alpha1.UserSignupComplete,
			Status:  corev1.ConditionFalse,
			Reason:  toolchainv1alpha1.UserSignupFailedToReadUserApprovalPolicyReason,
			Message: message,
		})
}

func (u *statusUpdater) setStatusInvalidMURState(userSignup *toolchainv1alpha1.UserSignup, message string) error {
	return u.updateStatusConditions(
		userSignup,
		toolchainv1alpha1.Condition{
			Type:    toolchainv1alpha1.UserSignupComplete,
			Status:  corev1.ConditionFalse,
			Reason:  toolchainv1alpha1.UserSignupInvalidMURStateReason,
			Message: message,
		})
}

func (u *statusUpdater) setStatusFailedToCreateMUR(userSignup *toolchainv1alpha1.UserSignup, message string) error {
	return u.updateStatusConditions(
		userSignup,
		toolchainv1alpha1.Condition{
			Type:    toolchainv1alpha1.UserSignupComplete,
			Status:  corev1.ConditionFalse,
			Reason:  toolchainv1alpha1.UserSignupUnableToCreateMURReason,
			Message: message,
		})
}

func (u *statusUpdater) setStatusFailedToDeleteMUR(userSignup *toolchainv1alpha1.UserSignup, message string) error {
	return u.updateStatusConditions(
		userSignup,
		toolchainv1alpha1.Condition{
			Type:    toolchainv1alpha1.UserSignupComplete,
			Status:  corev1.ConditionFalse,
			Reason:  toolchainv1alpha1.UserSignupUnableToDeleteMURReason,
			Message: message,
		})
}

func (u *statusUpdater) set(conditionCreators ...func(message string) toolchainv1alpha1.Condition) func(userSignup *toolchainv1alpha1.UserSignup, message string) error {
	return func(userSignup *toolchainv1alpha1.UserSignup, message string) error {
		conditions := make([]toolchainv1alpha1.Condition, len(conditionCreators))
		for index, createCondition := range conditionCreators {
			conditions[index] = createCondition(message)
		}
		return u.updateStatusConditions(userSignup, conditions...)
	}
}

var statusNoClustersAvailable = func(message string) toolchainv1alpha1.Condition {
	return toolchainv1alpha1.Condition{
		Type:    toolchainv1alpha1.UserSignupComplete,
		Status:  corev1.ConditionFalse,
		Reason:  toolchainv1alpha1.UserSignupNoClusterAvailableReason,
		Message: message,
	}
}

func (u *statusUpdater) setStatusNoTemplateTierAvailable(userSignup *toolchainv1alpha1.UserSignup, message string) error {
	return u.updateStatusConditions(
		userSignup,
		toolchainv1alpha1.Condition{
			Type:    toolchainv1alpha1.UserSignupComplete,
			Status:  corev1.ConditionFalse,
			Reason:  toolchainv1alpha1.UserSignupNoTemplateTierAvailableReason,
			Message: message,
		})
}

func (u *statusUpdater) setStatusBanning(userSignup *toolchainv1alpha1.UserSignup, message string) error {
	return u.updateStatusConditions(
		userSignup,
		toolchainv1alpha1.Condition{
			Type:    toolchainv1alpha1.UserSignupComplete,
			Status:  corev1.ConditionFalse,
			Reason:  toolchainv1alpha1.UserSignupUserBanningReason,
			Message: message,
		})
}

// setStatusBanned sets the Complete status to True, as the banning operation has been successful (with a reason of "Banned")
func (u *statusUpdater) setStatusBanned(userSignup *toolchainv1alpha1.UserSignup, message string) error {
	return u.updateStatusConditions(
		userSignup,
		toolchainv1alpha1.Condition{
			Type:    toolchainv1alpha1.UserSignupComplete,
			Status:  corev1.ConditionTrue,
			Reason:  toolchainv1alpha1.UserSignupUserBannedReason,
			Message: message,
		})
}

func (u *statusUpdater) setStatusDeactivating(userSignup *toolchainv1alpha1.UserSignup, message string) error {
	return u.updateStatusConditions(
		userSignup,
		toolchainv1alpha1.Condition{
			Type:    toolchainv1alpha1.UserSignupComplete,
			Status:  corev1.ConditionFalse,
			Reason:  toolchainv1alpha1.UserSignupUserDeactivatingReason,
			Message: message,
		})
}

func (u *statusUpdater) setStatusDeactivated(userSignup *toolchainv1alpha1.UserSignup, message string) error {
	return u.updateStatusConditions(
		userSignup,
		toolchainv1alpha1.Condition{
			Type:    toolchainv1alpha1.UserSignupComplete,
			Status:  corev1.ConditionTrue,
			Reason:  toolchainv1alpha1.UserSignupUserDeactivatedReason,
			Message: message,
		})
}

func (u *statusUpdater) setStatusFailedToReadBannedUsers(userSignup *toolchainv1alpha1.UserSignup, message string) error {
	return u.updateStatusConditions(
		userSignup,
		toolchainv1alpha1.Condition{
			Type:    toolchainv1alpha1.UserSignupComplete,
			Status:  corev1.ConditionFalse,
			Reason:  toolchainv1alpha1.UserSignupFailedToReadBannedUsersReason,
			Message: message,
		})
}

func (u *statusUpdater) setStatusInvalidMissingUserEmailAnnotation(userSignup *toolchainv1alpha1.UserSignup, message string) error {
	return u.updateStatusConditions(
		userSignup,
		toolchainv1alpha1.Condition{
			Type:    toolchainv1alpha1.UserSignupComplete,
			Status:  corev1.ConditionFalse,
			Reason:  toolchainv1alpha1.UserSignupMissingUserEmailAnnotationReason,
			Message: message,
		})
}

func (u *statusUpdater) setStatusMissingEmailHash(userSignup *toolchainv1alpha1.UserSignup, message string) error {
	return u.updateStatusConditions(
		userSignup,
		toolchainv1alpha1.Condition{
			Type:    toolchainv1alpha1.UserSignupComplete,
			Status:  corev1.ConditionFalse,
			Reason:  toolchainv1alpha1.UserSignupMissingEmailHashLabelReason,
			Message: message,
		})
}

func (u *statusUpdater) setStatusInvalidEmailHash(userSignup *toolchainv1alpha1.UserSignup, message string) error {
	return u.updateStatusConditions(
		userSignup,
		toolchainv1alpha1.Condition{
			Type:    toolchainv1alpha1.UserSignupComplete,
			Status:  corev1.ConditionFalse,
			Reason:  toolchainv1alpha1.UserSignupInvalidEmailHashLabelReason,
			Message: message,
		})
}

func (u *statusUpdater) setStatusVerificationRequired(userSignup *toolchainv1alpha1.UserSignup, message string) error {
	return u.updateStatusConditions(
		userSignup,
		toolchainv1alpha1.Condition{
			Type:    toolchainv1alpha1.UserSignupComplete,
			Status:  corev1.ConditionFalse,
			Reason:  toolchainv1alpha1.UserSignupVerificationRequiredReason,
			Message: message,
		})
}

func (u *statusUpdater) setStatusFailedToUpdateApprovedLabel(userSignup *toolchainv1alpha1.UserSignup, message string) error {
	return u.updateStatusConditions(
		userSignup,
		toolchainv1alpha1.Condition{
			Type:    toolchainv1alpha1.UserSignupComplete,
			Status:  corev1.ConditionFalse,
			Reason:  toolchainv1alpha1.UserSignupUnableToUpdateApprovedLabelReason,
			Message: message,
		})
}

func (u *statusUpdater) updateStatus(logger logr.Logger, userSignup *toolchainv1alpha1.UserSignup,
	statusUpdater func(userAcc *toolchainv1alpha1.UserSignup, message string) error) error {

	if err := statusUpdater(userSignup, ""); err != nil {
		logger.Error(err, "status update failed")
		return err
	}

	return nil
}

// updateCompleteStatus updates the `CompliantUsername` and `Conditions` in the status, should only be invoked on completion because
// both completion and the compliant username require the master user record to be created.
func (u *statusUpdater) updateCompleteStatus(compliantUsername string) func(userSignup *toolchainv1alpha1.UserSignup, message string) error {
	return func(userSignup *toolchainv1alpha1.UserSignup, message string) error {

		usernameUpdated := userSignup.Status.CompliantUsername != compliantUsername
		userSignup.Status.CompliantUsername = compliantUsername

		var conditionUpdated bool
		userSignup.Status.Conditions, conditionUpdated = commonCondition.AddOrUpdateStatusConditions(userSignup.Status.Conditions,
			toolchainv1alpha1.Condition{
				Type:    toolchainv1alpha1.UserSignupComplete,
				Status:  corev1.ConditionTrue,
				Reason:  "",
				Message: message,
			})

		if !usernameUpdated && !conditionUpdated {
			// Nothing changed
			return nil
		}
		return u.client.Status().Update(context.TODO(), userSignup)
	}
}

// wrapErrorWithStatusUpdate wraps the error and update the UserSignup status. If the update fails then the error is logged.
func (u *statusUpdater) wrapErrorWithStatusUpdate(logger logr.Logger, userSignup *toolchainv1alpha1.UserSignup,
	statusUpdater StatusUpdater, err error, format string, args ...interface{}) error {
	if err == nil {
		return nil
	}
	if err := statusUpdater(userSignup, err.Error()); err != nil {
		logger.Error(err, "Error updating UserSignup status")
	}
	return errs.Wrapf(err, format, args...)
}

func (u *statusUpdater) updateStatusConditions(userSignup *toolchainv1alpha1.UserSignup, newConditions ...toolchainv1alpha1.Condition) error {
	var updated bool
	userSignup.Status.Conditions, updated = commonCondition.AddOrUpdateStatusConditions(userSignup.Status.Conditions, newConditions...)
	if !updated {
		// Nothing changed
		return nil
	}
	return u.client.Status().Update(context.TODO(), userSignup)
}
