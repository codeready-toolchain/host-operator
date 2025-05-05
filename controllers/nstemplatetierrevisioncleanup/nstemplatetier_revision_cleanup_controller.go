package nstemplatetierrevisioncleanup

import (
	"context"
	"fmt"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/log"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/host-operator/controllers/nstemplatetier"
	"github.com/redhat-cop/operator-utils/pkg/util"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	ctrl "sigs.k8s.io/controller-runtime"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

const minTTRAge = 30 * time.Second

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr manager.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&toolchainv1alpha1.TierTemplateRevision{}).
		Complete(r)
}

type Reconciler struct {
	Client runtimeclient.Client
}

func (r *Reconciler) Reconcile(ctx context.Context, request ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// fetch the TTR
	ttr := &toolchainv1alpha1.TierTemplateRevision{}
	if err := r.Client.Get(ctx, request.NamespacedName, ttr); err != nil {
		if errors.IsNotFound(err) {
			logger.Info("TierTemplateRevision not found")
			return reconcile.Result{}, nil
		}

		// Error reading the object - requeue the request.
		return reconcile.Result{}, fmt.Errorf("unable to get the current TierTemplateRevision: %w", err)
	}

	// if the TTR has deletion time stamp, return, no need to do anything as its already marked for deleteion
	if util.IsBeingDeleted(ttr) {
		logger.Info("TierTemplateRevision already marked for deletion")
		return reconcile.Result{}, nil
	}
	//there is no point in fetching  the NStemplateTier, if the TTR  is just created,
	// as it is a new TTR created due to changes in NSTemplate Tier,
	// and the references are still being updated to nstemplatetier
	//get the tier template revision creation time stamp and the duration
	timeSinceCreation := time.Since(ttr.GetCreationTimestamp().Time)

	//the ttr age should be greater than 30 seconds
	if timeSinceCreation < minTTRAge {
		requeueAfter := minTTRAge - timeSinceCreation
		logger.Info("the TierTemplateRevision is not old enough", "requeue-after", requeueAfter)
		return reconcile.Result{RequeueAfter: requeueAfter}, nil
	}

	//check if there is tier-name label available
	tierName, ok := ttr.GetLabels()[toolchainv1alpha1.TierLabelKey]
	if !ok {
		//if tier-name label is not found we should delete the TTR
		return reconcile.Result{}, r.deleteTTR(ctx, ttr)
	}
	// fetch the related NSTemplateTier tier
	tier := &toolchainv1alpha1.NSTemplateTier{}
	if err := r.Client.Get(ctx, types.NamespacedName{
		Namespace: ttr.GetNamespace(),
		Name:      tierName,
	}, tier); err != nil {
		if errors.IsNotFound(err) {
			// if there is no related nstemplate tier, we can delete the TTR directly
			return reconcile.Result{}, r.deleteTTR(ctx, ttr)
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, fmt.Errorf("unable to get the current NSTemplateTier: %w", err)
	}

	// let's make sure that we delete the TTR only if it's not used
	isTTrUnused, err := r.verifyUnusedTTR(ctx, tier, ttr)
	if err != nil {
		return reconcile.Result{}, err
	}

	// Delete the unused revision
	if isTTrUnused {
		return reconcile.Result{}, r.deleteTTR(ctx, ttr)
	}

	return reconcile.Result{}, nil
}

// verifyUnusedTTR function verifies that the TTR is not used (returns true if it's NOT used).
// this is done by:
//   - checking the NSTemplateTier.status.revisions field, if the TTR is referenced there or not
//   - checking if all Spaces are up-to-date. In case there are some outdated space, we could risk that the TTR is still being used
func (r *Reconciler) verifyUnusedTTR(ctx context.Context, nsTmplTier *toolchainv1alpha1.NSTemplateTier,
	rev *toolchainv1alpha1.TierTemplateRevision) (bool, error) {

	logger := log.FromContext(ctx)
	//check if the ttr name is present status.revisions
	for _, ttStatusRev := range nsTmplTier.Status.Revisions {
		if ttStatusRev == rev.Name {
			logger.Info("the TierTemplateRevision is still being referenced in NSTemplateTier.Status.Revisions")
			return false, nil
		}
	}

	// get the outdated matching label to list outdated spaces
	matchOutdated, err := nstemplatetier.OutdatedTierSelector(nsTmplTier)
	if err != nil {
		return false, err

	}
	// look-up all spaces associated with the NSTemplateTier which are outdated
	spaces := &toolchainv1alpha1.SpaceList{}
	if err := r.Client.List(ctx, spaces, runtimeclient.InNamespace(nsTmplTier.Namespace),
		matchOutdated, runtimeclient.Limit(1)); err != nil {
		return false, err
	}

	//If there has been an update on nstemplatetier, it might be in a process to update all the spaces.
	// so we need to check that there should be no outdated spaces.
	if len(spaces.Items) > 0 {
		logger.Info("there are still some spaces which are outdated")
		return false, nil
	}
	return true, nil
}

// deleteTTR function delete the TTR
func (r *Reconciler) deleteTTR(ctx context.Context, ttr *toolchainv1alpha1.TierTemplateRevision) error {
	if err := r.Client.Delete(ctx, ttr); err != nil {
		if errors.IsNotFound(err) {
			return nil // was already deleted
		}
		return fmt.Errorf("unable to delete the current Tier Template Revision %s: %w", ttr.Name, err)
	}
	log.FromContext(ctx).Info("The TierTemplateRevision has been deleted")
	return nil
}
