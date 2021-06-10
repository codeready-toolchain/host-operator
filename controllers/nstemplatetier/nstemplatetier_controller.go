package nstemplatetier

import (
	"context"
	"fmt"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/host-operator/pkg/configuration"
	"github.com/codeready-toolchain/toolchain-common/pkg/condition"
	"github.com/redhat-cop/operator-utils/pkg/util"

	"github.com/go-logr/logr"
	errs "github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

// ----------------------------------------------------------------------------------------------------------------------------
// NSTemplateTier Controller Reconciler:
// . in case of a new NSTemplateTier update to process:
// .. inserts a new record in the `status.updates` history
// .. creates the first TemplateUpdateRequest resource, so the MasterUserRecord update process may begin
// . after a TemplateUpdateRequest belonging to an NSTemplateTier was created/updated/deleted
// .. creates or deletes subsequent TemplateUpdateRequest resources until all MasterUserRecords have been updated (or failed to)
// .. if the MasterUserRecord failed to updated: increment the failure counter and retain the resource name
// ----------------------------------------------------------------------------------------------------------------------------

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new NSTemplateTiers controller
	c, err := controller.New("nstemplatetier-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes on the primary resources (NSTemplateTier)
	if err := c.Watch(&source.Kind{Type: &toolchainv1alpha1.NSTemplateTier{}},
		&handler.EnqueueRequestForObject{},
		predicate.GenerationChangedPredicate{}); err != nil {
		return err
	}
	// Watch for changes on the secondary resources (TemplateUpdateRequest) and requeue the owner NSTemplateTier
	// we DO need to track changes in the status, so we can't use the `GenerationChangedPredicate`
	if err := c.Watch(&source.Kind{Type: &toolchainv1alpha1.TemplateUpdateRequest{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &toolchainv1alpha1.NSTemplateTier{},
	}); err != nil {
		return err
	}
	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr manager.Manager) error {
	return add(mgr, r)
}

// Reconciler reconciles a NSTemplateTier object (only when this latter's specs were updated)
type Reconciler struct {
	Client client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
	Config *configuration.Config
}

//+kubebuilder:rbac:groups=toolchain.dev.openshift.com,resources=nstemplatetiers,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=toolchain.dev.openshift.com,resources=nstemplatetiers/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=toolchain.dev.openshift.com,resources=nstemplatetiers/finalizers,verbs=update

// Reconcile takes care of:
// - inserting a new entry in the `status.updates` (and cleaning the 'failedAccounts` in the previous one)
// - creating and delete the TemplateUpdateRequest to update the MasterUserRecord associated with this tier
// - updating the `Failed` counter in the `status.updates` when a MasterUserRecord failed to update
// - setting the `completionTime` when all MasterUserRecord have been processed
func (r *Reconciler) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	logger := r.Log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)

	// fetch the NSTemplateTier tier
	tier := &toolchainv1alpha1.NSTemplateTier{}
	if err := r.Client.Get(context.TODO(), request.NamespacedName, tier); err != nil {
		if errors.IsNotFound(err) {
			logger.Info("NSTemplateTier not found")
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		logger.Error(err, "unable to get the current NSTemplateTier")
		return reconcile.Result{}, errs.Wrap(err, "unable to get the current NSTemplateTier")
	}

	// create a new entry in the `status.history`
	if added, err := r.ensureStatusUpdateRecord(logger, tier); err != nil {
		logger.Error(err, "unable to insert a new entry in status.updates after NSTemplateTier changed")
		return reconcile.Result{}, errs.Wrap(err, "unable to insert a new entry in status.updates after NSTemplateTier changed")
	} else if added {
		logger.Info("Requeing after adding a new entry in tier.status.updates")
		return reconcile.Result{Requeue: true}, nil
	}
	if done, err := r.ensureTemplateUpdateRequest(logger, tier); err != nil {
		logger.Error(err, "unable to ensure TemplateRequestUpdate resource after NSTemplateTier changed")
		return reconcile.Result{}, errs.Wrap(err, "unable to ensure TemplateRequestUpdate resource after NSTemplateTier changed")
	} else if done {
		logger.Info("All MasterUserRecords are up to date. Setting the completion timestamp")
		if err := r.markUpdateRecordAsCompleted(tier); err != nil {
			logger.Error(err, "unable to mark latest status.update as complete")
			return reconcile.Result{}, errs.Wrap(err, "unable to mark latest status.update as complete")
		}
	}
	return reconcile.Result{}, nil
}

// ensureStatusUpdateRecord adds a new entry in the `status.updates` with the current date/time
// if needed and cleans-up the previous one if needed (ie, clears the `failedAccounts` array)
// returns `true` if an entry was added, `err` if something wrong happened
func (r *Reconciler) ensureStatusUpdateRecord(logger logr.Logger, tier *toolchainv1alpha1.NSTemplateTier) (bool, error) {
	hash, err := ComputeHashForNSTemplateTier(tier)
	if err != nil {
		return false, errs.Wrapf(err, "unable to append an entry in the `status.updates` for NSTemplateTier '%s'", tier.Name)
	}
	// if there was no previous status:
	if len(tier.Status.Updates) == 0 {
		tier.Status.Updates = append(tier.Status.Updates, toolchainv1alpha1.NSTemplateTierHistory{
			StartTime: metav1.Now(),
			Hash:      hash,
		})
		return true, r.Client.Status().Update(context.TODO(), tier)
	}
	// check that last entry
	if tier.Status.Updates[len(tier.Status.Updates)-1].Hash == hash {
		logger.Info("current update (or check if update is needed) is still in progress")
		return false, nil
	}
	// reset the `FailedAccounts` in the previous update as we don't want to retain the usernames
	// for whom the update failed previously (no need to carry such data anymore)
	tier.Status.Updates[len(tier.Status.Updates)-1].FailedAccounts = nil
	logger.Info("Adding a new entry in tier.status.updates")
	tier.Status.Updates = append(tier.Status.Updates, toolchainv1alpha1.NSTemplateTierHistory{
		StartTime: metav1.Now(),
		Hash:      hash,
	})
	return true, r.Client.Status().Update(context.TODO(), tier)
}

// ensureTemplateUpdateRequest ensures that all relared MasterUserRecords are up-to-date with the NSTemplateTier that changed.
// If not, then it creates a TemplateUpdateRequest resource for the first MasterUserRecord not up-to-date with the tier, and
// returns `false, nil` so the controller will wait for the next reconcile loop to create subsequent TemplateUpdateRequest resources,
// until the `MaxPoolSize` threashold is reached (returns `false, nil`) or no other MasterUserRecord needs to be updated (returns `true,nil`)
func (r *Reconciler) ensureTemplateUpdateRequest(logger logr.Logger, tier *toolchainv1alpha1.NSTemplateTier) (bool, error) {
	if activeTemplateUpdateRequests, deleted, err := r.activeTemplateUpdateRequests(logger, tier); err != nil {
		return false, errs.Wrap(err, "unable to get active TemplateUpdateRequests")
	} else if deleted {
		logger.Info("requeuing as a TemplateUpdateRequest was deleted")
		// skip TemplateUpdateRequest creation in this reconcile loop since one was deleted
		return false, nil
	} else if activeTemplateUpdateRequests < r.Config.GetTemplateUpdateRequestMaxPoolSize() {
		// create a TemplateUpdateRequest if active count < MaxPoolSize,
		// ie, find a MasterUserRecord which is not already up-to-date
		// and for which there is no TemplateUpdateRequest yet

		// fetch by subsets of "MaxPoolSize + 1" size until a MasterUserRecord candidate is found
		murs := toolchainv1alpha1.MasterUserRecordList{}
		matchingLabels, err := murSelector(tier)
		if err != nil {
			return false, errs.Wrap(err, "unable to get MasterUserRecords to update")
		}
		if err = r.Client.List(context.Background(), &murs,
			client.InNamespace(tier.Namespace),
			client.Limit(r.Config.GetTemplateUpdateRequestMaxPoolSize()+1),
			matchingLabels,
		); err != nil {
			return false, errs.Wrap(err, "unable to get MasterUserRecords to update")
		}
		logger.Info("listed MasterUserRecords", "count", len(murs.Items), "selector", matchingLabels)
		if activeTemplateUpdateRequests == 0 && len(murs.Items) == 0 {
			// we've reached the end: all MasterUserRecords are up-to-date
			return true, nil
		}
		for _, mur := range murs.Items {
			// check if there's already a TemplateUpdateRequest for this MasterUserRecord
			templateUpdateRequest := toolchainv1alpha1.TemplateUpdateRequest{}
			if err := r.Client.Get(context.TODO(), types.NamespacedName{
				Namespace: tier.Namespace,
				Name:      mur.Name,
			}, &templateUpdateRequest); err == nil {
				logger.Info("MasterUserRecord already has an associated TemplateUpdateRequest", "name", mur.Name)
				continue
			} else if !errors.IsNotFound(err) {
				return false, errs.Wrapf(err, "unable to get TemplateUpdateRequest for MasterUserRecord '%s'", mur.Name)
			}
			logger.Info("creating a TemplateUpdateRequest to update the MasterUserRecord", "name", mur.Name, "tier", tier.Name)
			tur := &toolchainv1alpha1.TemplateUpdateRequest{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: tier.Namespace,
					Name:      mur.Name,
					Labels: map[string]string{
						toolchainv1alpha1.NSTemplateTierNameLabelKey: tier.Name,
					},
				},
				Spec: toolchainv1alpha1.TemplateUpdateRequestSpec{
					TierName:         tier.Name,
					Namespaces:       tier.Spec.Namespaces,
					ClusterResources: tier.Spec.ClusterResources,
				},
			}
			if err = controllerutil.SetControllerReference(tier, tur, r.Scheme); err != nil {
				return false, err
			}
			// the controller creates a single TemplateUpdateRequest resource per reconcile loop,
			// and the creation of this TemplateUpdateRequest will trigger another reconcile loop
			// since the controller watches TemplateUpdateRequests owned by the NSTemplateTier
			return false, r.Client.Create(context.TODO(), tur)
		}
	}
	logger.Info("done for now with creating TemplateUpdateRequest resources after update of NSTemplateTier", "tier", tier.Name)
	return false, nil
}

// activeTemplateUpdateRequests counts the "active" TemplateUpdateRequests.
// Returns:
// - the number of active TemplateUpdateRequests (ie, not complete, failed or being deleted)
// - `true` if a TemplateUpdateRequest was deleted
// - err if something bad happened
func (r *Reconciler) activeTemplateUpdateRequests(logger logr.Logger, tier *toolchainv1alpha1.NSTemplateTier) (int, bool, error) {
	// fetch the list of TemplateUpdateRequest owned by the NSTemplateTier tier
	templateUpdateRequests := toolchainv1alpha1.TemplateUpdateRequestList{}
	if err := r.Client.List(context.TODO(), &templateUpdateRequests, client.MatchingLabels{
		toolchainv1alpha1.NSTemplateTierNameLabelKey: tier.Name,
	}); err != nil {
		return -1, false, err
	}

	// count non-deleted templateUpdateRequest items
	items := make(map[string]*metav1.Time, len(templateUpdateRequests.Items))
	for _, item := range templateUpdateRequests.Items {
		items[item.Name] = item.DeletionTimestamp
	}
	logger.Info("checking templateUpdateRequests", "items", items)
	count := 0
	for _, tur := range templateUpdateRequests.Items {
		logger.Info("checking templateUpdateRequest", "name", tur.Name, "deleted", util.IsBeingDeleted(&tur))
		if util.IsBeingDeleted(&tur) {
			// ignore when already being deleted
			logger.Info("skipping TemplateUpdateRequest as it is already being deleted", "name", tur.Name)
			continue
		}

		// delete when in `complete=true` (reason=updated) or when in `complete=false/reason=failed` status conditions
		if condition.IsTrue(tur.Status.Conditions, toolchainv1alpha1.TemplateUpdateRequestComplete) ||
			(condition.IsFalseWithReason(tur.Status.Conditions, toolchainv1alpha1.TemplateUpdateRequestComplete, toolchainv1alpha1.TemplateUpdateRequestUnableToUpdateReason) &&
				maxUpdateFailuresReached(tur, r.Config.GetMasterUserRecordUpdateFailureThreshold())) {
			if err := r.incrementCounters(logger, tier, tur); err != nil {
				return -1, false, err
			}
			if err := r.Client.Delete(context.TODO(), &tur); err != nil {
				if errors.IsNotFound(err) {
					logger.Info("skipping failed TemplateUpdateRequest as it was already deleted", "name", tur.Name)
					continue
				}
				return -1, false, errs.Wrapf(err, "unable to delete the TemplateUpdateRequest resource '%s'", tur.Name)
			}
			// will exit the reconcile loop
			return -1, true, nil
		}
		count++
	}
	logger.Info("found active TemplateUpdateRequests for the current tier", "count", count)
	return count, false, nil
}

// maxUpdateFailuresReached checks if the number of failure to update the MasterUserRecord is beyond the configured threshold
func maxUpdateFailuresReached(tur toolchainv1alpha1.TemplateUpdateRequest, threshod int) bool {
	return condition.Count(tur.Status.Conditions,
		toolchainv1alpha1.TemplateUpdateRequestComplete,
		corev1.ConditionFalse,
		toolchainv1alpha1.TemplateUpdateRequestUnableToUpdateReason) >= threshod
}

// incrementCounters looks-up the latest entry in the `status.updates` and increments the `Total` and `Failures` counters
func (r *Reconciler) incrementCounters(logger logr.Logger, tier *toolchainv1alpha1.NSTemplateTier, tur toolchainv1alpha1.TemplateUpdateRequest) error {
	if len(tier.Status.Updates) == 0 {
		return fmt.Errorf("no entry in the `Status.Updates`")
	}
	latest := tier.Status.Updates[len(tier.Status.Updates)-1]
	if condition.IsFalseWithReason(tur.Status.Conditions, toolchainv1alpha1.TemplateUpdateRequestComplete, toolchainv1alpha1.TemplateUpdateRequestUnableToUpdateReason) {
		c, _ := condition.FindConditionByType(tur.Status.Conditions, toolchainv1alpha1.TemplateUpdateRequestComplete)
		logger.Info("incrementing failure counter after TemplateUpdateRequest failed", "reason", c.Reason)
		latest.Failures++
		latest.FailedAccounts = append(latest.FailedAccounts, tur.Name)
	}
	tier.Status.Updates[len(tier.Status.Updates)-1] = latest
	if err := r.Client.Status().Update(context.TODO(), tier); err != nil {
		return err
	}
	logger.Info("incrementing counter after TemplateUpdateRequest completed", "name", tur.Name)
	return nil
}

// markUpdateRecordAsCompleted looks-up the latest entry in the `status.updates` and sets the `CompletionTime` to `metav1.Now()`,
// which means that the whole update process is done (whether there were accounts to update or not)
func (r *Reconciler) markUpdateRecordAsCompleted(tier *toolchainv1alpha1.NSTemplateTier) error {
	if len(tier.Status.Updates) == 0 {
		return fmt.Errorf("no entry in the `Status.Updates`")
	}
	latest := tier.Status.Updates[len(tier.Status.Updates)-1]
	now := metav1.Now()
	latest.CompletionTime = &now
	tier.Status.Updates[len(tier.Status.Updates)-1] = latest
	return r.Client.Status().Update(context.TODO(), tier)
}
