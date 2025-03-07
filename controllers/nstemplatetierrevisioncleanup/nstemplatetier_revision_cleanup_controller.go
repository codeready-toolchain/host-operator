package nstemplatetierrevisioncleanup

import (
	"context"
	"fmt"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/log"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/host-operator/controllers/nstemplatetier"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	ctrl "sigs.k8s.io/controller-runtime"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

const deletionTimeThreshold = 30 * time.Second

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr manager.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&toolchainv1alpha1.TierTemplateRevision{}).
		Complete(r)
}

type Reconciler struct {
	Client runtimeclient.Client
	Scheme *runtime.Scheme
}

func (r *Reconciler) Reconcile(ctx context.Context, request ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// fetch the NSTemplateTier tier
	ttr := &toolchainv1alpha1.TierTemplateRevision{}
	if err := r.Client.Get(ctx, request.NamespacedName, ttr); err != nil {
		if errors.IsNotFound(err) {
			logger.Info("TierTemplateRevision not found")
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		logger.Error(err, "unable to get the current TierTemplateRevision")
		return reconcile.Result{}, fmt.Errorf("unable to get the current TierTemplateRevision: %w", err)
	}
	tierName, ok := ttr.GetLabels()[toolchainv1alpha1.TierLabelKey]
	if !ok {
		return reconcile.Result{}, fmt.Errorf("tier-name label not found in tiertemplaterevision")

	}
	// fetch the related NSTemplateTier tier
	tier := &toolchainv1alpha1.NSTemplateTier{}
	if err := r.Client.Get(ctx, types.NamespacedName{
		Namespace: ttr.GetNamespace(),
		Name:      tierName,
	}, tier); err != nil {
		if errors.IsNotFound(err) {
			logger.Info("NSTemplateTier not found")
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		logger.Error(err, "unable to get the current NSTemplateTier")
		return reconcile.Result{}, fmt.Errorf("unable to get the current NSTemplateTier: %w", err)
	}

	// verify that the tier template revision which is unsused(not referenced by any tiers and whose creation date is older that 30secs
	// and all the spaces are up-to-date)
	UnusedRevisionBool, requeue, reqAft, err := r.VerifyUnusedTTR(ctx, tier, ttr, deletionTimeThreshold)
	if err != nil {
		return reconcile.Result{RequeueAfter: reqAft, Requeue: requeue}, fmt.Errorf("cannot delete the current TTR,requing :%w", err)
	}

	// Delete the unused revision
	if UnusedRevisionBool {
		if err := r.Client.Delete(ctx, ttr); err != nil {
			return reconcile.Result{}, fmt.Errorf("unable to delete the current Tier Template Revision %s: %w", ttr.Name, err)
		}
	}

	return reconcile.Result{}, nil
}

// VerifyUnusedTTR function verifies that the tier template revision's creation time stamp is older than 30sec,
// it is not present/refernced in status.revisions field, and there are no outdated spaces.
func (r *Reconciler) VerifyUnusedTTR(ctx context.Context, nsTmplTier *toolchainv1alpha1.NSTemplateTier,
	rev *toolchainv1alpha1.TierTemplateRevision, deletionTimeThreshold time.Duration) (bool, bool, time.Duration, error) {

	//get the tier template revision creation time stamp and the duration
	timeSinceCreation := time.Since(rev.GetCreationTimestamp().Time)

	//the ttr age should be greater than 30 seconds
	if timeSinceCreation < deletionTimeThreshold {
		requeAfter := deletionTimeThreshold - timeSinceCreation
		return false, true, requeAfter, fmt.Errorf("the revision creation timestamp is less than 30 sec(new revision)")
	}

	//check if the ttr name is present status.revisions
	for _, ttstatusrev := range nsTmplTier.Status.Revisions {
		if ttstatusrev == rev.Name {
			return false, false, 0, fmt.Errorf("the revision is still being referenced in status.revisions")
		}
	}

	// get the outdated matchig label to list outdated spaces
	matchOutdated, err := nstemplatetier.OutdatedTierSelector(nsTmplTier)
	if err != nil {
		return false, false, 0, err

	}
	// look-up all spaces associated with the NSTemplateTier which are outdated
	spaces := &toolchainv1alpha1.SpaceList{}
	if err := r.Client.List(ctx, spaces, runtimeclient.InNamespace(nsTmplTier.Namespace),
		matchOutdated, runtimeclient.Limit(1)); err != nil {
		return false, false, 0, err
	}

	//there should not be any outdated spaces
	if len(spaces.Items) > 0 {
		return false, false, 0, fmt.Errorf("there are still some spaces which are outdated")
	}

	return true, false, 0, nil

}
