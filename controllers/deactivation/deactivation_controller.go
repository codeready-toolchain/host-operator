package deactivation

import (
	"context"
	"fmt"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"strings"
	"time"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/host-operator/controllers/toolchainconfig"
	"github.com/codeready-toolchain/host-operator/pkg/metrics"
	"github.com/codeready-toolchain/toolchain-common/pkg/condition"
	"github.com/codeready-toolchain/toolchain-common/pkg/states"

	errs "github.com/pkg/errors"
	coputil "github.com/redhat-cop/operator-utils/pkg/util"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr manager.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		Named("deactivation").
		For(&toolchainv1alpha1.MasterUserRecord{}, builder.WithPredicates(CreateAndUpdateOnlyPredicate{})).
		Watches(&source.Kind{Type: &toolchainv1alpha1.UserSignup{}},
			handler.EnqueueRequestsFromMapFunc(MapUserSignupToMasterUserRecord())).
		Complete(r)
}

// Reconciler reconciles a Deactivation object
type Reconciler struct {
	Client runtimeclient.Client
	Scheme *runtime.Scheme
}

// Reconcile reads the state of the cluster for a MUR object and determines whether to trigger deactivation or requeue based on its current status
// Note:
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
// nolint: gocyclo
func (r *Reconciler) Reconcile(ctx context.Context, request ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("Reconciling Deactivation")

	config, err := toolchainconfig.GetToolchainConfig(r.Client)
	if err != nil {
		return reconcile.Result{}, errs.Wrapf(err, "unable to get ToolchainConfig")
	}

	// Fetch the MasterUserRecord instance
	mur := &toolchainv1alpha1.MasterUserRecord{}
	err = r.Client.Get(ctx, request.NamespacedName, mur)
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

	// If the MasterUserRecord is being deleted, no need to do anything else
	if coputil.IsBeingDeleted(mur) {
		return reconcile.Result{}, nil
	}

	// Deactivation only applies to users that have been provisioned
	if mur.Status.ProvisionedTime == nil {
		return reconcile.Result{}, nil
	}
	provisionedTimestamp := *mur.Status.ProvisionedTime

	// Get the associated usersignup
	usersignup := &toolchainv1alpha1.UserSignup{}
	if err := r.Client.Get(ctx, types.NamespacedName{
		Namespace: mur.Namespace,
		Name:      mur.Labels[toolchainv1alpha1.MasterUserRecordOwnerLabelKey],
	}, usersignup); err != nil {
		// Error getting usersignup - requeue the request.
		return reconcile.Result{}, err
	}

	// If the usersignup is already deactivated then there's nothing else to do
	if states.Deactivated(usersignup) {
		return reconcile.Result{}, nil
	}

	// Check the domain exclusion list, if the user's email matches then they cannot be automatically deactivated
	if usersignup.Spec.IdentityClaims.Email != "" {
		for _, domain := range config.Deactivation().DeactivationDomainsExcluded() {
			if strings.HasSuffix(usersignup.Spec.IdentityClaims.Email, domain) {
				logger.Info("user cannot be automatically deactivated because they belong to the exclusion list", "domain", domain)

				// Also set the Scheduled deactivation time to nil if it's not already
				if usersignup.Status.ScheduledDeactivationTimestamp != nil {
					usersignup.Status.ScheduledDeactivationTimestamp = nil
					if err := r.Client.Status().Update(ctx, usersignup); err != nil {
						logger.Error(err, "failed to update usersignup status")
						return reconcile.Result{}, err
					}
				}

				return reconcile.Result{}, nil
			}
		}
	}

	if len(mur.Spec.UserAccounts) == 0 {
		err = fmt.Errorf("cannot determine deactivation timeout period because the mur has no associated user accounts")
		logger.Error(err, "failed to process deactivation")
		return reconcile.Result{}, err
	}

	// Get the tier associated with the MasterUserRecord, we'll observe the deactivation timeout period from the tier spec
	userTier := &toolchainv1alpha1.UserTier{}
	if err := r.Client.Get(ctx, types.NamespacedName{Namespace: request.Namespace, Name: mur.Spec.TierName}, userTier); err != nil {
		logger.Error(err, "unable to get the deactivationTimeoutDays from UserTier", "name", mur.Spec.TierName)
		return reconcile.Result{}, err
	}
	deactivationTimeoutDays := userTier.Spec.DeactivationTimeoutDays

	// If the deactivation timeout is 0 then users that belong to this tier should not be automatically deactivated
	if deactivationTimeoutDays == 0 {
		// If the usersignup was already set to deactivating then reset it to false
		if err := r.resetDeactivatingState(ctx, usersignup); err != nil {
			return reconcile.Result{}, err
		}
		// Set the scheduled deactivation time to nil
		if usersignup.Status.ScheduledDeactivationTimestamp != nil {
			usersignup.Status.ScheduledDeactivationTimestamp = nil
			if err := r.Client.Status().Update(ctx, usersignup); err != nil {
				logger.Error(err, "failed to update usersignup status")
				return reconcile.Result{}, err
			}
		}

		logger.Info("User belongs to a tier that does not have a deactivation timeout. The user will not be automatically deactivated")
		// Users belonging to this tier will not be auto deactivated, no need to requeue.
		return reconcile.Result{}, nil
	}

	deactivationTimeout := time.Duration(deactivationTimeoutDays*24) * time.Hour

	logger.Info("user account time values", "deactivation timeout duration", deactivationTimeout, "provisionedTimestamp", provisionedTimestamp)

	timeSinceProvisioned := time.Since(provisionedTimestamp.Time)

	deactivatingNotificationDays := config.Deactivation().DeactivatingNotificationDays()
	deactivatingNotificationTimeout := time.Duration((deactivationTimeoutDays-deactivatingNotificationDays)*24) * time.Hour

	if timeSinceProvisioned < deactivatingNotificationTimeout {
		// It is not yet time to send the deactivating notification

		// If the usersignup was already set to deactivating then reset it to false
		// Example: promotion of a user after 28 days from a user tier with deactivationTimeoutDays = 30 to one with 90, and where deactivatingNotificationDays = 3
		//   Usersignup would have spec.states[Deactivating] = true but there are now 62 days left before deactivation so the deactivating notification should be sent again
		//   when it is 3 days left until deactivation
		if err := r.resetDeactivatingState(ctx, usersignup); err != nil {
			return reconcile.Result{}, err
		}

		// Reset the scheduled deactivation time if required
		scheduledDeactivationTime := (*mur.Status.ProvisionedTime).Add(deactivationTimeout)
		if usersignup.Status.ScheduledDeactivationTimestamp == nil || !usersignup.Status.ScheduledDeactivationTimestamp.Time.Equal(scheduledDeactivationTime) {
			ts := v1.NewTime(scheduledDeactivationTime)
			usersignup.Status.ScheduledDeactivationTimestamp = &ts
			if err := r.Client.Status().Update(ctx, usersignup); err != nil {
				logger.Error(err, "failed to update usersignup status")
				return reconcile.Result{}, err
			}
		}

		// requeue until it will be time to send it
		requeueAfterTimeToNotify := deactivatingNotificationTimeout - timeSinceProvisioned
		logger.Info("requeueing request", "RequeueAfter", requeueAfterTimeToNotify,
			"Expected deactivating notification date/time", time.Now().Add(requeueAfterTimeToNotify).String())
		return reconcile.Result{RequeueAfter: requeueAfterTimeToNotify}, nil
	}

	// If the usersignup state hasn't been set to deactivating, then set it now
	if !states.Deactivating(usersignup) {
		states.SetDeactivating(usersignup, true)

		// Before we update the UserSignup in order to set the deactivating state, we should reset the scheduled
		// deactivation time if required just in case the current value is nil or has somehow changed.  Since the UserSignup
		// controller is going to be reconciling immediately after setting the deactivating state then it will be
		// creating a deactivating notification, meaning that the scheduled deactivation time that we set here is going
		// to be extremely temporary (as it will be recalculated after the next reconcile), however it is done for correctness
		scheduledDeactivationTime := (*mur.Status.ProvisionedTime).Add(deactivationTimeout)
		if usersignup.Status.ScheduledDeactivationTimestamp == nil || !usersignup.Status.ScheduledDeactivationTimestamp.Time.Equal(scheduledDeactivationTime) {
			ts := v1.NewTime(scheduledDeactivationTime)
			usersignup.Status.ScheduledDeactivationTimestamp = &ts
			if err := r.Client.Status().Update(ctx, usersignup); err != nil {
				logger.Error(err, "failed to update usersignup status")
				return reconcile.Result{}, err
			}
		}

		logger.Info("setting usersignup state to deactivating")
		if err := r.Client.Update(ctx, usersignup); err != nil {
			logger.Error(err, "failed to update usersignup")
			return reconcile.Result{}, err
		}

		// Upon the next reconciliation, the deactivation due time can be calculated after the notification has been sent.
		// The sequence of events from here are:
		// 1. This controller has now set the UserSignup state to deactivating if it's not already set
		// 2. UserSignup controller picks up that the deactivating state has been set, and responds by:
		//		a) creating a pre-deactivating notification for the user, and
		//		b) setting the "deactivating notification created" status condition to true.
		// 3. This controller watches UserSignup as a secondary resource, so will be reconciled again once the
		//    UserSignup is updated.
		// 4. This controller calculates the amount of time that has passed since the deactivating notification was sent,
		//  based on the LastTransitionTime of the condition. If enough time has now passed, it sets the UserSignup to deactivated.
		return reconcile.Result{}, nil
	}

	deactivatingCondition, found := condition.FindConditionByType(usersignup.Status.Conditions,
		toolchainv1alpha1.UserSignupUserDeactivatingNotificationCreated)
	if !found || deactivatingCondition.Status != corev1.ConditionTrue ||
		deactivatingCondition.Reason != toolchainv1alpha1.UserSignupDeactivatingNotificationCRCreatedReason {
		// If the UserSignup has been marked as deactivating, however the deactivating notification hasn't been
		// created yet, then wait - the notification should be created shortly by the UserSignup controller
		// once the "deactivating" state has been set which should cause a new reconciliation to be triggered here
		return reconcile.Result{}, nil
	}

	// We calculate the actual deactivation due time based on when the deactivating condition was set.  This may end up
	// being significantly later than the scheduled deactivation time set in UserSignup.Status.ScheduledDeactivationTime, as
	// in some rare circumstances the deactivating notification/status may fail to be set due to cluster downtime or
	// other reasons.  Because of this, the scheduled deactivation time that is set in the UserSignup.Status should be
	// treated as informational only.
	deactivationDueTime := deactivatingCondition.LastTransitionTime.Time.Add(time.Duration(deactivatingNotificationDays*24) * time.Hour)

	if time.Now().Before(deactivationDueTime) {
		// Update the ScheduledDeactivationTimestamp to the recalculated time base on when the deactivating notification was sent
		if usersignup.Status.ScheduledDeactivationTimestamp == nil || !usersignup.Status.ScheduledDeactivationTimestamp.Time.Equal(deactivationDueTime) {
			ts := v1.NewTime(deactivationDueTime)
			usersignup.Status.ScheduledDeactivationTimestamp = &ts
			if err := r.Client.Status().Update(ctx, usersignup); err != nil {
				logger.Error(err, "failed to update usersignup status")
				return reconcile.Result{}, err
			}
		}

		// It is not yet time to deactivate so requeue when it will be
		requeueAfterExpired := time.Until(deactivationDueTime)

		logger.Info("requeueing request", "RequeueAfter", requeueAfterExpired,
			"Expected deactivation date/time", time.Now().Add(requeueAfterExpired).String())
		return reconcile.Result{RequeueAfter: requeueAfterExpired}, nil
	}

	// Deactivate the user
	if states.Deactivated(usersignup) {
		// The UserSignup is already set for deactivation, nothing left to do
		return reconcile.Result{}, nil
	}
	states.SetDeactivated(usersignup, true)

	if err := r.Client.Update(ctx, usersignup); err != nil {
		logger.Error(err, "failed to update usersignup")
		return reconcile.Result{}, err
	}

	metrics.UserSignupAutoDeactivatedTotal.Inc()

	return reconcile.Result{}, nil
}

func (r *Reconciler) resetDeactivatingState(ctx context.Context, usersignup *toolchainv1alpha1.UserSignup) error {
	if states.Deactivating(usersignup) {
		states.SetDeactivating(usersignup, false)
		if err := r.Client.Update(ctx, usersignup); err != nil {
			log.FromContext(ctx).Error(err, "failed to reset usersignup deactivating state")
			return err
		}
	}
	return nil
}
