package changetierrequest

import (
	"context"
	"fmt"
	"time"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/host-operator/controllers/nstemplatetier"
	"github.com/codeready-toolchain/host-operator/controllers/usersignup"
	"github.com/codeready-toolchain/host-operator/pkg/configuration"
	"github.com/codeready-toolchain/toolchain-common/pkg/condition"

	"github.com/go-logr/logr"
	"github.com/operator-framework/operator-sdk/pkg/predicate"
	errs "github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("changetierrequest-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to primary resource ChangeTierRequest
	return c.Watch(&source.Kind{Type: &toolchainv1alpha1.ChangeTierRequest{}}, &handler.EnqueueRequestForObject{},
		&predicate.GenerationChangedPredicate{})
}

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr manager.Manager) error {
	return add(mgr, r)
}

// Reconciler reconciles a ChangeTierRequest object
type Reconciler struct {
	Client client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
	Config *configuration.Config
}

//+kubebuilder:rbac:groups=toolchain.dev.openshift.com,resources=changetierrequests,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=toolchain.dev.openshift.com,resources=changetierrequests/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=toolchain.dev.openshift.com,resources=changetierrequests/finalizers,verbs=update

// Reconcile reads that state of the cluster for a ChangeTierRequest object and makes changes based on the state read
// and what is in the ChangeTierRequest.Spec
// Note:
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *Reconciler) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	reqLogger := r.Log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	reqLogger.Info("Reconciling ChangeTierRequest")

	// Fetch the ChangeTierRequest instance
	changeTierRequest := &toolchainv1alpha1.ChangeTierRequest{}
	err := r.Client.Get(context.TODO(), request.NamespacedName, changeTierRequest)
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
	completeCond, found := condition.FindConditionByType(changeTierRequest.Status.Conditions, toolchainv1alpha1.ChangeTierRequestComplete)
	if found && completeCond.Status == corev1.ConditionTrue {
		deleted, requeueAfter, err := r.checkTransitionTimeAndDelete(reqLogger, changeTierRequest, completeCond)
		if deleted {
			return reconcile.Result{}, err
		}
		if err != nil {
			return reconcile.Result{}, r.wrapErrorWithStatusUpdate(reqLogger, changeTierRequest, r.setStatusChangeTierRequestDeletionFailed, err, "failed to delete changeTierRequest")
		}
		return reconcile.Result{
			Requeue:      true,
			RequeueAfter: requeueAfter,
		}, nil
	}

	err = r.changeTier(reqLogger, changeTierRequest, request.Namespace)
	if err != nil {
		return reconcile.Result{}, err
	}

	reqLogger.Info("Change of the tier is completed")
	err = r.setStatusChangeComplete(changeTierRequest)
	if err != nil {
		reqLogger.Error(err, "unable to set change complete status to ChangeTierRequest")
		return reconcile.Result{}, err
	}

	return reconcile.Result{
		Requeue:      true,
		RequeueAfter: r.Config.GetDurationBeforeChangeTierRequestDeletion(),
	}, nil
}

// checkTransitionTimeAndDelete checks if the last transition time has surpassed
// the duration before the changetierrequest should be deleted. If so, the changetierrequest is deleted.
// Returns bool indicating if the changetierrequest was deleted, the time before the changetierrequest
// can be deleted and error
func (r *Reconciler) checkTransitionTimeAndDelete(logger logr.Logger, changeTierRequest *toolchainv1alpha1.ChangeTierRequest, completeCond toolchainv1alpha1.Condition) (bool, time.Duration, error) {
	logger.Info("the ChangeTierRequest is completed so we can deal with its deletion")
	timeSinceCompletion := time.Since(completeCond.LastTransitionTime.Time)

	if timeSinceCompletion >= r.Config.GetDurationBeforeChangeTierRequestDeletion() {
		logger.Info("the ChangeTierRequest has been completed for a longer time than the 'durationBeforeChangeRequestDeletion', so it's ready to be deleted",
			"durationBeforeChangeRequestDeletion", r.Config.GetDurationBeforeChangeTierRequestDeletion().String())
		if err := r.Client.Delete(context.TODO(), changeTierRequest, &client.DeleteOptions{}); err != nil {
			return false, 0, errs.Wrapf(err, "unable to delete ChangeTierRequest object '%s'", changeTierRequest.Name)
		}
		return true, 0, nil
	}
	diff := r.Config.GetDurationBeforeChangeTierRequestDeletion() - timeSinceCompletion
	logger.Info("the ChangeTierRequest has been completed for shorter time than 'durationBeforeChangeRequestDeletion', so it's going to be reconciled again",
		"durationBeforeChangeRequestDeletion", r.Config.GetDurationBeforeChangeTierRequestDeletion().String(), "reconcileAfter", diff.String())
	return false, diff, nil
}

func (r *Reconciler) changeTier(logger logr.Logger, changeTierRequest *toolchainv1alpha1.ChangeTierRequest, namespace string) error {
	mur := &toolchainv1alpha1.MasterUserRecord{}
	murName := types.NamespacedName{Namespace: namespace, Name: changeTierRequest.Spec.MurName}
	if err := r.Client.Get(context.TODO(), murName, mur); err != nil {
		return r.wrapErrorWithStatusUpdate(logger, changeTierRequest, r.setStatusChangeFailed, err, "unable to get MasterUserRecord with name %s", changeTierRequest.Spec.MurName)
	}

	nsTemplateTier := &toolchainv1alpha1.NSTemplateTier{}
	tierName := types.NamespacedName{Namespace: namespace, Name: changeTierRequest.Spec.TierName}
	if err := r.Client.Get(context.TODO(), tierName, nsTemplateTier); err != nil {
		return r.wrapErrorWithStatusUpdate(logger, changeTierRequest, r.setStatusChangeFailed, err, "unable to get NSTemplateTier with name %s", changeTierRequest.Spec.TierName)
	}

	newNsTemplateSet := usersignup.NewNSTemplateSetSpec(nsTemplateTier)
	changed := false

	for i, ua := range mur.Spec.UserAccounts {
		if changeTierRequest.Spec.TargetCluster != "" {
			if ua.TargetCluster == changeTierRequest.Spec.TargetCluster {
				// here we remove the template hash label since it was change for one or all target clusters
				delete(mur.Labels, nstemplatetier.TemplateTierHashLabelKey(mur.Spec.UserAccounts[i].Spec.NSTemplateSet.TierName))
				mur.Spec.UserAccounts[i].Spec.NSTemplateSet = newNsTemplateSet
				changed = true
				break
			}
		} else {
			changed = true
			// here we remove the template hash label since it was change for one or all target clusters
			delete(mur.Labels, nstemplatetier.TemplateTierHashLabelKey(mur.Spec.UserAccounts[i].Spec.NSTemplateSet.TierName))
			mur.Spec.UserAccounts[i].Spec.NSTemplateSet = newNsTemplateSet
		}
	}

	if !changed {
		err := fmt.Errorf("the MasterUserRecord '%s' doesn't contain UserAccount with cluster '%s' whose tier should be changed", changeTierRequest.Spec.MurName, changeTierRequest.Spec.TargetCluster)
		return r.wrapErrorWithStatusUpdate(logger, changeTierRequest, r.setStatusChangeFailed, err, "unable to change tier in MasterUserRecord %s", changeTierRequest.Spec.MurName)
	}

	// also update some of the labels on the MUR, those related to the new Tier in use.
	if mur.Labels == nil {
		mur.Labels = map[string]string{}
	}
	// then we compute again *all* hashes, in case we removed the entry for a single target cluster, but others still "use" it.
	for _, ua := range mur.Spec.UserAccounts {
		hash, err := nstemplatetier.ComputeHashForNSTemplateSetSpec(ua.Spec.NSTemplateSet)
		if err != nil {
			return r.wrapErrorWithStatusUpdate(logger, changeTierRequest, r.setStatusChangeFailed, err, "unable to compute hash for NSTemplateTier with name '%s'", nsTemplateTier.Name)
		}
		mur.Labels[nstemplatetier.TemplateTierHashLabelKey(ua.Spec.NSTemplateSet.TierName)] = hash
	}
	if err := r.Client.Update(context.TODO(), mur); err != nil {
		return r.wrapErrorWithStatusUpdate(logger, changeTierRequest, r.setStatusChangeFailed, err, "unable to change tier in MasterUserRecord %s", changeTierRequest.Spec.MurName)
	}

	return nil
}

func (r *Reconciler) wrapErrorWithStatusUpdate(logger logr.Logger, changeRequest *toolchainv1alpha1.ChangeTierRequest, statusUpdater func(changeRequest *toolchainv1alpha1.ChangeTierRequest, message string) error, err error, format string, args ...interface{}) error {
	if err == nil {
		return nil
	}
	if err := statusUpdater(changeRequest, err.Error()); err != nil {
		logger.Error(err, "status update failed")
	}
	return errs.Wrapf(err, format, args...)
}

func (r *Reconciler) updateStatusConditions(changeRequest *toolchainv1alpha1.ChangeTierRequest, newConditions ...toolchainv1alpha1.Condition) error {
	var updated bool
	changeRequest.Status.Conditions, updated = condition.AddOrUpdateStatusConditions(changeRequest.Status.Conditions, newConditions...)
	if !updated {
		// Nothing changed
		return nil
	}
	return r.Client.Status().Update(context.TODO(), changeRequest)
}

func (r *Reconciler) setStatusChangeComplete(changeRequest *toolchainv1alpha1.ChangeTierRequest) error {
	return r.updateStatusConditions(
		changeRequest,
		toolchainv1alpha1.Condition{
			Type:   toolchainv1alpha1.ChangeTierRequestComplete,
			Status: corev1.ConditionTrue,
			Reason: toolchainv1alpha1.ChangeTierRequestChangedReason,
		})
}

func (r *Reconciler) setStatusChangeFailed(changeRequest *toolchainv1alpha1.ChangeTierRequest, message string) error {
	return r.updateStatusConditions(
		changeRequest,
		toolchainv1alpha1.Condition{
			Type:    toolchainv1alpha1.ChangeTierRequestComplete,
			Status:  corev1.ConditionFalse,
			Reason:  toolchainv1alpha1.ChangeTierRequestChangeFiledReason,
			Message: message,
		})
}

func (r *Reconciler) setStatusChangeTierRequestDeletionFailed(changeRequest *toolchainv1alpha1.ChangeTierRequest, message string) error {
	return r.updateStatusConditions(
		changeRequest,
		toolchainv1alpha1.Condition{
			Type:    toolchainv1alpha1.ChangeTierRequestDeletionError,
			Status:  corev1.ConditionTrue,
			Reason:  toolchainv1alpha1.ChangeTierRequestDeletionErrorReason,
			Message: message,
		})
}
