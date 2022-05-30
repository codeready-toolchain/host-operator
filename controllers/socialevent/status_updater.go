package socialevent

import (
	"context"
	"fmt"

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

func (u *StatusUpdater) ready(event *toolchainv1alpha1.SocialEvent) error {
	return u.updateStatusConditions(event, toolchainv1alpha1.Condition{
		Type:   toolchainv1alpha1.ConditionReady,
		Status: corev1.ConditionTrue,
	})
}

func (u *StatusUpdater) tierNotFound(logger logr.Logger, event *toolchainv1alpha1.SocialEvent) error {
	logger.Info("NSTemplateTier not found", "nstemplatetier_name", event.Spec.Tier)
	return u.updateStatusConditions(event, toolchainv1alpha1.Condition{
		Type:    toolchainv1alpha1.ConditionReady,
		Status:  corev1.ConditionFalse,
		Reason:  toolchainv1alpha1.SocialEventInvalidTierReason,
		Message: fmt.Sprintf("NSTemplateTier '%s' not found", event.Spec.Tier),
	})
}

func (u *StatusUpdater) unableToGetTier(logger logr.Logger, event *toolchainv1alpha1.SocialEvent, err error) error {
	logger.Error(err, "unable to get the NSTemplateTier", "nstemplatetier_name", event.Spec.Tier)
	if err2 := u.updateStatusConditions(event, toolchainv1alpha1.Condition{
		Type:    toolchainv1alpha1.ConditionReady,
		Status:  corev1.ConditionFalse,
		Reason:  toolchainv1alpha1.SocialEventUnableToGetTierReason,
		Message: fmt.Sprintf("unable to get the '%s' NSTemplateTier: %s", event.Spec.Tier, err.Error()),
	}); err2 != nil {
		// if status could not be updated, then return the associated error
		return err2
	}
	// if status was updated, then return the "main" error
	return errs.Wrapf(err, "unable to get the '%s' NSTemplateTier", event.Spec.Tier)
}

func (u *StatusUpdater) updateStatusConditions(event *toolchainv1alpha1.SocialEvent, newConditions ...toolchainv1alpha1.Condition) error {
	var updated bool
	event.Status.Conditions, updated = commonCondition.AddOrUpdateStatusConditions(event.Status.Conditions, newConditions...)
	if !updated {
		// Nothing changed
		return nil
	}
	return u.Client.Status().Update(context.TODO(), event)
}
