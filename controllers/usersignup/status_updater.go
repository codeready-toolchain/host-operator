package usersignup

import (
	"context"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	commonCondition "github.com/codeready-toolchain/toolchain-common/pkg/condition"

	errs "github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

type StatusUpdater struct {
	Client runtimeclient.Client
}

func (u *StatusUpdater) setStatusApprovedAutomatically(ctx context.Context, userSignup *toolchainv1alpha1.UserSignup, message string) error {
	return u.updateStatusConditions(
		ctx,
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

func (u *StatusUpdater) setStatusInvalidMURState(ctx context.Context, userSignup *toolchainv1alpha1.UserSignup, message string) error {
	return u.updateStatusConditions(
		ctx,
		userSignup,
		toolchainv1alpha1.Condition{
			Type:    toolchainv1alpha1.UserSignupComplete,
			Status:  corev1.ConditionFalse,
			Reason:  toolchainv1alpha1.UserSignupInvalidMURStateReason,
			Message: message,
		})
}

func (u *StatusUpdater) setStatusFailedToCreateMUR(ctx context.Context, userSignup *toolchainv1alpha1.UserSignup, message string) error {
	return u.updateStatusConditions(
		ctx,
		userSignup,
		toolchainv1alpha1.Condition{
			Type:    toolchainv1alpha1.UserSignupComplete,
			Status:  corev1.ConditionFalse,
			Reason:  toolchainv1alpha1.UserSignupUnableToCreateMURReason,
			Message: message,
		})
}

func (u *StatusUpdater) setStatusFailedToDeleteMUR(ctx context.Context, userSignup *toolchainv1alpha1.UserSignup, message string) error {
	return u.updateStatusConditions(
		ctx,
		userSignup,
		toolchainv1alpha1.Condition{
			Type:    toolchainv1alpha1.UserSignupComplete,
			Status:  corev1.ConditionFalse,
			Reason:  toolchainv1alpha1.UserSignupUnableToDeleteMURReason,
			Message: message,
		})
}

func (u *StatusUpdater) setStatusFailedToCreateSpace(ctx context.Context, userSignup *toolchainv1alpha1.UserSignup, message string) error {
	return u.updateStatusConditions(
		ctx,
		userSignup,
		toolchainv1alpha1.Condition{
			Type:    toolchainv1alpha1.UserSignupComplete,
			Status:  corev1.ConditionFalse,
			Reason:  toolchainv1alpha1.UserSignupUnableToCreateSpaceReason,
			Message: message,
		})
}

func (u *StatusUpdater) setStatusFailedToCreateSpaceBinding(ctx context.Context, userSignup *toolchainv1alpha1.UserSignup, message string) error {
	return u.updateStatusConditions(
		ctx,
		userSignup,
		toolchainv1alpha1.Condition{
			Type:    toolchainv1alpha1.UserSignupComplete,
			Status:  corev1.ConditionFalse,
			Reason:  toolchainv1alpha1.UserSignupUnableToCreateSpaceBindingReason,
			Message: message,
		})
}

func (u *StatusUpdater) set(conditionCreators ...func(message string) toolchainv1alpha1.Condition) func(context.Context, *toolchainv1alpha1.UserSignup, string) error {
	return func(ctx context.Context, userSignup *toolchainv1alpha1.UserSignup, message string) error {
		conditions := make([]toolchainv1alpha1.Condition, len(conditionCreators))
		for index, createCondition := range conditionCreators {
			conditions[index] = createCondition(message)
		}
		return u.updateStatusConditions(ctx, userSignup, conditions...)
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

func (u *StatusUpdater) setStatusNoUserTierAvailable(ctx context.Context, userSignup *toolchainv1alpha1.UserSignup, message string) error {
	return u.updateStatusConditions(
		ctx,
		userSignup,
		toolchainv1alpha1.Condition{
			Type:    toolchainv1alpha1.UserSignupComplete,
			Status:  corev1.ConditionFalse,
			Reason:  toolchainv1alpha1.UserSignupNoUserTierAvailableReason,
			Message: message,
		})
}

func (u *StatusUpdater) setStatusNoTemplateTierAvailable(ctx context.Context, userSignup *toolchainv1alpha1.UserSignup, message string) error {
	return u.updateStatusConditions(
		ctx,
		userSignup,
		toolchainv1alpha1.Condition{
			Type:    toolchainv1alpha1.UserSignupComplete,
			Status:  corev1.ConditionFalse,
			Reason:  toolchainv1alpha1.UserSignupNoTemplateTierAvailableReason,
			Message: message,
		})
}

func (u *StatusUpdater) setStatusBanning(ctx context.Context, userSignup *toolchainv1alpha1.UserSignup, message string) error {
	return u.updateStatusConditions(
		ctx,
		userSignup,
		toolchainv1alpha1.Condition{
			Type:    toolchainv1alpha1.UserSignupComplete,
			Status:  corev1.ConditionFalse,
			Reason:  toolchainv1alpha1.UserSignupUserBanningReason,
			Message: message,
		})
}

// setStatusBanned sets the Complete status to True, as the banning operation has been successful (with a reason of "Banned")
func (u *StatusUpdater) setStatusBanned(ctx context.Context, userSignup *toolchainv1alpha1.UserSignup, message string) error {
	return u.updateStatusConditions(
		ctx,
		userSignup,
		toolchainv1alpha1.Condition{
			Type:    toolchainv1alpha1.UserSignupComplete,
			Status:  corev1.ConditionTrue,
			Reason:  toolchainv1alpha1.UserSignupUserBannedReason,
			Message: message,
		})
}

func (u *StatusUpdater) setStatusDeactivationInProgress(ctx context.Context, userSignup *toolchainv1alpha1.UserSignup, message string) error {
	return u.updateStatusConditions(
		ctx,
		userSignup,
		toolchainv1alpha1.Condition{
			Type:    toolchainv1alpha1.UserSignupComplete,
			Status:  corev1.ConditionFalse,
			Reason:  toolchainv1alpha1.UserSignupDeactivationInProgressReason,
			Message: message,
		})
}

func (u *StatusUpdater) setStatusDeactivated(ctx context.Context, userSignup *toolchainv1alpha1.UserSignup, message string) error {
	return u.updateStatusConditions(
		ctx,
		userSignup,
		toolchainv1alpha1.Condition{
			Type:    toolchainv1alpha1.UserSignupComplete,
			Status:  corev1.ConditionTrue,
			Reason:  toolchainv1alpha1.UserSignupUserDeactivatedReason,
			Message: message,
		})
}

func (u *StatusUpdater) setStatusFailedToReadBannedUsers(ctx context.Context, userSignup *toolchainv1alpha1.UserSignup, message string) error {
	return u.updateStatusConditions(
		ctx,
		userSignup,
		toolchainv1alpha1.Condition{
			Type:    toolchainv1alpha1.UserSignupComplete,
			Status:  corev1.ConditionFalse,
			Reason:  toolchainv1alpha1.UserSignupFailedToReadBannedUsersReason,
			Message: message,
		})
}

func (u *StatusUpdater) setStatusInvalidMissingUserEmailAnnotation(ctx context.Context, userSignup *toolchainv1alpha1.UserSignup, message string) error {
	return u.updateStatusConditions(
		ctx,
		userSignup,
		toolchainv1alpha1.Condition{
			Type:    toolchainv1alpha1.UserSignupComplete,
			Status:  corev1.ConditionFalse,
			Reason:  toolchainv1alpha1.UserSignupMissingUserEmailAnnotationReason,
			Message: message,
		})
}

func (u *StatusUpdater) setStatusMissingEmailHash(ctx context.Context, userSignup *toolchainv1alpha1.UserSignup, message string) error {
	return u.updateStatusConditions(
		ctx,
		userSignup,
		toolchainv1alpha1.Condition{
			Type:    toolchainv1alpha1.UserSignupComplete,
			Status:  corev1.ConditionFalse,
			Reason:  toolchainv1alpha1.UserSignupMissingEmailHashLabelReason,
			Message: message,
		})
}

func (u *StatusUpdater) setStatusInvalidEmailHash(ctx context.Context, userSignup *toolchainv1alpha1.UserSignup, message string) error {
	return u.updateStatusConditions(
		ctx,
		userSignup,
		toolchainv1alpha1.Condition{
			Type:    toolchainv1alpha1.UserSignupComplete,
			Status:  corev1.ConditionFalse,
			Reason:  toolchainv1alpha1.UserSignupInvalidEmailHashLabelReason,
			Message: message,
		})
}

func (u *StatusUpdater) setStatusVerificationRequired(ctx context.Context, userSignup *toolchainv1alpha1.UserSignup, message string) error {
	return u.updateStatusConditions(
		ctx,
		userSignup,
		toolchainv1alpha1.Condition{
			Type:    toolchainv1alpha1.UserSignupComplete,
			Status:  corev1.ConditionFalse,
			Reason:  toolchainv1alpha1.UserSignupVerificationRequiredReason,
			Message: message,
		})
}

func (u *StatusUpdater) setStatusFailedToUpdateStateLabel(ctx context.Context, userSignup *toolchainv1alpha1.UserSignup, message string) error {
	return u.updateStatusConditions(
		ctx,
		userSignup,
		toolchainv1alpha1.Condition{
			Type:    toolchainv1alpha1.UserSignupComplete,
			Status:  corev1.ConditionFalse,
			Reason:  toolchainv1alpha1.UserSignupUnableToUpdateStateLabelReason,
			Message: message,
		})
}

func (u *StatusUpdater) setStatusFailedToUpdateAnnotation(ctx context.Context, userSignup *toolchainv1alpha1.UserSignup, message string) error {
	return u.updateStatusConditions(
		ctx,
		userSignup,
		toolchainv1alpha1.Condition{
			Type:    toolchainv1alpha1.UserSignupComplete,
			Status:  corev1.ConditionFalse,
			Reason:  toolchainv1alpha1.UserSignupUnableToUpdateAnnotationReason,
			Message: message,
		})
}

func (u *StatusUpdater) setStatusDeactivationNotificationCreated(ctx context.Context, userSignup *toolchainv1alpha1.UserSignup, _ string) error {
	return u.updateStatusConditions(
		ctx,
		userSignup,
		toolchainv1alpha1.Condition{
			Type:   toolchainv1alpha1.UserSignupUserDeactivatedNotificationCreated,
			Status: corev1.ConditionTrue,
			Reason: toolchainv1alpha1.UserSignupDeactivatedNotificationCRCreatedReason,
		})
}

func (u *StatusUpdater) setStatusDeactivationNotificationUserIsActive(ctx context.Context, userSignup *toolchainv1alpha1.UserSignup, _ string) error {
	return u.updateStatusConditions(
		ctx,
		userSignup,
		toolchainv1alpha1.Condition{
			Type:   toolchainv1alpha1.UserSignupUserDeactivatedNotificationCreated,
			Status: corev1.ConditionFalse,
			Reason: toolchainv1alpha1.UserSignupDeactivatedNotificationUserIsActiveReason,
		})
}

func (u *StatusUpdater) setStatusDeactivationNotificationCreationFailed(ctx context.Context, userSignup *toolchainv1alpha1.UserSignup, message string) error {
	return u.updateStatusConditions(
		ctx,
		userSignup,
		toolchainv1alpha1.Condition{
			Type:    toolchainv1alpha1.UserSignupUserDeactivatedNotificationCreated,
			Status:  corev1.ConditionFalse,
			Reason:  toolchainv1alpha1.UserSignupDeactivatedNotificationCRCreationFailedReason,
			Message: message,
		})
}

func (u *StatusUpdater) setStatusDeactivatingNotificationCreated(ctx context.Context, userSignup *toolchainv1alpha1.UserSignup, _ string) error {
	return u.updateStatusConditions(
		ctx,
		userSignup,
		toolchainv1alpha1.Condition{
			Type:   toolchainv1alpha1.UserSignupUserDeactivatingNotificationCreated,
			Status: corev1.ConditionTrue,
			Reason: toolchainv1alpha1.UserSignupDeactivatingNotificationCRCreatedReason,
		})
}

func (u *StatusUpdater) setStatusDeactivatingNotificationNotInPreDeactivation(ctx context.Context, userSignup *toolchainv1alpha1.UserSignup, _ string) error {
	return u.updateStatusConditions(
		ctx,
		userSignup,
		toolchainv1alpha1.Condition{
			Type:   toolchainv1alpha1.UserSignupUserDeactivatingNotificationCreated,
			Status: corev1.ConditionFalse,
			Reason: toolchainv1alpha1.UserSignupDeactivatingNotificationUserNotInPreDeactivationReason,
		})
}

func (u *StatusUpdater) setStatusDeactivatingNotificationCreationFailed(ctx context.Context, userSignup *toolchainv1alpha1.UserSignup, message string) error {
	return u.updateStatusConditions(
		ctx,
		userSignup,
		toolchainv1alpha1.Condition{
			Type:    toolchainv1alpha1.UserSignupUserDeactivatingNotificationCreated,
			Status:  corev1.ConditionFalse,
			Reason:  toolchainv1alpha1.UserSignupDeactivatingNotificationCRCreationFailedReason,
			Message: message,
		})
}

func (u *StatusUpdater) updateStatus(ctx context.Context, userSignup *toolchainv1alpha1.UserSignup, statusUpdater StatusUpdaterFunc) error {
	if err := statusUpdater(ctx, userSignup, ""); err != nil {
		logger := log.FromContext(ctx)
		logger.Error(err, "status update failed")
		return err
	}

	return nil
}

func (u *StatusUpdater) updateIncompleteStatus(ctx context.Context, userSignup *toolchainv1alpha1.UserSignup, message string) error {
	return u.updateStatusConditions(
		ctx,
		userSignup,
		toolchainv1alpha1.Condition{
			Type:    toolchainv1alpha1.UserSignupComplete,
			Status:  corev1.ConditionFalse,
			Reason:  toolchainv1alpha1.UserSignupProvisioningSpaceReason,
			Message: message,
		})
}

// updateCompleteStatus updates the `CompliantUsername` and `Conditions` in the status, should only be invoked on completion because
// both completion and the compliant username require the master user record to be created.
func (u *StatusUpdater) updateCompleteStatus(compliantUsername string) func(context.Context, *toolchainv1alpha1.UserSignup, string) error {
	return func(ctx context.Context, userSignup *toolchainv1alpha1.UserSignup, message string) error {

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

		return u.Client.Status().Update(ctx, userSignup)
	}
}

// wrapErrorWithStatusUpdate wraps the error and update the UserSignup status. If the update fails then the error is logged.
func (u *StatusUpdater) wrapErrorWithStatusUpdate(
	ctx context.Context,
	userSignup *toolchainv1alpha1.UserSignup,
	statusUpdater StatusUpdaterFunc,
	err error,
	format string,
	args ...interface{},
) error {
	if err == nil {
		return nil
	}
	if err := statusUpdater(ctx, userSignup, err.Error()); err != nil {
		logger := log.FromContext(ctx)
		logger.Error(err, "Error updating UserSignup status")
	}
	return errs.Wrapf(err, format, args...)
}

func (u *StatusUpdater) updateStatusConditions(ctx context.Context, userSignup *toolchainv1alpha1.UserSignup, newConditions ...toolchainv1alpha1.Condition) error {
	var updated bool
	userSignup.Status.Conditions, updated = commonCondition.AddOrUpdateStatusConditions(userSignup.Status.Conditions, newConditions...)
	if !updated {
		// Nothing changed
		return nil
	}
	return u.Client.Status().Update(ctx, userSignup)
}
