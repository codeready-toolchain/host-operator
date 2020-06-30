package templateupdaterequest

import (
	"context"
	"strings"
	"time"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/pkg/apis/toolchain/v1alpha1"
	"github.com/codeready-toolchain/host-operator/pkg/configuration"
	"github.com/codeready-toolchain/host-operator/pkg/controller/nstemplatetier"
	"github.com/codeready-toolchain/toolchain-common/pkg/condition"
	"github.com/go-logr/logr"

	errs "github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

var log = logf.Log.WithName("controller_templateupdaterequest")

const (
	// DeletionTimeout the duration after which the TemplateUpdateRequest can be deleted, once it reached the `complete=true` condition
	DeletionTimeout = 5 * time.Second
)

// Add creates a new TemplateUpdateRequest Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager, _ *configuration.Config) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileTemplateUpdateRequest{client: mgr.GetClient(), scheme: mgr.GetScheme()}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("templateupdaterequest-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to primary resource TemplateUpdateRequest
	err = c.Watch(&source.Kind{Type: &toolchainv1alpha1.TemplateUpdateRequest{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	// Watch for changes to secondary resource MasterUserRecords (although, not owned by the TemplateUpdateRequest)
	err = c.Watch(&source.Kind{Type: &toolchainv1alpha1.MasterUserRecord{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	return nil
}

// blank assignment to verify that ReconcileTemplateUpdateRequest implements reconcile.Reconciler
var _ reconcile.Reconciler = &ReconcileTemplateUpdateRequest{}

// ReconcileTemplateUpdateRequest reconciles a TemplateUpdateRequest object
type ReconcileTemplateUpdateRequest struct {
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the apiserver
	client client.Client
	scheme *runtime.Scheme
}

// Reconcile reads that state of the cluster for a TemplateUpdateRequest object and makes changes based on the state read
// and what is in the TemplateUpdateRequest.Spec
// Note:
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcileTemplateUpdateRequest) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	logger := log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	logger.Info("Reconciling TemplateUpdateRequest")

	// Fetch the TemplateUpdateRequest tur
	tur := &toolchainv1alpha1.TemplateUpdateRequest{}
	err := r.client.Get(context.TODO(), request.NamespacedName, tur)
	if err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}

	// if is complete, then check when status was changed and delete it if the requested duration has passed
	completeCond, found := condition.FindConditionByType(tur.Status.Conditions, toolchainv1alpha1.TemplateUpdateRequestComplete)
	if found && completeCond.Status == corev1.ConditionTrue {
		deleted, requeueAfter, err := r.checkTransitionTimeAndDelete(logger, tur, completeCond)
		if deleted {
			return reconcile.Result{}, nil
		}
		if err != nil {
			return reconcile.Result{}, r.updateStatusConditions(logger, tur, toFailure(errs.Wrap(err, "unable to delete the resource")))
		}
		return reconcile.Result{
			Requeue:      true,
			RequeueAfter: requeueAfter,
		}, nil
	}

	// lookup the MasterUserRecord with the same name as the TemplateUpdateRequest tur
	mur := &toolchainv1alpha1.MasterUserRecord{}
	if err = r.client.Get(context.TODO(), request.NamespacedName, mur); err != nil {
		if errors.IsNotFound(err) {
			// MUR object not found, could have been deleted after reconcile request.
			// Marking this TemplateUpdateRequest as failed
			return reconcile.Result{}, r.updateStatusConditions(logger, tur, toFailure(err))
		}
		// Error reading the object - requeue the request.
		logger.Error(err, "Unable to get the MasterUserRecord associated with the TemplateUpdateRequest")
		return reconcile.Result{}, err
	}
	if len(tur.Status.SyncIndexes) == 0 {
		// need to be "captured" before updating the MURs
		syncIndexes := syncIndexes(tur.Spec.TierName, *mur)
		// if the TemplateUpdateRequest was just created (ie, `Status.SyncIndexes` is empty), then we should update the associated MasterUserRecord
		// and retain its current syncIndexex in the status
		if err = r.updateTemplateRefs(logger, *tur, mur); err != nil {
			logger.Error(err, "Unable to update the MasterUserRecord associated with the TemplateUpdateRequest")
			return reconcile.Result{}, r.updateStatusConditions(logger, tur, toFailure(err))
		}
		// update the TemplateUpdateRequest status and requeue to keep tracking the MUR changes
		logger.Info("MasterUserRecord update started. Updating TemplateUpdateRequest status accordingly")
		if err = r.setStatusConditionsUpdating(logger, tur, syncIndexes); err != nil {
			logger.Error(err, "Unable to update the TemplateUpdateRequest status")
			return reconcile.Result{}, err
		}
		// no explicit requeue: expect new reconcile loop when MasterUserRecord changes
		return reconcile.Result{}, nil
	}
	// otherwise, we should compare the sync indexes of the MasterUserRecord until all tier-related values changed
	if r.compareSyncIndexes(logger, *tur, *mur) && condition.IsTrue(mur.Status.Conditions, toolchainv1alpha1.ConditionReady) {
		// once MasterUserRecord is up-to-date, we can delete this TemplateUpdateRequest
		logger.Info("MasterUserRecord is up-to-date. Marking the TemplateUpdateRequest as complete")
		return reconcile.Result{Requeue: true, RequeueAfter: DeletionTimeout}, r.updateStatusConditions(logger, tur, toBeComplete())
	}
	// otherwise, we need to wait
	logger.Info("MasterUserRecord still being updated...")
	// no explicit requeue: expect new reconcile loop when MasterUserRecord changes
	return reconcile.Result{}, nil
}

func (r ReconcileTemplateUpdateRequest) updateTemplateRefs(logger logr.Logger, tur toolchainv1alpha1.TemplateUpdateRequest, mur *toolchainv1alpha1.MasterUserRecord) error {
	// update MasterUserRecord' accounts whose tier matches the TemplateUpdateRequest
	for i, ua := range mur.Spec.UserAccounts {
		if ua.Spec.NSTemplateSet.TierName == tur.Spec.TierName {
			logger.Info("updating templaterefs", "tier", tur.Spec.TierName, "target_cluster", ua.TargetCluster)
			// reset the new templateRefs, only retain those with a custom template in use
			namespaces := make(map[string]*toolchainv1alpha1.NSTemplateSetNamespace, len(ua.Spec.NSTemplateSet.Namespaces))
			for _, ns := range ua.Spec.NSTemplateSet.Namespaces {
				if ns.Template != "" {
					t := namespaceType(ns.TemplateRef)
					namespaces[t] = &ns
				}
			}
			// now, add the new templateRefs, uneless there's a custom template in use (for the same ns type)
			for _, ns := range tur.Spec.Namespaces {
				t := namespaceType(ns.TemplateRef)
				if _, found := namespaces[t]; found {
					// don't override the custom template
					namespaces[t].TemplateRef = ns.TemplateRef
				} else {
					namespaces[t] = &toolchainv1alpha1.NSTemplateSetNamespace{
						TemplateRef: ns.TemplateRef,
					}
				}
			}
			// finally, set the new namespace templates in the user account
			ua.Spec.NSTemplateSet.Namespaces = []toolchainv1alpha1.NSTemplateSetNamespace{}
			for _, ns := range namespaces {
				ua.Spec.NSTemplateSet.Namespaces = append(ua.Spec.NSTemplateSet.Namespaces, *ns)
			}
			// now, let's take care about the cluster resources
			if ua.Spec.NSTemplateSet.ClusterResources != nil && ua.Spec.NSTemplateSet.ClusterResources.Template != "" {
				// retain the custom template
				if tur.Spec.ClusterResources != nil {
					ua.Spec.NSTemplateSet.ClusterResources.TemplateRef = tur.Spec.ClusterResources.TemplateRef
				} else {
					ua.Spec.NSTemplateSet.ClusterResources.TemplateRef = ""
				}
			} else if tur.Spec.ClusterResources != nil {
				ua.Spec.NSTemplateSet.ClusterResources = &toolchainv1alpha1.NSTemplateSetClusterResources{
					TemplateRef: tur.Spec.ClusterResources.TemplateRef,
				}
			} else {
				ua.Spec.NSTemplateSet.ClusterResources = nil
			}
			mur.Spec.UserAccounts[i] = ua
		}
		// also, update the tier template hash label
		hash, err := nstemplatetier.ComputeHashForTemplateUpdateRequest(tur)
		if err != nil {
			return err
		}
		mur.Labels[nstemplatetier.TemplateTierHashLabelKey(tur.Spec.TierName)] = hash

	}
	return r.client.Update(context.TODO(), mur)

}

// extract the type from the given templateRef
// templateRef format: `<tier>-<type>-<hash>`
func namespaceType(templateRef string) string {
	parts := strings.Split(templateRef, "-")
	return parts[1]
}

// compareSyncIndexes compares the sync indexes in the given TemplateUpdateRequest status vs the given MasterUserRecord
// returns `true` if ALL values are DIFFERENT, meaning that all user accounts were updated on the target clusters where the tier is in use
func (r ReconcileTemplateUpdateRequest) compareSyncIndexes(logger logr.Logger, tur toolchainv1alpha1.TemplateUpdateRequest, mur toolchainv1alpha1.MasterUserRecord) bool {
	murIndexes := syncIndexes(tur.Spec.TierName, mur)
	for targetCluster, syncIndex := range tur.Status.SyncIndexes {
		if current, ok := murIndexes[targetCluster]; ok && current == syncIndex {
			logger.Info("Sync index still unchanged", "target_cluster", targetCluster, "sync_index", syncIndex)
			return false
		}
	}
	logger.Info("All sync indexes have been updated")
	return true
}

// checkTransitionTimeAndDelete checks if the last transition time has surpassed
// the duration before the TemplateUpdateRequest should be deleted. If so, the TemplateUpdateRequest is deleted.
// Returns bool indicating if the TemplateUpdateRequest was deleted, the time before the TemplateUpdateRequest
// can be deleted and error
func (r *ReconcileTemplateUpdateRequest) checkTransitionTimeAndDelete(logger logr.Logger, tur *toolchainv1alpha1.TemplateUpdateRequest, completeCond toolchainv1alpha1.Condition) (bool, time.Duration, error) {
	logger.Info("the ChangeTierRequest is completed so we can deal with its deletion")
	timeSinceCompletion := time.Since(completeCond.LastTransitionTime.Time)
	if timeSinceCompletion >= DeletionTimeout {
		logger.Info("the TemplateUpdateRequest has been completes for a long enough time, so it's ready to be deleted")
		if err := r.client.Delete(context.TODO(), tur); err != nil {
			return false, 0, errs.Wrapf(err, "unable to delete the TemplateUpdateRequest resource '%s'", tur.Name)
		}
		return true, 0, nil
	}
	diff := DeletionTimeout - timeSinceCompletion
	logger.Info("the TemplateUpdateRequest has been completed for short time, so it's not going to be deleted yet", "reconcileAfter", diff.String())
	return false, diff, nil
}

// --------------------------------------------------
// status updates
// --------------------------------------------------
type statusUpdater func(logger logr.Logger, tur *toolchainv1alpha1.TemplateUpdateRequest, message string) error

func toFailure(err error) toolchainv1alpha1.Condition {
	return toolchainv1alpha1.Condition{
		Type:    toolchainv1alpha1.TemplateUpdateRequestComplete,
		Status:  corev1.ConditionFalse,
		Reason:  toolchainv1alpha1.TemplateUpdateRequestUnableToUpdateReason,
		Message: err.Error(),
	}
}

func toBeUpdating() toolchainv1alpha1.Condition {
	return toolchainv1alpha1.Condition{
		Type:   toolchainv1alpha1.TemplateUpdateRequestComplete,
		Status: corev1.ConditionFalse,
		Reason: toolchainv1alpha1.TemplateUpdateRequestUpdatingReason,
	}
}

func toBeComplete() toolchainv1alpha1.Condition {
	return toolchainv1alpha1.Condition{
		Type:   toolchainv1alpha1.TemplateUpdateRequestComplete,
		Status: corev1.ConditionTrue,
		Reason: toolchainv1alpha1.TemplateUpdateRequestUpdatedReason,
	}
}

// updateStatusConditions updates the TemplateUpdateRequest status conditions with the new conditions
func (r *ReconcileTemplateUpdateRequest) updateStatusConditions(logger logr.Logger, tur *toolchainv1alpha1.TemplateUpdateRequest, newConditions ...toolchainv1alpha1.Condition) error {
	var updated bool
	tur.Status.Conditions, updated = condition.AddOrUpdateStatusConditions(tur.Status.Conditions, newConditions...)
	if !updated {
		// Nothing changed
		logger.Info("TemplateUpdateRequest status conditions unchanged")
		return nil
	}
	return r.client.Status().Update(context.TODO(), tur)
}

// setStatusConditionsUpdating sets the TemplateUpdateRequest status condition to `complete=false/reason=updating` and retains the sync indexes
func (r *ReconcileTemplateUpdateRequest) setStatusConditionsUpdating(logger logr.Logger, tur *toolchainv1alpha1.TemplateUpdateRequest, syncIndexes map[string]string, conditions ...toolchainv1alpha1.Condition) error {
	tur.Status.SyncIndexes = syncIndexes
	tur.Status.Conditions = []toolchainv1alpha1.Condition{toBeUpdating()}
	return r.client.Status().Update(context.TODO(), tur)
}

// syncIndexes returns the sync indexes related to the given tier, indexed by target cluster
func syncIndexes(tierName string, mur toolchainv1alpha1.MasterUserRecord) map[string]string {
	indexes := map[string]string{}
	for _, ua := range mur.Spec.UserAccounts {
		if ua.Spec.NSTemplateSet.TierName == tierName {
			indexes[ua.TargetCluster] = ua.SyncIndex
		}
	}
	return indexes
}
