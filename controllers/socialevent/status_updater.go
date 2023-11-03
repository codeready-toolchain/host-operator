package socialevent

import (
	"context"
	"fmt"

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

func (u *StatusUpdater) ready(ctx context.Context, event *toolchainv1alpha1.SocialEvent) error {
	return u.updateStatusConditions(ctx, event, toolchainv1alpha1.Condition{
		Type:   toolchainv1alpha1.ConditionReady,
		Status: corev1.ConditionTrue,
	})
}

func (u *StatusUpdater) userTierNotFound(ctx context.Context, event *toolchainv1alpha1.SocialEvent) error {
	log.FromContext(ctx).Info("UserTier not found", "nstemplatetier_name", event.Spec.UserTier)
	return u.updateStatusConditions(ctx, event, toolchainv1alpha1.Condition{
		Type:    toolchainv1alpha1.ConditionReady,
		Status:  corev1.ConditionFalse,
		Reason:  toolchainv1alpha1.SocialEventInvalidUserTierReason,
		Message: fmt.Sprintf("UserTier '%s' not found", event.Spec.UserTier),
	})
}

func (u *StatusUpdater) unableToGetUserTier(ctx context.Context, event *toolchainv1alpha1.SocialEvent, err error) error {
	log.FromContext(ctx).Error(err, "unable to get the UserTier", "usertier_name", event.Spec.UserTier)
	if err2 := u.updateStatusConditions(ctx, event, toolchainv1alpha1.Condition{
		Type:    toolchainv1alpha1.ConditionReady,
		Status:  corev1.ConditionFalse,
		Reason:  toolchainv1alpha1.SocialEventUnableToGetUserTierReason,
		Message: fmt.Sprintf("unable to get the '%s' UserTier: %s", event.Spec.UserTier, err.Error()),
	}); err2 != nil {
		// if status could not be updated, then return the associated error
		return err2
	}
	// if status was updated, then return the "main" error
	return errs.Wrapf(err, "unable to get the '%s' UserTier", event.Spec.UserTier)
}

func (u *StatusUpdater) spaceTierNotFound(ctx context.Context, event *toolchainv1alpha1.SocialEvent) error {
	log.FromContext(ctx).Info("NSTemplateTier not found", "nstemplatetier_name", event.Spec.SpaceTier)
	return u.updateStatusConditions(ctx, event, toolchainv1alpha1.Condition{
		Type:    toolchainv1alpha1.ConditionReady,
		Status:  corev1.ConditionFalse,
		Reason:  toolchainv1alpha1.SocialEventInvalidSpaceTierReason,
		Message: fmt.Sprintf("NSTemplateTier '%s' not found", event.Spec.SpaceTier),
	})
}

func (u *StatusUpdater) unableToGetSpaceTier(ctx context.Context, event *toolchainv1alpha1.SocialEvent, err error) error {
	log.FromContext(ctx).Error(err, "unable to get the NSTemplateTier", "nstemplatetier_name", event.Spec.SpaceTier)
	if err2 := u.updateStatusConditions(ctx, event, toolchainv1alpha1.Condition{
		Type:    toolchainv1alpha1.ConditionReady,
		Status:  corev1.ConditionFalse,
		Reason:  toolchainv1alpha1.SocialEventUnableToGetSpaceTierReason,
		Message: fmt.Sprintf("unable to get the '%s' NSTemplateTier: %s", event.Spec.SpaceTier, err.Error()),
	}); err2 != nil {
		// if status could not be updated, then return the associated error
		return err2
	}
	// if status was updated, then return the "main" error
	return errs.Wrapf(err, "unable to get the '%s' NSTemplateTier", event.Spec.SpaceTier)
}

func (u *StatusUpdater) updateStatusConditions(ctx context.Context, event *toolchainv1alpha1.SocialEvent, newConditions ...toolchainv1alpha1.Condition) error {
	var updated bool
	event.Status.Conditions, updated = commonCondition.AddOrUpdateStatusConditions(event.Status.Conditions, newConditions...)
	if !updated {
		// Nothing changed
		return nil
	}
	return u.Client.Status().Update(ctx, event)
}
