package usersignup

import (
	"context"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	commonCondition "github.com/codeready-toolchain/toolchain-common/pkg/condition"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type StatusUpdater struct {
	Client client.Client
}

func (u *StatusUpdater) updateStatus(userSignup *toolchainv1alpha1.UserSignup, newConditions ...toolchainv1alpha1.Condition) error {
	userSignup.Status.Conditions, _ = commonCondition.AddOrUpdateStatusConditions(userSignup.Status.Conditions, newConditions...)
	return u.Client.Status().Update(context.TODO(), userSignup)
}

func approvedAutomatically() toolchainv1alpha1.Condition {
	return toolchainv1alpha1.Condition{
		Type:   toolchainv1alpha1.UserSignupApproved,
		Status: corev1.ConditionTrue,
		Reason: toolchainv1alpha1.UserSignupApprovedAutomaticallyReason,
	}
}

func approvedByAdmin() toolchainv1alpha1.Condition {
	return toolchainv1alpha1.Condition{
		Type:   toolchainv1alpha1.UserSignupApproved,
		Status: corev1.ConditionTrue,
		Reason: toolchainv1alpha1.UserSignupApprovedByAdminReason,
	}
}

func unapprovedPendingApproval() toolchainv1alpha1.Condition {
	return toolchainv1alpha1.Condition{
		Type:   toolchainv1alpha1.UserSignupApproved,
		Status: corev1.ConditionFalse,
		Reason: toolchainv1alpha1.UserSignupPendingApprovalReason,
	}
}

func incompletePendingApproval() toolchainv1alpha1.Condition {
	return toolchainv1alpha1.Condition{
		Type:   toolchainv1alpha1.UserSignupComplete,
		Status: corev1.ConditionFalse,
		Reason: toolchainv1alpha1.UserSignupPendingApprovalReason,
	}
}

func invalidMURState(message string) toolchainv1alpha1.Condition {
	return toolchainv1alpha1.Condition{
		Type:    toolchainv1alpha1.UserSignupComplete,
		Status:  corev1.ConditionFalse,
		Reason:  toolchainv1alpha1.UserSignupInvalidMURStateReason,
		Message: message,
	}
}

func failedToCreateMUR(message string) toolchainv1alpha1.Condition {
	return toolchainv1alpha1.Condition{
		Type:    toolchainv1alpha1.UserSignupComplete,
		Status:  corev1.ConditionFalse,
		Reason:  toolchainv1alpha1.UserSignupUnableToCreateMURReason,
		Message: message,
	}
}

func failedToDeleteMUR(message string) toolchainv1alpha1.Condition {
	return toolchainv1alpha1.Condition{
		Type:    toolchainv1alpha1.UserSignupComplete,
		Status:  corev1.ConditionFalse,
		Reason:  toolchainv1alpha1.UserSignupUnableToDeleteMURReason,
		Message: message,
	}
}

func failedToCreateSpace(message string) toolchainv1alpha1.Condition {
	return toolchainv1alpha1.Condition{
		Type:    toolchainv1alpha1.UserSignupComplete,
		Status:  corev1.ConditionFalse,
		Reason:  toolchainv1alpha1.UserSignupUnableToCreateSpaceReason,
		Message: message,
	}
}

func failedToCreateSpaceBinding(message string) toolchainv1alpha1.Condition {
	return toolchainv1alpha1.Condition{
		Type:    toolchainv1alpha1.UserSignupComplete,
		Status:  corev1.ConditionFalse,
		Reason:  toolchainv1alpha1.UserSignupUnableToCreateSpaceBindingReason,
		Message: message,
	}
}

func noClustersAvailable(message string) toolchainv1alpha1.Condition {
	return toolchainv1alpha1.Condition{
		Type:    toolchainv1alpha1.UserSignupComplete,
		Status:  corev1.ConditionFalse,
		Reason:  toolchainv1alpha1.UserSignupNoClusterAvailableReason,
		Message: message,
	}
}

func noUserTierAvailable(message string) toolchainv1alpha1.Condition {
	return toolchainv1alpha1.Condition{
		Type:    toolchainv1alpha1.UserSignupComplete,
		Status:  corev1.ConditionFalse,
		Reason:  toolchainv1alpha1.UserSignupNoUserTierAvailableReason,
		Message: message,
	}
}

func noTemplateTierAvailable(message string) toolchainv1alpha1.Condition {
	return toolchainv1alpha1.Condition{
		Type:    toolchainv1alpha1.UserSignupComplete,
		Status:  corev1.ConditionFalse,
		Reason:  toolchainv1alpha1.UserSignupNoTemplateTierAvailableReason,
		Message: message,
	}
}

func userBanning() toolchainv1alpha1.Condition {
	return toolchainv1alpha1.Condition{
		Type:   toolchainv1alpha1.UserSignupComplete,
		Status: corev1.ConditionFalse,
		Reason: toolchainv1alpha1.UserSignupUserBanningReason,
	}
}

// userBanned sets the Complete status to True, as the banning operation has been successful (with a reason of "Banned")
func userBanned() toolchainv1alpha1.Condition {
	return toolchainv1alpha1.Condition{
		Type:   toolchainv1alpha1.UserSignupComplete,
		Status: corev1.ConditionTrue,
		Reason: toolchainv1alpha1.UserSignupUserBannedReason,
	}
}

func deactivationInProgress() toolchainv1alpha1.Condition {
	return toolchainv1alpha1.Condition{
		Type:   toolchainv1alpha1.UserSignupComplete,
		Status: corev1.ConditionFalse,
		Reason: toolchainv1alpha1.UserSignupDeactivationInProgressReason,
	}
}

func userDeactivated() toolchainv1alpha1.Condition {
	return toolchainv1alpha1.Condition{
		Type:   toolchainv1alpha1.UserSignupComplete,
		Status: corev1.ConditionTrue,
		Reason: toolchainv1alpha1.UserSignupUserDeactivatedReason,
	}
}

func failedToReadBannedUsers(message string) toolchainv1alpha1.Condition {
	return toolchainv1alpha1.Condition{
		Type:    toolchainv1alpha1.UserSignupComplete,
		Status:  corev1.ConditionFalse,
		Reason:  toolchainv1alpha1.UserSignupFailedToReadBannedUsersReason,
		Message: message,
	}
}

func missingUserEmailAnnotation(message string) toolchainv1alpha1.Condition {
	return toolchainv1alpha1.Condition{
		Type:    toolchainv1alpha1.UserSignupComplete,
		Status:  corev1.ConditionFalse,
		Reason:  toolchainv1alpha1.UserSignupMissingUserEmailAnnotationReason,
		Message: message,
	}
}

func missingEmailHash(message string) toolchainv1alpha1.Condition {
	return toolchainv1alpha1.Condition{
		Type:    toolchainv1alpha1.UserSignupComplete,
		Status:  corev1.ConditionFalse,
		Reason:  toolchainv1alpha1.UserSignupMissingEmailHashLabelReason,
		Message: message,
	}
}

func invalidEmailHash(message string) toolchainv1alpha1.Condition {
	return toolchainv1alpha1.Condition{
		Type:    toolchainv1alpha1.UserSignupComplete,
		Status:  corev1.ConditionFalse,
		Reason:  toolchainv1alpha1.UserSignupInvalidEmailHashLabelReason,
		Message: message,
	}
}

func verificationRequired() toolchainv1alpha1.Condition {
	return toolchainv1alpha1.Condition{
		Type:   toolchainv1alpha1.UserSignupComplete,
		Status: corev1.ConditionFalse,
		Reason: toolchainv1alpha1.UserSignupVerificationRequiredReason,
	}
}

func failedToUpdateStateLabel(message string) toolchainv1alpha1.Condition {
	return toolchainv1alpha1.Condition{
		Type:    toolchainv1alpha1.UserSignupComplete,
		Status:  corev1.ConditionFalse,
		Reason:  toolchainv1alpha1.UserSignupUnableToUpdateStateLabelReason,
		Message: message,
	}
}

func failedToUpdateAnnotation(message string) toolchainv1alpha1.Condition {
	return toolchainv1alpha1.Condition{
		Type:    toolchainv1alpha1.UserSignupComplete,
		Status:  corev1.ConditionFalse,
		Reason:  toolchainv1alpha1.UserSignupUnableToUpdateAnnotationReason,
		Message: message,
	}
}

func deactivatedNotificationCreated() toolchainv1alpha1.Condition {
	return toolchainv1alpha1.Condition{
		Type:   toolchainv1alpha1.UserSignupUserDeactivatedNotificationCreated,
		Status: corev1.ConditionTrue,
		Reason: toolchainv1alpha1.UserSignupDeactivatedNotificationCRCreatedReason,
	}
}

func deactivatedNotificationUserIsActive() toolchainv1alpha1.Condition {
	return toolchainv1alpha1.Condition{
		Type:   toolchainv1alpha1.UserSignupUserDeactivatedNotificationCreated,
		Status: corev1.ConditionFalse,
		Reason: toolchainv1alpha1.UserSignupDeactivatedNotificationUserIsActiveReason,
	}
}

func deactivatedNotificationCreationFailed(message string) toolchainv1alpha1.Condition {
	return toolchainv1alpha1.Condition{
		Type:    toolchainv1alpha1.UserSignupUserDeactivatedNotificationCreated,
		Status:  corev1.ConditionFalse,
		Reason:  toolchainv1alpha1.UserSignupDeactivatedNotificationCRCreationFailedReason,
		Message: message,
	}
}

func deactivatingNotificationCreated() toolchainv1alpha1.Condition {
	return toolchainv1alpha1.Condition{
		Type:   toolchainv1alpha1.UserSignupUserDeactivatingNotificationCreated,
		Status: corev1.ConditionTrue,
		Reason: toolchainv1alpha1.UserSignupDeactivatingNotificationCRCreatedReason,
	}
}

func deactivatingNotificationNotInPreDeactivation() toolchainv1alpha1.Condition {
	return toolchainv1alpha1.Condition{
		Type:   toolchainv1alpha1.UserSignupUserDeactivatingNotificationCreated,
		Status: corev1.ConditionFalse,
		Reason: toolchainv1alpha1.UserSignupDeactivatingNotificationUserNotInPreDeactivationReason,
	}
}

func deactivatingNotificationCreationFailed(message string) toolchainv1alpha1.Condition {
	return toolchainv1alpha1.Condition{
		Type:    toolchainv1alpha1.UserSignupUserDeactivatingNotificationCreated,
		Status:  corev1.ConditionFalse,
		Reason:  toolchainv1alpha1.UserSignupDeactivatingNotificationCRCreationFailedReason,
		Message: message,
	}
}

func provisioningSpace(message string) toolchainv1alpha1.Condition {
	return toolchainv1alpha1.Condition{
		Type:    toolchainv1alpha1.UserSignupComplete,
		Status:  corev1.ConditionFalse,
		Reason:  toolchainv1alpha1.UserSignupProvisioningSpaceReason,
		Message: message,
	}
}

func complete() toolchainv1alpha1.Condition {
	return toolchainv1alpha1.Condition{
		Type:   toolchainv1alpha1.UserSignupComplete,
		Status: corev1.ConditionTrue,
	}
}
