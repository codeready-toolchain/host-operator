package masteruserrecord

import (
	"context"
	"fmt"
	"time"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/host-operator/pkg/cluster"
	"github.com/codeready-toolchain/host-operator/pkg/mapper"
	"github.com/codeready-toolchain/host-operator/pkg/metrics"
	"github.com/codeready-toolchain/toolchain-common/controllers"
	"github.com/codeready-toolchain/toolchain-common/pkg/condition"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/source"

	errs "github.com/pkg/errors"
	coputil "github.com/redhat-cop/operator-utils/pkg/util"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	// Finalizers
	murFinalizerName = "finalizer.toolchain.dev.openshift.com"
)

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr manager.Manager, memberClusters map[string]cluster.Cluster) error {
	b := ctrl.NewControllerManagedBy(mgr).
		For(&toolchainv1alpha1.MasterUserRecord{}, builder.WithPredicates(predicate.GenerationChangedPredicate{})).
		Watches(&toolchainv1alpha1.SpaceBinding{}, handler.EnqueueRequestsFromMapFunc(
			controllers.MapToOwnerByLabel(r.Namespace, toolchainv1alpha1.SpaceBindingMasterUserRecordLabelKey))).
		Watches(&toolchainv1alpha1.Space{}, handler.EnqueueRequestsFromMapFunc(
			MapSpaceToMasterUserRecord(r.Client)), builder.WithPredicates(predicate.GenerationChangedPredicate{}))

	// watch UserAccounts in all the member clusters
	for _, memberCluster := range memberClusters {
		b = b.WatchesRawSource(source.Kind[runtimeclient.Object](memberCluster.Cache, &toolchainv1alpha1.UserAccount{},
			handler.EnqueueRequestsFromMapFunc(mapper.MapByResourceName(r.Namespace)),
		))
	}
	return b.Complete(r)
}

// Reconciler reconciles a MasterUserRecord object
type Reconciler struct {
	Client         runtimeclient.Client
	Scheme         *runtime.Scheme
	Namespace      string
	MemberClusters map[string]cluster.Cluster
}

//+kubebuilder:rbac:groups=toolchain.dev.openshift.com,resources=masteruserrecords,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=toolchain.dev.openshift.com,resources=masteruserrecords/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=toolchain.dev.openshift.com,resources=masteruserrecords/finalizers,verbs=update

// Reconcile reads that state of the cluster for a MasterUserRecord object and makes changes based on the state read
// and what is in the MasterUserRecord.Spec
// Note:
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.RequeueAfter > 0 is true, otherwise upon completion it will remove the work from the queue.
func (r *Reconciler) Reconcile(ctx context.Context, request ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("Reconciling MasterUserRecord")

	// Fetch the MasterUserRecord instance
	mur := &toolchainv1alpha1.MasterUserRecord{}
	err := r.Client.Get(ctx, request.NamespacedName, mur)
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

	// If the MUR is not being deleted, create or synchronize UserAccounts.
	if !coputil.IsBeingDeleted(mur) {
		// Add the finalizer if it is not present
		if err := r.addFinalizer(ctx, mur, murFinalizerName); err != nil {
			return reconcile.Result{}, err
		}
		logger.Info("ensuring user accounts")
		membersWithUserAccounts, err := r.ensureUserAccounts(ctx, mur)
		if err != nil {
			return reconcile.Result{}, err
		}
		// if any was just created or updated, then just return
		for _, createdOrUpdated := range membersWithUserAccounts {
			if createdOrUpdated {
				return reconcile.Result{}, nil
			}
		}
		requeueTime, err := r.ensureUserAccountsAreNotPresent(ctx, mur, r.membersWithoutUserAccount(membersWithUserAccounts))
		if err != nil {
			return reconcile.Result{}, err
		} else if requeueTime > 0 {
			return reconcile.Result{RequeueAfter: requeueTime}, err
		}
		// just in case there was no change in the set of UserAccounts and there was no provisioned
		if len(membersWithUserAccounts) == 0 {
			if _, err := alignReadiness(ctx, r.Scheme, r.Client, mur); err != nil {
				return reconcile.Result{}, err
			}
			return reconcile.Result{}, r.Client.Status().Update(ctx, mur)
		}

		// If the MUR is being deleted, delete the UserAccounts in members.
	} else if coputil.HasFinalizer(mur, murFinalizerName) {
		requeueTime, err := r.manageCleanUp(ctx, mur)
		if err != nil {
			return reconcile.Result{}, err
		} else if requeueTime > 0 {
			return reconcile.Result{RequeueAfter: requeueTime}, err
		}
	}

	return reconcile.Result{}, nil
}

func (r *Reconciler) ensureUserAccountsAreNotPresent(ctx context.Context, mur *toolchainv1alpha1.MasterUserRecord, targetClusters map[string]cluster.Cluster) (time.Duration, error) {
	for clusterName, memberCluster := range targetClusters {
		requeueTime, err := r.deleteUserAccount(ctx, memberCluster, mur)
		if err != nil {
			return 0, r.wrapErrorWithStatusUpdate(ctx, mur, r.setStatusFailed(toolchainv1alpha1.MasterUserRecordUnableToDeleteUserAccountsReason), err,
				"failed to delete UserAccount in the member cluster '%s'", clusterName)
		} else if requeueTime > 0 {
			return requeueTime, nil
		}
	}
	return 0, nil
}

func (r *Reconciler) membersWithoutUserAccount(membersWithUserAccounts map[string]bool) map[string]cluster.Cluster {
	membersWithout := map[string]cluster.Cluster{}
	for memberName, memberCluster := range r.MemberClusters {
		if _, found := membersWithUserAccounts[memberName]; !found {
			membersWithout[memberName] = memberCluster
		}
	}
	return membersWithout
}

func (r *Reconciler) ensureUserAccounts(ctx context.Context, mur *toolchainv1alpha1.MasterUserRecord) (map[string]bool, error) {
	spaceBindings := &toolchainv1alpha1.SpaceBindingList{}
	if err := r.Client.List(ctx, spaceBindings,
		runtimeclient.InNamespace(mur.GetNamespace()),
		runtimeclient.MatchingLabels{
			toolchainv1alpha1.SpaceBindingMasterUserRecordLabelKey: mur.Name,
		}); err != nil {
		return nil, r.wrapErrorWithStatusUpdate(ctx, mur, r.setStatusFailed(toolchainv1alpha1.MasterUserRecordUnableToCreateUserAccountReason), err,
			"unable to list SpaceBindings for the MasterUserRecord")
	}
	// let's keep the list of target clusters the UserAccounts should be provisioned to in a map - the value defines if the account was just created or updated
	targetClusters := map[string]bool{}
	for _, binding := range spaceBindings.Items {
		if !coputil.IsBeingDeleted(&binding) { // nolint:gosec
			space := &toolchainv1alpha1.Space{}
			if err := r.Client.Get(ctx, namespacedName(mur.Namespace, binding.Spec.Space), space); err != nil {
				return nil, r.wrapErrorWithStatusUpdate(ctx, mur, r.setStatusFailed(toolchainv1alpha1.MasterUserRecordUnableToCreateUserAccountReason), err,
					"unable to get Space '%s' for the SpaceBinding '%s'", binding.Spec.Space, binding.Name)
			}
			if !coputil.IsBeingDeleted(space) && space.Spec.TargetCluster != "" {
				// todo - right now we provision only one UserAccount. It's provisioned in the same cluster where the default space is created
				// todo - as soon as the other components (reg-service & proxy) are updated to support more UserAccounts per MUR, then this should be changed as well
				if space.Labels[toolchainv1alpha1.SpaceCreatorLabelKey] == mur.Labels[toolchainv1alpha1.MasterUserRecordOwnerLabelKey] {
					if createdOrUpdated, err := r.ensureUserAccount(ctx, mur, space.Spec.TargetCluster); err != nil || createdOrUpdated {
						targetClusters[space.Spec.TargetCluster] = true
						return targetClusters, err
					}
					targetClusters[space.Spec.TargetCluster] = false
					break
				}
			}
		}
	}
	return targetClusters, nil
}

func (r *Reconciler) addFinalizer(ctx context.Context, mur *toolchainv1alpha1.MasterUserRecord, finalizer string) error {
	logger := log.FromContext(ctx)

	// Add the finalizer if it is not present
	if !coputil.HasFinalizer(mur, finalizer) {
		coputil.AddFinalizer(mur, finalizer)
		if err := r.Client.Update(ctx, mur); err != nil {
			return r.wrapErrorWithStatusUpdate(ctx, mur, r.setStatusFailed(toolchainv1alpha1.MasterUserRecordUnableToAddFinalizerReason), err,
				"failed while updating with added finalizer")
		}
		logger.Info("MasterUserRecord now has finalizer")
		return nil
	}
	logger.Info("MasterUserRecord already has finalizer")
	return nil
}

// ensureUserAccount ensures that there's a UserAccount resource on the member clusters for the given `murAccount`.
// If the UserAccount resource already exists, then this latter is synchronized using the given `murAccount` and the associated `mur` status is also updated to reflect
// the UserAccount specs.
// Returns bool as the first argument if the UserAccount was either created or updated
func (r *Reconciler) ensureUserAccount(ctx context.Context, mur *toolchainv1alpha1.MasterUserRecord, targetCluster string) (bool, error) {
	// get & check member cluster
	memberCluster, found := r.MemberClusters[targetCluster]
	if !found {
		return false, r.wrapErrorWithStatusUpdate(ctx, mur, r.setStatusFailed(toolchainv1alpha1.MasterUserRecordTargetClusterNotReadyReason),
			fmt.Errorf("unknown target member cluster '%s'", targetCluster),
			"failed to get the member cluster '%s'", targetCluster)
	}

	// get UserAccount from member
	nsdName := namespacedName(memberCluster.OperatorNamespace, mur.Name)
	userAccount := &toolchainv1alpha1.UserAccount{}
	if err := memberCluster.Client.Get(ctx, nsdName, userAccount); err != nil {
		if errors.IsNotFound(err) {
			// does not exist - should create
			userAccount = newUserAccount(nsdName, mur)

			if err := memberCluster.Client.Create(ctx, userAccount); err != nil {
				return false, r.wrapErrorWithStatusUpdate(ctx, mur, r.setStatusFailed(toolchainv1alpha1.MasterUserRecordUnableToCreateUserAccountReason), err,
					"failed to create UserAccount in the member cluster '%s'", targetCluster)
			}
			return true, updateStatusConditions(ctx, r.Client, mur, toBeNotReady(toolchainv1alpha1.MasterUserRecordProvisioningReason, ""))
		}
		// another/unexpected error occurred while trying to fetch the user account on the member cluster
		return false, r.wrapErrorWithStatusUpdate(ctx, mur, r.setStatusFailed(toolchainv1alpha1.MasterUserRecordUnableToGetUserAccountReason), err,
			"failed to get userAccount '%s' from cluster '%s'", mur.Name, targetCluster)
	}
	// if the UserAccount is being deleted (by accident?), then we should wait until is has been totally deleted, and this controller will recreate it again
	logger := log.FromContext(ctx)
	if coputil.IsBeingDeleted(userAccount) {
		logger.Info("UserAccount is being deleted. Waiting until deletion is complete", "member_cluster", memberCluster.Name)

		return true, updateStatusConditions(ctx, r.Client, mur, toBeNotReady(toolchainv1alpha1.MasterUserRecordProvisioningReason, "recovering deleted UserAccount"))
	}

	sync := Synchronizer{
		record:        mur,
		hostClient:    r.Client,
		memberCluster: memberCluster,
		memberUserAcc: userAccount,
		logger:        logger,
		scheme:        r.Scheme,
	}
	updated, err := sync.synchronizeSpec(ctx)
	if err != nil {
		// note: if we got an error while sync'ing the spec, then we may not be able to update the MUR status it here neither.
		return false, r.wrapErrorWithStatusUpdate(ctx, mur, r.setStatusFailed(toolchainv1alpha1.MasterUserRecordUnableToSynchronizeUserAccountSpecReason), err,
			"update of the UserAccount.spec in the cluster '%s' failed", targetCluster)
	}
	if err := sync.synchronizeStatus(ctx); err != nil {
		err = errs.Wrapf(err, "update of the MasterUserRecord failed while synchronizing with UserAccount status from the cluster '%s'", targetCluster)
		// note: if we got an error while updating the status, then we probably can't update it here neither.
		return false, r.wrapErrorWithStatusUpdate(ctx, mur, r.useExistingConditionOfType(toolchainv1alpha1.ConditionReady), err, "")
	}
	// nothing done and no error occurred
	logger.Info("user account on member cluster was already present", "target_cluster", targetCluster, "updated", updated)
	return updated, nil
}

type statusUpdater func(ctx context.Context, mur *toolchainv1alpha1.MasterUserRecord, message string) error

// wrapErrorWithStatusUpdate wraps the error and update the user account status. If the update failed then logs the error.
func (r *Reconciler) wrapErrorWithStatusUpdate(ctx context.Context, mur *toolchainv1alpha1.MasterUserRecord, updateStatus statusUpdater, err error, format string, args ...interface{}) error {
	if err == nil {
		return nil
	}
	if err := updateStatus(ctx, mur, err.Error()); err != nil {
		log.FromContext(ctx).Error(err, "status update failed")
	}
	if format != "" {
		return errs.Wrapf(err, format, args...)
	}
	return err
}

func (r *Reconciler) setStatusFailed(reason string) statusUpdater {
	return func(ctx context.Context, mur *toolchainv1alpha1.MasterUserRecord, message string) error {
		return updateStatusConditions(
			ctx,
			r.Client,
			mur,
			toBeNotReady(reason, message))
	}
}

func (r *Reconciler) useExistingConditionOfType(condType toolchainv1alpha1.ConditionType) statusUpdater {
	return func(ctx context.Context, mur *toolchainv1alpha1.MasterUserRecord, message string) error {
		cond := toolchainv1alpha1.Condition{Type: condType}
		for _, con := range mur.Status.Conditions {
			if con.Type == condType {
				cond = con
				break
			}
		}
		cond.Message = message
		return updateStatusConditions(ctx, r.Client, mur, cond)
	}
}

func (r *Reconciler) manageCleanUp(ctx context.Context, mur *toolchainv1alpha1.MasterUserRecord) (time.Duration, error) {
	if requeue, err := r.ensureUserAccountsAreNotPresent(ctx, mur, r.MemberClusters); err != nil || requeue > 0 {
		return requeue, err
	}
	// Remove finalizer from MasterUserRecord
	coputil.RemoveFinalizer(mur, murFinalizerName)
	if err := r.Client.Update(context.Background(), mur); err != nil {
		return 0, r.wrapErrorWithStatusUpdate(ctx, mur, r.setStatusFailed(toolchainv1alpha1.MasterUserRecordUnableToRemoveFinalizerReason), err,
			"failed to update MasterUserRecord while deleting finalizer")
	}
	domain := metrics.GetEmailDomain(mur)
	logger := log.FromContext(ctx)
	metrics.DecrementMasterUserRecordCount(logger, domain)
	logger.Info("Finalizer removed from MasterUserRecord")
	return 0, nil
}

func (r *Reconciler) deleteUserAccount(ctx context.Context, memberCluster cluster.Cluster, mur *toolchainv1alpha1.MasterUserRecord) (time.Duration, error) {
	logger := log.FromContext(ctx)
	requeueTime := 10 * time.Second

	userAcc := &toolchainv1alpha1.UserAccount{}
	sync := Synchronizer{
		record:        mur,
		hostClient:    r.Client,
		memberCluster: memberCluster,
		memberUserAcc: userAcc,
		logger:        logger,
		scheme:        r.Scheme,
	}

	// Get the User associated with the UserAccount
	namespacedName := types.NamespacedName{Namespace: memberCluster.OperatorNamespace, Name: mur.Name}
	if err := memberCluster.Client.Get(ctx, namespacedName, userAcc); err != nil {
		if errors.IsNotFound(err) {
			logger.Info(fmt.Sprintf("UserAccount is not present in '%s' - making sure that it's not in the MasterUserRecord.Status", memberCluster.Name))
			return 0, sync.removeAccountFromStatus(ctx)
		}
		return 0, err
	}

	if coputil.IsBeingDeleted(userAcc) {
		if err := sync.synchronizeStatus(ctx); err != nil {
			return 0, err
		}
		// if the UserAccount is being deleted, allow up to 1 minute of retries before reporting an error
		deletionTimestamp := userAcc.GetDeletionTimestamp()
		if time.Since(deletionTimestamp.Time) > 60*time.Second {
			return 0, fmt.Errorf("UserAccount deletion has not completed in over 1 minute")
		}
		return requeueTime, nil
	}
	propagationPolicy := metav1.DeletePropagationForeground
	err := memberCluster.Client.Delete(ctx, userAcc, &runtimeclient.DeleteOptions{
		PropagationPolicy: &propagationPolicy,
	})
	if err != nil {
		return 0, err
	}

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
func updateStatusConditions(ctx context.Context, cl runtimeclient.Client, mur *toolchainv1alpha1.MasterUserRecord, newConditions ...toolchainv1alpha1.Condition) error {
	logger := log.FromContext(ctx)

	var updated bool
	mur.Status.Conditions, updated = condition.AddOrUpdateStatusConditions(mur.Status.Conditions, newConditions...)
	if !updated {
		// Nothing changed
		logger.Info("MUR status conditions unchanged")
		return nil
	}
	logger.Info("updating MUR status conditions", "generation", mur.Generation, "resource_version", mur.ResourceVersion)
	err := cl.Status().Update(ctx, mur)
	logger.Info("updated MUR status conditions", "generation", mur.Generation, "resource_version", mur.ResourceVersion)
	return err
}

func newUserAccount(nsdName types.NamespacedName, mur *toolchainv1alpha1.MasterUserRecord) *toolchainv1alpha1.UserAccount {
	ua := &toolchainv1alpha1.UserAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      nsdName.Name,
			Namespace: nsdName.Namespace,
			Labels: map[string]string{
				toolchainv1alpha1.TierLabelKey: mur.Spec.TierName,
			},
		},
		Spec: toolchainv1alpha1.UserAccountSpec{
			Disabled:         mur.Spec.Disabled,
			PropagatedClaims: mur.Spec.PropagatedClaims,
		},
	}

	return ua
}

func namespacedName(namespace, name string) types.NamespacedName {
	return types.NamespacedName{Namespace: namespace, Name: name}
}
