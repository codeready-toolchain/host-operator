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

func (u *StatusUpdater) setStatusApprovedAutomatically(userSignup *toolchainv1alpha1.UserSignup) error {
	return u.updateStatusConditions(
		userSignup,
		toolchainv1alpha1.Condition{
			Type:   toolchainv1alpha1.UserSignupApproved,
			Status: corev1.ConditionTrue,
			Reason: toolchainv1alpha1.UserSignupApprovedAutomaticallyReason,
		})
}

func (u *StatusUpdater) setStatusApprovedByAdmin(userSignup *toolchainv1alpha1.UserSignup) error {
	return u.updateStatusConditions(
		userSignup,
		toolchainv1alpha1.Condition{
			Type:   toolchainv1alpha1.UserSignupApproved,
			Status: corev1.ConditionTrue,
			Reason: toolchainv1alpha1.UserSignupApprovedByAdminReason,
		})
}

func (u *StatusUpdater) setStatusApprovedByAdminButNoClusterAvailable(logger logr.Logger, userSignup *toolchainv1alpha1.UserSignup, cause error, message string) error {
	if err := u.updateStatusConditions(
		userSignup,
		toolchainv1alpha1.Condition{
			Type:   toolchainv1alpha1.UserSignupApproved,
			Status: corev1.ConditionTrue,
			Reason: toolchainv1alpha1.UserSignupApprovedByAdminReason,
		},
		toolchainv1alpha1.Condition{
			Type:    toolchainv1alpha1.UserSignupComplete,
			Status:  corev1.ConditionFalse,
			Reason:  toolchainv1alpha1.UserSignupNoClusterAvailableReason,
			Message: message,
		}); err != nil {
		logger.Error(cause, message)
		return err
	}
	return cause
}

func (u *StatusUpdater) setStatusPendingApprovalButErrorGettingCluster(logger logr.Logger, userSignup *toolchainv1alpha1.UserSignup, cause error, message string) error {
	if err := u.updateStatusConditions(
		userSignup,
		toolchainv1alpha1.Condition{
			Type:   toolchainv1alpha1.UserSignupApproved,
			Status: corev1.ConditionFalse,
			Reason: toolchainv1alpha1.UserSignupPendingApprovalReason,
		},
		toolchainv1alpha1.Condition{
			Type:    toolchainv1alpha1.UserSignupComplete,
			Status:  corev1.ConditionFalse,
			Reason:  toolchainv1alpha1.UserSignupNoClusterAvailableReason,
			Message: message,
		}); err != nil {
		logger.Error(cause, message)
		return err
	}
	return cause
}

func (u *StatusUpdater) setStatusPendingApprovalButNoClusterAvailable(userSignup *toolchainv1alpha1.UserSignup) error {
	return u.updateStatusConditions(
		userSignup,
		toolchainv1alpha1.Condition{
			Type:   toolchainv1alpha1.UserSignupApproved,
			Status: corev1.ConditionFalse,
			Reason: toolchainv1alpha1.UserSignupPendingApprovalReason,
		},
		toolchainv1alpha1.Condition{
			Type:   toolchainv1alpha1.UserSignupComplete,
			Status: corev1.ConditionFalse,
			Reason: toolchainv1alpha1.UserSignupNoClusterAvailableReason,
		})
}

func (u *StatusUpdater) setStatusCompleteButPendingApproval(userSignup *toolchainv1alpha1.UserSignup) error {
	return u.updateStatusConditions(
		userSignup,
		toolchainv1alpha1.Condition{
			Type:   toolchainv1alpha1.UserSignupApproved,
			Status: corev1.ConditionFalse,
			Reason: toolchainv1alpha1.UserSignupPendingApprovalReason,
		},
		toolchainv1alpha1.Condition{
			Type:   toolchainv1alpha1.UserSignupComplete,
			Status: corev1.ConditionFalse,
			Reason: toolchainv1alpha1.UserSignupPendingApprovalReason,
		})
}

func (u *StatusUpdater) setStatusInvalidMURState(logger logr.Logger, userSignup *toolchainv1alpha1.UserSignup, cause error, message string) error {
	if err := u.updateStatusConditions(
		userSignup,
		toolchainv1alpha1.Condition{
			Type:    toolchainv1alpha1.UserSignupComplete,
			Status:  corev1.ConditionFalse,
			Reason:  toolchainv1alpha1.UserSignupInvalidMURStateReason,
			Message: message,
		}); err != nil {
		logger.Error(cause, message)
		return err
	}
	return cause
}

func (u *StatusUpdater) setStatusFailedToCreateMUR(logger logr.Logger, userSignup *toolchainv1alpha1.UserSignup, cause error, message string) error {
	if err := u.updateStatusConditions(
		userSignup,
		toolchainv1alpha1.Condition{
			Type:    toolchainv1alpha1.UserSignupComplete,
			Status:  corev1.ConditionFalse,
			Reason:  toolchainv1alpha1.UserSignupUnableToCreateMURReason,
			Message: message,
		}); err != nil {
		logger.Error(cause, message)
		return err
	}
	return cause
}

func (u *StatusUpdater) setStatusFailedToDeleteMUR(logger logr.Logger, userSignup *toolchainv1alpha1.UserSignup, cause error, message string) error {
	if err := u.updateStatusConditions(
		userSignup,
		toolchainv1alpha1.Condition{
			Type:    toolchainv1alpha1.UserSignupComplete,
			Status:  corev1.ConditionFalse,
			Reason:  toolchainv1alpha1.UserSignupUnableToDeleteMURReason,
			Message: message,
		}); err != nil {
		logger.Error(cause, message)
		return err
	}
	return cause
}

func (u *StatusUpdater) setStatusNoTemplateTierAvailable(logger logr.Logger, userSignup *toolchainv1alpha1.UserSignup, cause error, message string) error {
	if err := u.updateStatusConditions(
		userSignup,
		toolchainv1alpha1.Condition{
			Type:    toolchainv1alpha1.UserSignupComplete,
			Status:  corev1.ConditionFalse,
			Reason:  toolchainv1alpha1.UserSignupNoTemplateTierAvailableReason,
			Message: message,
		}); err != nil {
		logger.Error(cause, message)
		return err
	}
	return cause
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

func (u *StatusUpdater) setStatusFailedToReadBannedUsers(logger logr.Logger, userSignup *toolchainv1alpha1.UserSignup, cause error, message string) error {
	if err := u.updateStatusConditions(
		userSignup,
		toolchainv1alpha1.Condition{
			Type:    toolchainv1alpha1.UserSignupComplete,
			Status:  corev1.ConditionFalse,
			Reason:  toolchainv1alpha1.UserSignupFailedToReadBannedUsersReason,
			Message: message,
		}); err != nil {
		logger.Error(cause, message)
		return err
	}
	return cause
}

func (u *StatusUpdater) setStatusInvalidMissingUserEmailAnnotation(logger logr.Logger, userSignup *toolchainv1alpha1.UserSignup, cause error, message string) error {
	if err := u.updateStatusConditions(
		userSignup,
		toolchainv1alpha1.Condition{
			Type:    toolchainv1alpha1.UserSignupComplete,
			Status:  corev1.ConditionFalse,
			Reason:  toolchainv1alpha1.UserSignupMissingUserEmailAnnotationReason,
			Message: message,
		}); err != nil {
		logger.Error(cause, message)
		return err
	}
	return cause
}

func (u *StatusUpdater) setStatusMissingEmailHash(logger logr.Logger, userSignup *toolchainv1alpha1.UserSignup, cause error, message string) error {
	if err := u.updateStatusConditions(
		userSignup,
		toolchainv1alpha1.Condition{
			Type:    toolchainv1alpha1.UserSignupComplete,
			Status:  corev1.ConditionFalse,
			Reason:  toolchainv1alpha1.UserSignupMissingEmailHashLabelReason,
			Message: message,
		}); err != nil {
		logger.Error(cause, message)
		return err
	}
	return cause
}

func (u *StatusUpdater) setStatusInvalidEmailHash(logger logr.Logger, userSignup *toolchainv1alpha1.UserSignup, cause error, message string) error {
	if err := u.updateStatusConditions(
		userSignup,
		toolchainv1alpha1.Condition{
			Type:    toolchainv1alpha1.UserSignupComplete,
			Status:  corev1.ConditionFalse,
			Reason:  toolchainv1alpha1.UserSignupInvalidEmailHashLabelReason,
			Message: message,
		}); err != nil {
		logger.Error(cause, message)
		return err
	}
	return cause
}

func (u *StatusUpdater) setStatusVerificationRequired(userSignup *toolchainv1alpha1.UserSignup) error {
	return u.updateStatusConditions(
		userSignup,
		toolchainv1alpha1.Condition{
			Type:   toolchainv1alpha1.UserSignupComplete,
			Status: corev1.ConditionFalse,
			Reason: toolchainv1alpha1.UserSignupVerificationRequiredReason,
		})
}

func (u *StatusUpdater) setStatusFailedToUpdateStateLabel(logger logr.Logger, userSignup *toolchainv1alpha1.UserSignup, cause error, message string) error {
	if err := u.updateStatusConditions(
		userSignup,
		toolchainv1alpha1.Condition{
			Type:    toolchainv1alpha1.UserSignupComplete,
			Status:  corev1.ConditionFalse,
			Reason:  toolchainv1alpha1.UserSignupUnableToUpdateStateLabelReason,
			Message: message,
		}); err != nil {
		logger.Error(cause, message)
		return err
	}
	return cause
}

func (u *StatusUpdater) setStatusFailedToUpdateAnnotation(logger logr.Logger, userSignup *toolchainv1alpha1.UserSignup, cause error, message string) error {
	if err := u.updateStatusConditions(
		userSignup,
		toolchainv1alpha1.Condition{
			Type:    toolchainv1alpha1.UserSignupComplete,
			Status:  corev1.ConditionFalse,
			Reason:  toolchainv1alpha1.UserSignupUnableToUpdateAnnotationReason,
			Message: message,
		}); err != nil {
		logger.Error(cause, message)
		return err
	}
	return cause
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

func (u *StatusUpdater) setStatusDeactivationNotificationCreationFailed(logger logr.Logger, userSignup *toolchainv1alpha1.UserSignup, cause error, message string) error {
	if err := u.updateStatusConditions(
		userSignup,
		toolchainv1alpha1.Condition{
			Type:    toolchainv1alpha1.UserSignupUserDeactivatedNotificationCreated,
			Status:  corev1.ConditionFalse,
			Reason:  toolchainv1alpha1.UserSignupDeactivatedNotificationCRCreationFailedReason,
			Message: message,
		}); err != nil {
		logger.Error(cause, message)
		return err
	}
	return cause
}

func (u *StatusUpdater) setStatusDeactivatingNotificationCreated(userSignup *toolchainv1alpha1.UserSignup) error {
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

func (u *StatusUpdater) setStatusDeactivatingNotificationCreationFailed(logger logr.Logger, userSignup *toolchainv1alpha1.UserSignup, cause error) error {
	if err := u.updateStatusConditions(
		userSignup,
		toolchainv1alpha1.Condition{
			Type:    toolchainv1alpha1.UserSignupUserDeactivatingNotificationCreated,
			Status:  corev1.ConditionFalse,
			Reason:  toolchainv1alpha1.UserSignupDeactivatingNotificationCRCreationFailedReason,
			Message: cause.Error(),
		}); err != nil {
		logger.Error(cause, "failed to create user deactivating notification")
		return err
	}
	return cause
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
		logger.Error(err, "error updating UserSignup status")
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
