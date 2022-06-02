package usersignup

import (
	"context"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	commonCondition "github.com/codeready-toolchain/toolchain-common/pkg/condition"
	"github.com/go-logr/logr"
	errs "github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type StatusUpdater struct {
	Client client.Client
}

func (u *StatusUpdater) setStatusApprovedAutomatically(userSignup *toolchainv1alpha1.UserSignup, message string) error {
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

func (u *StatusUpdater) setStatusInvalidMURState(userSignup *toolchainv1alpha1.UserSignup, message string) error {
	return u.updateStatusConditions(
		userSignup,
		toolchainv1alpha1.Condition{
			Type:    toolchainv1alpha1.UserSignupComplete,
			Status:  corev1.ConditionFalse,
			Reason:  toolchainv1alpha1.UserSignupInvalidMURStateReason,
			Message: message,
		})
}

func (u *StatusUpdater) setStatusFailedToCreateMUR(userSignup *toolchainv1alpha1.UserSignup, message string) error {
	return u.updateStatusConditions(
		userSignup,
		toolchainv1alpha1.Condition{
			Type:    toolchainv1alpha1.UserSignupComplete,
			Status:  corev1.ConditionFalse,
			Reason:  toolchainv1alpha1.UserSignupUnableToCreateMURReason,
			Message: message,
		})
}

func (u *StatusUpdater) setStatusFailedToDeleteMUR(userSignup *toolchainv1alpha1.UserSignup, message string) error {
	return u.updateStatusConditions(
		userSignup,
		toolchainv1alpha1.Condition{
			Type:    toolchainv1alpha1.UserSignupComplete,
			Status:  corev1.ConditionFalse,
			Reason:  toolchainv1alpha1.UserSignupUnableToDeleteMURReason,
			Message: message,
		})
}

func (u *StatusUpdater) setStatusFailedToCreateSpace(userSignup *toolchainv1alpha1.UserSignup, message string) error {
	return u.updateStatusConditions(
		userSignup,
		toolchainv1alpha1.Condition{
			Type:    toolchainv1alpha1.UserSignupComplete,
			Status:  corev1.ConditionFalse,
			Reason:  toolchainv1alpha1.UserSignupUnableToCreateSpaceReason,
			Message: message,
		})
}

func (u *StatusUpdater) setStatusFailedToCreateSpaceBinding(userSignup *toolchainv1alpha1.UserSignup, message string) error {
	return u.updateStatusConditions(
		userSignup,
		toolchainv1alpha1.Condition{
			Type:    toolchainv1alpha1.UserSignupComplete,
			Status:  corev1.ConditionFalse,
			Reason:  toolchainv1alpha1.UserSignupUnableToCreateSpaceBindingReason,
			Message: message,
		})
}

func (u *StatusUpdater) set(conditionCreators ...func(message string) toolchainv1alpha1.Condition) func(userSignup *toolchainv1alpha1.UserSignup, message string) error {
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

func (u *StatusUpdater) setStatusNoUserTierAvailable(userSignup *toolchainv1alpha1.UserSignup, message string) error {
	return u.updateStatusConditions(
		userSignup,
		toolchainv1alpha1.Condition{
			Type:    toolchainv1alpha1.UserSignupComplete,
			Status:  corev1.ConditionFalse,
			Reason:  toolchainv1alpha1.UserSignupNoUserTierAvailableReason,
			Message: message,
		})
}

func (u *StatusUpdater) setStatusNoTemplateTierAvailable(userSignup *toolchainv1alpha1.UserSignup, message string) error {
	return u.updateStatusConditions(
		userSignup,
		toolchainv1alpha1.Condition{
			Type:    toolchainv1alpha1.UserSignupComplete,
			Status:  corev1.ConditionFalse,
			Reason:  toolchainv1alpha1.UserSignupNoTemplateTierAvailableReason,
			Message: message,
		})
}

func (u *StatusUpdater) setStatusBanning(userSignup *toolchainv1alpha1.UserSignup, message string) error {
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
func (u *StatusUpdater) setStatusBanned(userSignup *toolchainv1alpha1.UserSignup, message string) error {
	return u.updateStatusConditions(
		userSignup,
		toolchainv1alpha1.Condition{
			Type:    toolchainv1alpha1.UserSignupComplete,
			Status:  corev1.ConditionTrue,
			Reason:  toolchainv1alpha1.UserSignupUserBannedReason,
			Message: message,
		})
}

func (u *StatusUpdater) setStatusDeactivationInProgress(userSignup *toolchainv1alpha1.UserSignup, message string) error {
	return u.updateStatusConditions(
		userSignup,
		toolchainv1alpha1.Condition{
			Type:    toolchainv1alpha1.UserSignupComplete,
			Status:  corev1.ConditionFalse,
			Reason:  toolchainv1alpha1.UserSignupDeactivationInProgressReason,
			Message: message,
		})
}

func (u *StatusUpdater) setStatusDeactivated(userSignup *toolchainv1alpha1.UserSignup, message string) error {
	return u.updateStatusConditions(
		userSignup,
		toolchainv1alpha1.Condition{
			Type:    toolchainv1alpha1.UserSignupComplete,
			Status:  corev1.ConditionTrue,
			Reason:  toolchainv1alpha1.UserSignupUserDeactivatedReason,
			Message: message,
		})
}

const (
	UserMigrated toolchainv1alpha1.ConditionType = "UserMigrated"
)

func (u *StatusUpdater) setStatusMigrationFailedLookup(userSignup *toolchainv1alpha1.UserSignup, message string) error {
	return u.updateStatusConditions(
		userSignup,
		toolchainv1alpha1.Condition{
			Type:    UserMigrated,
			Status:  corev1.ConditionFalse,
			Reason:  "UserSignupLookupFailed",
			Message: message,
		})
}

func (u *StatusUpdater) setStatusMigrationFailedCreate(userSignup *toolchainv1alpha1.UserSignup, message string) error {
	return u.updateStatusConditions(
		userSignup,
		toolchainv1alpha1.Condition{
			Type:    UserMigrated,
			Status:  corev1.ConditionFalse,
			Reason:  "UserSignupCreateFailed",
			Message: message,
		})
}

func (u *StatusUpdater) setStatusMigrationFailedCleanup(userSignup *toolchainv1alpha1.UserSignup, message string) error {
	return u.updateStatusConditions(
		userSignup,
		toolchainv1alpha1.Condition{
			Type:    UserMigrated,
			Status:  corev1.ConditionFalse,
			Reason:  "UserSignupCleanupFailed",
			Message: message,
		})
}

func (u *StatusUpdater) setStatusMigrationSuccessful(userSignup *toolchainv1alpha1.UserSignup) error {
	// Cleanup the migrated condition from the status, if it exists
	conditions := []toolchainv1alpha1.Condition{}
	for _, cond := range userSignup.Status.Conditions {
		if cond.Type != UserMigrated {
			conditions = append(conditions, cond)
		}
	}

	userSignup.Status.Conditions = conditions
	return u.Client.Status().Update(context.TODO(), userSignup)
}

func (u *StatusUpdater) setStatusFailedToReadBannedUsers(userSignup *toolchainv1alpha1.UserSignup, message string) error {
	return u.updateStatusConditions(
		userSignup,
		toolchainv1alpha1.Condition{
			Type:    toolchainv1alpha1.UserSignupComplete,
			Status:  corev1.ConditionFalse,
			Reason:  toolchainv1alpha1.UserSignupFailedToReadBannedUsersReason,
			Message: message,
		})
}

func (u *StatusUpdater) setStatusInvalidMissingUserEmailAnnotation(userSignup *toolchainv1alpha1.UserSignup, message string) error {
	return u.updateStatusConditions(
		userSignup,
		toolchainv1alpha1.Condition{
			Type:    toolchainv1alpha1.UserSignupComplete,
			Status:  corev1.ConditionFalse,
			Reason:  toolchainv1alpha1.UserSignupMissingUserEmailAnnotationReason,
			Message: message,
		})
}

func (u *StatusUpdater) setStatusMissingEmailHash(userSignup *toolchainv1alpha1.UserSignup, message string) error {
	return u.updateStatusConditions(
		userSignup,
		toolchainv1alpha1.Condition{
			Type:    toolchainv1alpha1.UserSignupComplete,
			Status:  corev1.ConditionFalse,
			Reason:  toolchainv1alpha1.UserSignupMissingEmailHashLabelReason,
			Message: message,
		})
}

func (u *StatusUpdater) setStatusInvalidEmailHash(userSignup *toolchainv1alpha1.UserSignup, message string) error {
	return u.updateStatusConditions(
		userSignup,
		toolchainv1alpha1.Condition{
			Type:    toolchainv1alpha1.UserSignupComplete,
			Status:  corev1.ConditionFalse,
			Reason:  toolchainv1alpha1.UserSignupInvalidEmailHashLabelReason,
			Message: message,
		})
}

func (u *StatusUpdater) setStatusVerificationRequired(userSignup *toolchainv1alpha1.UserSignup, message string) error {
	return u.updateStatusConditions(
		userSignup,
		toolchainv1alpha1.Condition{
			Type:    toolchainv1alpha1.UserSignupComplete,
			Status:  corev1.ConditionFalse,
			Reason:  toolchainv1alpha1.UserSignupVerificationRequiredReason,
			Message: message,
		})
}

func (u *StatusUpdater) setStatusFailedToUpdateStateLabel(userSignup *toolchainv1alpha1.UserSignup, message string) error {
	return u.updateStatusConditions(
		userSignup,
		toolchainv1alpha1.Condition{
			Type:    toolchainv1alpha1.UserSignupComplete,
			Status:  corev1.ConditionFalse,
			Reason:  toolchainv1alpha1.UserSignupUnableToUpdateStateLabelReason,
			Message: message,
		})
}

func (u *StatusUpdater) setStatusFailedToUpdateAnnotation(userSignup *toolchainv1alpha1.UserSignup, message string) error {
	return u.updateStatusConditions(
		userSignup,
		toolchainv1alpha1.Condition{
			Type:    toolchainv1alpha1.UserSignupComplete,
			Status:  corev1.ConditionFalse,
			Reason:  toolchainv1alpha1.UserSignupUnableToUpdateAnnotationReason,
			Message: message,
		})
}

func (u *StatusUpdater) setStatusDeactivationNotificationCreated(userSignup *toolchainv1alpha1.UserSignup, _ string) error {
	return u.updateStatusConditions(
		userSignup,
		toolchainv1alpha1.Condition{
			Type:   toolchainv1alpha1.UserSignupUserDeactivatedNotificationCreated,
			Status: corev1.ConditionTrue,
			Reason: toolchainv1alpha1.UserSignupDeactivatedNotificationCRCreatedReason,
		})
}

func (u *StatusUpdater) setStatusDeactivationNotificationUserIsActive(userSignup *toolchainv1alpha1.UserSignup, _ string) error {
	return u.updateStatusConditions(
		userSignup,
		toolchainv1alpha1.Condition{
			Type:   toolchainv1alpha1.UserSignupUserDeactivatedNotificationCreated,
			Status: corev1.ConditionFalse,
			Reason: toolchainv1alpha1.UserSignupDeactivatedNotificationUserIsActiveReason,
		})
}

func (u *StatusUpdater) setStatusDeactivationNotificationCreationFailed(userSignup *toolchainv1alpha1.UserSignup, message string) error {
	return u.updateStatusConditions(
		userSignup,
		toolchainv1alpha1.Condition{
			Type:    toolchainv1alpha1.UserSignupUserDeactivatedNotificationCreated,
			Status:  corev1.ConditionFalse,
			Reason:  toolchainv1alpha1.UserSignupDeactivatedNotificationCRCreationFailedReason,
			Message: message,
		})
}

func (u *StatusUpdater) setStatusDeactivatingNotificationCreated(userSignup *toolchainv1alpha1.UserSignup, _ string) error {
	return u.updateStatusConditions(
		userSignup,
		toolchainv1alpha1.Condition{
			Type:   toolchainv1alpha1.UserSignupUserDeactivatingNotificationCreated,
			Status: corev1.ConditionTrue,
			Reason: toolchainv1alpha1.UserSignupDeactivatingNotificationCRCreatedReason,
		})
}

func (u *StatusUpdater) setStatusDeactivatingNotificationNotInPreDeactivation(userSignup *toolchainv1alpha1.UserSignup, _ string) error {
	return u.updateStatusConditions(
		userSignup,
		toolchainv1alpha1.Condition{
			Type:   toolchainv1alpha1.UserSignupUserDeactivatingNotificationCreated,
			Status: corev1.ConditionFalse,
			Reason: toolchainv1alpha1.UserSignupDeactivatingNotificationUserNotInPreDeactivationReason,
		})
}

func (u *StatusUpdater) setStatusDeactivatingNotificationCreationFailed(userSignup *toolchainv1alpha1.UserSignup, message string) error {
	return u.updateStatusConditions(
		userSignup,
		toolchainv1alpha1.Condition{
			Type:    toolchainv1alpha1.UserSignupUserDeactivatingNotificationCreated,
			Status:  corev1.ConditionFalse,
			Reason:  toolchainv1alpha1.UserSignupDeactivatingNotificationCRCreationFailedReason,
			Message: message,
		})
}

func (u *StatusUpdater) updateStatus(logger logr.Logger, userSignup *toolchainv1alpha1.UserSignup,
	statusUpdater func(userAcc *toolchainv1alpha1.UserSignup, message string) error) error {

	if err := statusUpdater(userSignup, ""); err != nil {
		logger.Error(err, "status update failed")
		return err
	}

	return nil
}

// updateCompleteStatus updates the `CompliantUsername` and `Conditions` in the status, should only be invoked on completion because
// both completion and the compliant username require the master user record to be created.
func (u *StatusUpdater) updateCompleteStatus(logger logr.Logger, compliantUsername string) func(userSignup *toolchainv1alpha1.UserSignup, message string) error { //nolint: unparam
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

		return u.Client.Status().Update(context.TODO(), userSignup)
	}
}

// wrapErrorWithStatusUpdate wraps the error and update the UserSignup status. If the update fails then the error is logged.
func (u *StatusUpdater) wrapErrorWithStatusUpdate(logger logr.Logger, userSignup *toolchainv1alpha1.UserSignup,
	statusUpdater StatusUpdaterFunc, err error, format string, args ...interface{}) error {
	if err == nil {
		return nil
	}
	if err := statusUpdater(userSignup, err.Error()); err != nil {
		logger.Error(err, "Error updating UserSignup status")
	}
	return errs.Wrapf(err, format, args...)
}

func (u *StatusUpdater) updateStatusConditions(userSignup *toolchainv1alpha1.UserSignup, newConditions ...toolchainv1alpha1.Condition) error {
	var updated bool
	userSignup.Status.Conditions, updated = commonCondition.AddOrUpdateStatusConditions(userSignup.Status.Conditions, newConditions...)
	if !updated {
		// Nothing changed
		return nil
	}
	return u.Client.Status().Update(context.TODO(), userSignup)
}
