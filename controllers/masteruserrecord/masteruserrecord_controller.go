package masteruserrecord

import (
	"context"
	"fmt"
	"time"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/host-operator/pkg/configuration"
	"github.com/codeready-toolchain/host-operator/pkg/counter"
	"github.com/codeready-toolchain/host-operator/pkg/metrics"
	"github.com/codeready-toolchain/toolchain-common/pkg/cluster"
	"github.com/codeready-toolchain/toolchain-common/pkg/condition"

	"github.com/go-logr/logr"
	"github.com/operator-framework/operator-sdk/pkg/predicate"
	errs "github.com/pkg/errors"
	"github.com/redhat-cop/operator-utils/pkg/util"
	coputil "github.com/redhat-cop/operator-utils/pkg/util"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

const (
	// Finalizers
	murFinalizerName = "finalizer.toolchain.dev.openshift.com"
)

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("masteruserrecord-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to primary resource MasterUserRecord
	err = c.Watch(&source.Kind{
		Type: &toolchainv1alpha1.MasterUserRecord{}},
		&handler.EnqueueRequestForObject{},
		predicate.GenerationChangedPredicate{})
	if err != nil {
		return err
	}

	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr manager.Manager) error {
	return add(mgr, r)
}

// Reconciler reconciles a MasterUserRecord object
type Reconciler struct {
	Client                client.Client
	Log                   logr.Logger
	Scheme                *runtime.Scheme
	RetrieveMemberCluster func(name string) (*cluster.CachedToolchainCluster, bool)
	Config                *configuration.Config
}

//+kubebuilder:rbac:groups=toolchain.dev.openshift.com,resources=masteruserrecords,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=toolchain.dev.openshift.com,resources=masteruserrecords/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=toolchain.dev.openshift.com,resources=masteruserrecords/finalizers,verbs=update

// Reconcile reads that state of the cluster for a MasterUserRecord object and makes changes based on the state read
// and what is in the MasterUserRecord.Spec
// Note:
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *Reconciler) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	logger := r.Log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	logger.Info("Reconciling MasterUserRecord")

	// Fetch the MasterUserRecord instance
	mur := &toolchainv1alpha1.MasterUserRecord{}
	err := r.Client.Get(context.TODO(), request.NamespacedName, mur)
	if err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		logger.Error(err, "unable to get MasterUserRecord")
		return reconcile.Result{}, err
	}

	// If the UserAccount is not being deleted, create or synchronize UserAccounts.
	if !coputil.IsBeingDeleted(mur) {
		// Add the finalizer if it is not present
		if err := r.addFinalizer(logger, mur, murFinalizerName); err != nil {
			logger.Error(err, "unable to add finalizer to MasterUserRecord")
			return reconcile.Result{}, err
		}
		logger.Info("ensuring user accounts")
		for _, account := range mur.Spec.UserAccounts {
			requeueTime, err := r.ensureUserAccount(logger, account, mur)
			if err != nil {
				logger.Error(err, "unable to synchronize with member UserAccount")
				return reconcile.Result{}, err
			} else if requeueTime > 0 {
				return reconcile.Result{Requeue: true, RequeueAfter: requeueTime}, err // waiting for a few seconds to give time to the member cluster to finish its deletions
			}
		}
		// If the UserAccount is being deleted, delete the UserAccounts in members.
	} else if coputil.HasFinalizer(mur, murFinalizerName) {
		requeueTime, err := r.manageCleanUp(logger, mur)
		if err != nil {
			logger.Error(err, "unable to clean up MasterUserRecord as part of deletion")
			return reconcile.Result{}, err
		} else if requeueTime > 0 {
			return reconcile.Result{Requeue: true, RequeueAfter: requeueTime}, err
		}
	}

	return reconcile.Result{}, nil
}

func (r *Reconciler) addFinalizer(logger logr.Logger, mur *toolchainv1alpha1.MasterUserRecord, finalizer string) error {
	// Add the finalizer if it is not present
	if !coputil.HasFinalizer(mur, finalizer) {
		coputil.AddFinalizer(mur, finalizer)
		if err := r.Client.Update(context.TODO(), mur); err != nil {
			return r.wrapErrorWithStatusUpdate(logger, mur, r.setStatusFailed(toolchainv1alpha1.MasterUserRecordUnableToAddFinalizerReason), err,
				"failed while updating with added finalizer")
		}
		logger.Info("MasterUserRecord now has finalizer")
		return nil
	}
	logger.Info("MasterUserRecord already has finalizer")
	return nil
}

// ensureUserAccount ensures that there's a UserAccount resource on the member cluster for the given `murAccount`.
// If the UserAccount resource already exists, then this latter is synchronized using the given `murAccount` and the associated `mur` status is also updated to reflect
// the UserAccount specs.
// Returns non-zero duration as the first argument if there is a need for requeing (eg, if the remote UserAccount is being deleted and the controller should wait until the deletion is complete)
func (r *Reconciler) ensureUserAccount(logger logr.Logger, murAccount toolchainv1alpha1.UserAccountEmbedded, mur *toolchainv1alpha1.MasterUserRecord) (time.Duration, error) {
	// get & check member cluster
	memberCluster, err := r.getMemberCluster(murAccount.TargetCluster)
	if err != nil {
		return 0, r.wrapErrorWithStatusUpdate(logger, mur, r.setStatusFailed(toolchainv1alpha1.MasterUserRecordTargetClusterNotReadyReason), err,
			"failed to get the member cluster '%s'", murAccount.TargetCluster)
	}

	// get UserAccount from member
	nsdName := namespacedName(memberCluster.OperatorNamespace, mur.Name)
	userAccount := &toolchainv1alpha1.UserAccount{}
	if err := memberCluster.Client.Get(context.TODO(), nsdName, userAccount); err != nil {
		if errors.IsNotFound(err) {
			// does not exist - should create
			userAccount = newUserAccount(nsdName, murAccount.Spec, mur.Spec)
			if err := memberCluster.Client.Create(context.TODO(), userAccount); err != nil {
				return 0, r.wrapErrorWithStatusUpdate(logger, mur, r.setStatusFailed(toolchainv1alpha1.MasterUserRecordUnableToCreateUserAccountReason), err,
					"failed to create UserAccount in the member cluster '%s'", murAccount.TargetCluster)
			}
			counter.IncrementUserAccountCount(logger, murAccount.TargetCluster)
			return 0, updateStatusConditions(logger, r.Client, mur, toBeNotReady(toolchainv1alpha1.MasterUserRecordProvisioningReason, ""))
		}
		// another/unexpected error occurred while trying to fetch the user account on the member cluster
		return 0, r.wrapErrorWithStatusUpdate(logger, mur, r.setStatusFailed(toolchainv1alpha1.MasterUserRecordUnableToGetUserAccountReason), err,
			"failed to get userAccount '%s' from cluster '%s'", mur.Name, murAccount.TargetCluster)
	}
	// if the UserAccount is being deleted (by accident?), then we should wait until is has been totally deleted, and this controller will recreate it again
	if util.IsBeingDeleted(userAccount) {
		logger.Info("UserAccount is being deleted. Waiting until deletion is complete", "member_cluster", memberCluster.Name)
		deletionTimestamp := userAccount.GetDeletionTimestamp()

		// this code block makes sure that we will requeue but only 3s after the deletion timestamp, and we should not update the counter twice
		requeueTime := 3 * time.Second
		timeUntilDeletion := time.Until(deletionTimestamp.Time)
		if timeUntilDeletion+3*time.Second >= 0 {
			counter.DecrementUserAccountCount(logger, murAccount.TargetCluster)
			if timeUntilDeletion > 0 {
				requeueTime = timeUntilDeletion + 3*time.Second
			}
		}

		return requeueTime, updateStatusConditions(logger, r.Client, mur, toBeNotReady(toolchainv1alpha1.MasterUserRecordProvisioningReason, "recovering deleted UserAccount"))
	}

	sync := Synchronizer{
		record:            mur,
		hostClient:        r.Client,
		memberCluster:     memberCluster,
		memberUserAcc:     userAccount,
		recordSpecUserAcc: murAccount,
		logger:            logger,
		scheme:            r.Scheme,
		config:            r.Config,
	}
	if err := sync.synchronizeSpec(); err != nil {
		// note: if we got an error while sync'ing the spec, then we may not be able to update the MUR status it here neither.
		return 0, r.wrapErrorWithStatusUpdate(logger, mur, r.setStatusFailed(toolchainv1alpha1.MasterUserRecordUnableToSynchronizeUserAccountSpecReason), err,
			"update of the UserAccount.spec in the cluster '%s' failed", murAccount.TargetCluster)
	}
	if err := sync.synchronizeStatus(); err != nil {
		err = errs.Wrapf(err, "update of the MasterUserRecord failed while synchronizing with UserAccount status from the cluster '%s'", murAccount.TargetCluster)
		// note: if we got an error while updating the status, then we probably can't update it here neither.
		return 0, r.wrapErrorWithStatusUpdate(logger, mur, r.useExistingConditionOfType(toolchainv1alpha1.ConditionReady), err, "")
	}
	// nothing done and no error occurred
	logger.Info("user account on member cluster was already in sync", "target_cluster", murAccount.TargetCluster)
	return 0, nil
}

func (r *Reconciler) getMemberCluster(targetCluster string) (*cluster.CachedToolchainCluster, error) {
	// get & check toolchain cluster
	toolchainCluster, ok := r.RetrieveMemberCluster(targetCluster)
	if !ok {
		return nil, fmt.Errorf("the member cluster %s not found in the registry", targetCluster)
	}
	if !cluster.IsReady(toolchainCluster.ClusterStatus) {
		return nil, fmt.Errorf("the member cluster %s is not ready", targetCluster)
	}
	return toolchainCluster, nil
}

type statusUpdater func(logger logr.Logger, mur *toolchainv1alpha1.MasterUserRecord, message string) error

// wrapErrorWithStatusUpdate wraps the error and update the user account status. If the update failed then logs the error.
func (r *Reconciler) wrapErrorWithStatusUpdate(logger logr.Logger, mur *toolchainv1alpha1.MasterUserRecord, updateStatus statusUpdater, err error, format string, args ...interface{}) error {
	if err == nil {
		return nil
	}
	if err := updateStatus(logger, mur, err.Error()); err != nil {
		logger.Error(err, "status update failed")
	}
	if format != "" {
		return errs.Wrapf(err, format, args...)
	}
	return err
}

func (r *Reconciler) setStatusFailed(reason string) statusUpdater {
	return func(logger logr.Logger, mur *toolchainv1alpha1.MasterUserRecord, message string) error {
		return updateStatusConditions(
			logger,
			r.Client,
			mur,
			toBeNotReady(reason, message))
	}
}

func (r *Reconciler) useExistingConditionOfType(condType toolchainv1alpha1.ConditionType) statusUpdater {
	return func(logger logr.Logger, mur *toolchainv1alpha1.MasterUserRecord, message string) error {
		cond := toolchainv1alpha1.Condition{Type: condType}
		for _, con := range mur.Status.Conditions {
			if con.Type == condType {
				cond = con
				break
			}
		}
		cond.Message = message
		return updateStatusConditions(logger, r.Client, mur, cond)
	}
}

func (r *Reconciler) manageCleanUp(logger logr.Logger, mur *toolchainv1alpha1.MasterUserRecord) (time.Duration, error) {
	for _, ua := range mur.Spec.UserAccounts {
		requeueTime, err := r.deleteUserAccount(logger, ua.TargetCluster, mur.Name)
		if err != nil {
			return 0, r.wrapErrorWithStatusUpdate(logger, mur, r.setStatusFailed(toolchainv1alpha1.MasterUserRecordUnableToDeleteUserAccountsReason), err,
				"failed to delete UserAccount in the member cluster '%s'", ua.TargetCluster)
		} else if requeueTime > 0 {
			return requeueTime, nil
		}
	}
	// Remove finalizer from MasterUserRecord
	coputil.RemoveFinalizer(mur, murFinalizerName)
	if err := r.Client.Update(context.Background(), mur); err != nil {
		return 0, r.wrapErrorWithStatusUpdate(logger, mur, r.setStatusFailed(toolchainv1alpha1.MasterUserRecordUnableToRemoveFinalizerReason), err,
			"failed to update MasterUserRecord while deleting finalizer")
	}
	domain := metrics.GetEmailDomain(mur)
	counter.DecrementMasterUserRecordCount(logger, domain)
	logger.Info("Finalizer removed from MasterUserRecord")
	return 0, nil
}

func (r *Reconciler) deleteUserAccount(logger logr.Logger, targetCluster, name string) (time.Duration, error) {
	requeueTime := 10 * time.Second
	// get & check member cluster
	memberCluster, err := r.getMemberCluster(targetCluster)
	if err != nil {
		return 0, err
	}
	// Get the User associated with the UserAccount
	userAcc := &toolchainv1alpha1.UserAccount{}
	namespacedName := types.NamespacedName{Namespace: memberCluster.OperatorNamespace, Name: name}
	if err = memberCluster.Client.Get(context.TODO(), namespacedName, userAcc); err != nil {
		if errors.IsNotFound(err) {
			logger.Info("UserAccount deleted")
			return 0, nil
		}
		return 0, err
	}

	if coputil.IsBeingDeleted(userAcc) {
		// if the UserAccount is being deleted, allow up to 1 minute of retries before reporting an error
		deletionTimestamp := userAcc.GetDeletionTimestamp()
		if time.Since(deletionTimestamp.Time) > 60*time.Second {
			return 0, fmt.Errorf("UserAccount deletion has not completed in over 1 minute")
		}
		return requeueTime, nil
	}

	if err := memberCluster.Client.Delete(context.TODO(), userAcc); err != nil {
		return 0, err
	}
	counter.DecrementUserAccountCount(logger, targetCluster)

	return requeueTime, nil
}

func toBeProvisioned() toolchainv1alpha1.Condition {
	return toolchainv1alpha1.Condition{
		Type:   toolchainv1alpha1.ConditionReady,
		Status: corev1.ConditionTrue,
		Reason: toolchainv1alpha1.MasterUserRecordProvisionedReason,
	}
}

func toBeNotReady(reason, msg string) toolchainv1alpha1.Condition {
	return toolchainv1alpha1.Condition{
		Type:    toolchainv1alpha1.ConditionReady,
		Status:  corev1.ConditionFalse,
		Reason:  reason,
		Message: msg,
	}
}

func toBeDisabled() toolchainv1alpha1.Condition {
	return toolchainv1alpha1.Condition{
		Type:   toolchainv1alpha1.ConditionReady,
		Status: corev1.ConditionFalse,
		Reason: toolchainv1alpha1.MasterUserRecordDisabledReason,
	}
}

func toBeProvisionedNotificationCreated() toolchainv1alpha1.Condition {
	return toolchainv1alpha1.Condition{
		Type:   toolchainv1alpha1.MasterUserRecordUserProvisionedNotificationCreated,
		Status: corev1.ConditionTrue,
		Reason: toolchainv1alpha1.MasterUserRecordNotificationCRCreatedReason,
	}
}

// updateStatusConditions updates user account status conditions with the new conditions
func updateStatusConditions(logger logr.Logger, cl client.Client, mur *toolchainv1alpha1.MasterUserRecord, newConditions ...toolchainv1alpha1.Condition) error {
	var updated bool
	mur.Status.Conditions, updated = condition.AddOrUpdateStatusConditions(mur.Status.Conditions, newConditions...)
	if !updated {
		// Nothing changed
		logger.Info("MUR status conditions unchanged")
		return nil
	}
	logger.Info("updating MUR status conditions", "generation", mur.Generation, "resource_version", mur.ResourceVersion)
	err := cl.Status().Update(context.TODO(), mur)
	logger.Info("updated MUR status conditions", "generation", mur.Generation, "resource_version", mur.ResourceVersion)
	return err
}

func newUserAccount(nsdName types.NamespacedName, spec toolchainv1alpha1.UserAccountSpecEmbedded, murSpec toolchainv1alpha1.MasterUserRecordSpec) *toolchainv1alpha1.UserAccount {
	return &toolchainv1alpha1.UserAccount{
		ObjectMeta: v1.ObjectMeta{
			Name:      nsdName.Name,
			Namespace: nsdName.Namespace,
		},
		Spec: toolchainv1alpha1.UserAccountSpec{
			UserID:   murSpec.UserID,
			Disabled: murSpec.Disabled,
			UserAccountSpecBase: toolchainv1alpha1.UserAccountSpecBase{
				NSLimit:       spec.UserAccountSpecBase.NSLimit,
				NSTemplateSet: spec.UserAccountSpecBase.NSTemplateSet,
			},
		},
	}
}

func namespacedName(namespace, name string) types.NamespacedName {
	return types.NamespacedName{Namespace: namespace, Name: name}
}
