package nstemplatetierrevisioncleanup

import (
	"context"
	"fmt"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/log"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	ctrl "sigs.k8s.io/controller-runtime"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

var AgeOld = 24 * time.Hour

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr manager.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&toolchainv1alpha1.NSTemplateTier{}, builder.WithPredicates(predicate.GenerationChangedPredicate{})).
		Complete(r)
}

type Reconciler struct {
	Client runtimeclient.Client
	Scheme *runtime.Scheme
}

func (r *Reconciler) Reconcile(ctx context.Context, request ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// fetch the NSTemplateTier tier
	tier := &toolchainv1alpha1.NSTemplateTier{}
	if err := r.Client.Get(ctx, request.NamespacedName, tier); err != nil {
		if errors.IsNotFound(err) {
			logger.Info("NSTemplateTier not found")
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		logger.Error(err, "unable to get the current NSTemplateTier")
		return reconcile.Result{}, fmt.Errorf("unable to get the current NSTemplateTier: %w", err)
	}

	// Get the list of tier template revisions which are unsused(not referenced by any tiers and whose creation date is older that 24hours(1 day))
	ttrevisions, err := r.ListUnusedTTRs(ctx, tier, AgeOld)
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("unable to get the current unused Tier Template Revisions List: %w", err)
	}

	// Delete the unused revisions
	if err := r.DeleteRevision(ctx, ttrevisions); err != nil {
		return reconcile.Result{}, fmt.Errorf("unable to delete the current Tier Template Revision: %w", err)
	}
	return reconcile.Result{}, nil
}

// ListTTRS function first lists all the tier template revisions, then compare it with the nsTemplatetierRevisions map list, and gets the revisions
// which are no longer referenced/used by any space and whose creation time stamp is older than 1 day(24hours) and returns this list of unused revisions
func (r *Reconciler) ListUnusedTTRs(ctx context.Context, nsTmplTier *toolchainv1alpha1.NSTemplateTier, ageOld time.Duration) ([]toolchainv1alpha1.TierTemplateRevision, error) {

	ttrDelList := []toolchainv1alpha1.TierTemplateRevision{}
	ttr := toolchainv1alpha1.TierTemplateRevision{}
	ttRevList := toolchainv1alpha1.TierTemplateRevisionList{}
	unUsedRevisions := make(map[string]bool)

	//list all the template revisions in a given NStemplate tier
	if err := r.Client.List(ctx, &ttRevList, runtimeclient.InNamespace(nsTmplTier.GetNamespace()),
		runtimeclient.MatchingLabels{toolchainv1alpha1.TierLabelKey: nsTmplTier.GetName()}); err != nil {
		return nil, err
	}

	//finding the tier revisions which are not refernced/used by the nstemplatetier.status.revisions
	for _, ttrs := range ttRevList.Items {
		unUsedRevisions[ttrs.Name] = true
		for _, revCR := range nsTmplTier.Status.Revisions {
			if ttrs.Name == revCR {
				unUsedRevisions[ttrs.Name] = false
			}
		}
	}

	//list the not refernced/unused tier revisions whose creation time stamp is older than 1 day.
	for _, ttrs := range ttRevList.Items {
		for ttrname, value := range unUsedRevisions {
			if ttrs.Name == ttrname && value {
				ttCreateTime, err := time.Parse(time.RFC3339, ttrs.CreationTimestamp.String())
				if err != nil {
					return nil, err
				}
				duration := -time.Until(ttCreateTime)
				if duration > ageOld {
					ttrDelList = append(ttrDelList, ttr)
				}
			}
		}
	}

	return ttrDelList, nil
}

func (r *Reconciler) DeleteRevision(ctx context.Context, ttrevisions []toolchainv1alpha1.TierTemplateRevision) error {
	for _, revision := range ttrevisions {
		if err := r.Client.Delete(ctx, &revision); err != nil {
			return fmt.Errorf("%s was not deleted : %w", revision.Name, err)
		}
	}
	return nil
}
