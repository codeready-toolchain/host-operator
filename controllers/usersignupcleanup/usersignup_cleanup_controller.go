package usersignupcleanup

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/codeready-toolchain/toolchain-common/pkg/hash"
	"github.com/redhat-cop/operator-utils/pkg/util"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/host-operator/controllers/toolchainconfig"
	"github.com/codeready-toolchain/host-operator/pkg/metrics"
	"github.com/codeready-toolchain/toolchain-common/pkg/condition"
	"github.com/codeready-toolchain/toolchain-common/pkg/states"

	errs "github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	// MaxActivationsForCleanup is the maximum number of activations that a user may have in order to be considered
	// for cleanup.  If the number of user activations exceeds this constant value, then cleanup will not occur.
	MaxActivationsForCleanup = 1
)

type StatusUpdater func(userAcc *toolchainv1alpha1.UserSignup, message string) error

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr manager.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		Named("UserSignupCleanup").
		For(&toolchainv1alpha1.UserSignup{}).
		Complete(r)
}

// Reconciler cleans up old UserSignup resources
type Reconciler struct {
	Client runtimeclient.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=toolchain.dev.openshift.com,resources=masteruserrecords,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=toolchain.dev.openshift.com,resources=masteruserrecords/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=toolchain.dev.openshift.com,resources=masteruserrecords/finalizers,verbs=update

// Reconcile reads that state of the cluster for a UserSignup object and makes changes based on the state read
// and what is in the UserSignup.Spec
// Note:
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *Reconciler) Reconcile(ctx context.Context, request ctrl.Request) (ctrl.Result, error) {
	reqLogger := log.FromContext(ctx)
	reqLogger.Info("Reconciling UserSignup")

	// Fetch the UserSignup instance
	instance := &toolchainv1alpha1.UserSignup{}
	err := r.Client.Get(ctx, request.NamespacedName, instance)
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
	reqLogger = reqLogger.WithValues("username", instance.Spec.IdentityClaims.PreferredUsername)
	ctx = log.IntoContext(ctx, reqLogger)

	if util.IsBeingDeleted(instance) {
		reqLogger.Info("UserSignup is already being deleted")
		return reconcile.Result{}, nil
	}

	config, err := toolchainconfig.GetToolchainConfig(r.Client)
	if err != nil {
		return reconcile.Result{}, errs.Wrapf(err, "unable to get ToolchainConfig")
	}
	activations, activationsAnnotationPresent := instance.Annotations[toolchainv1alpha1.UserSignupActivationCounterAnnotationKey]

	// Check if the UserSignup is waiting for phone verification to be finished
	if states.VerificationRequired(instance) {
		// If the UserSignup has no previous activations (i.e. it's a new, never-provisioned UserSignup) then
		// it should be deleted if it has been in an unverified state beyond a configured period of time
		if !activationsAnnotationPresent || activations == "0" {
			createdTime := instance.CreationTimestamp

			unverifiedThreshold := time.Now().Add(-time.Duration(config.Deactivation().UserSignupUnverifiedRetentionDays()*24) * time.Hour)

			if createdTime.Time.Before(unverifiedThreshold) {
				reqLogger.Info("Deleting UserSignup due to exceeding unverified retention period")
				return reconcile.Result{}, r.deleteSignupUnverifiedRetentionPeriod(ctx, instance)
			}

			// Requeue this for reconciliation after the time has passed between the last active time
			// and the current unverified user deletion expiry threshold
			requeueAfter := createdTime.Sub(unverifiedThreshold)

			// Requeue the reconciler to process this resource again after the threshold for unverified user deletion
			return reconcile.Result{
				Requeue:      true,
				RequeueAfter: requeueAfter,
			}, nil
		}

		// If the UserSignup has been reactivated however the user has failed to complete the verification process
		// after a configured threshold, then it should be returned to deactivated state.
		cond, found := condition.FindConditionByType(instance.Status.Conditions, toolchainv1alpha1.UserSignupComplete)
		// If the "Complete" condition is found, and its Reason value is equal to "VerificationRequired" then proceed
		if found && cond.Reason == toolchainv1alpha1.UserSignupVerificationRequiredReason {

			// Use the same "unverified retention days" configuration parameter to determine whether the UserSignup should
			// be returned to a deactivated state
			unverifiedThreshold := time.Now().Add(-time.Duration(config.Deactivation().UserSignupUnverifiedRetentionDays()*24) * time.Hour)
			if cond.LastTransitionTime.Time.Before(unverifiedThreshold) {
				// The UserSignup has been in an unverified state for an excessive period of time, reset it to deactivated state
				states.SetDeactivated(instance, true)
				states.SetVerificationRequired(instance, false)

				reqLogger.Info("Resetting UserSignup back to deactivated state due to exceeding unverified period threshold")
				return reconcile.Result{}, r.Client.Update(ctx, instance)
			}
		}
	}

	// Check if the UserSignup is:
	// * either deactivated
	if states.Deactivated(instance) {
		// Find the UserSignupComplete condition
		cond, found := condition.FindConditionByType(instance.Status.Conditions, toolchainv1alpha1.UserSignupComplete)
		if !found {
			// We cannot find the status Complete condition
			return reconcile.Result{}, nil
		}

		// If the LastTransitionTime of the deactivated status condition is older than the configured threshold,
		// then delete the UserSignup
		deactivatedThreshold := time.Now().Add(-time.Duration(config.Deactivation().UserSignupDeactivatedRetentionDays()*24) * time.Hour)

		meetsActivationCriteria := false
		if !activationsAnnotationPresent {
			// Meets the criteria if there is no activations annotation
			meetsActivationCriteria = true
		} else {
			activationCount, err := strconv.Atoi(activations)
			if err != nil {
				// If there was an error converting the activation count to an integer, then assume it meets the criteria for deletion
				meetsActivationCriteria = true
			} else {
				// Otherwise it meets the criteria if we can determine there has only ever been one activation in total
				meetsActivationCriteria = activationCount <= MaxActivationsForCleanup
			}
		}

		banned, err := r.isUserBanned(ctx, instance)
		if err != nil {
			return reconcile.Result{}, err
		}

		if cond.LastTransitionTime.Time.Before(deactivatedThreshold) && meetsActivationCriteria && !banned {
			reqLogger.Info("Deleting UserSignup due to exceeding deactivated retention period")
			return reconcile.Result{}, r.DeleteUserSignup(ctx, instance)
		}

		// Requeue this for reconciliation after the time has passed between the last transition time
		// and the current deletion expiry threshold
		requeueAfter := cond.LastTransitionTime.Sub(deactivatedThreshold)

		// Requeue the reconciler to process this resource again after the threshold for deletion
		return reconcile.Result{
			Requeue:      true,
			RequeueAfter: requeueAfter,
		}, nil
	}

	return reconcile.Result{}, nil
}

func (r *Reconciler) isUserBanned(ctx context.Context, userSignup *toolchainv1alpha1.UserSignup) (bool, error) {
	banned := false
	// Lookup the user email
	if userSignup.Spec.IdentityClaims.Email != "" {

		// Lookup the email hash label
		if emailHashLbl, exists := userSignup.Labels[toolchainv1alpha1.UserSignupUserEmailHashLabelKey]; exists {

			labels := map[string]string{toolchainv1alpha1.BannedUserEmailHashLabelKey: emailHashLbl}
			opts := runtimeclient.MatchingLabels(labels)
			bannedUserList := &toolchainv1alpha1.BannedUserList{}

			// Query BannedUser for resources that match the same email hash
			if err := r.Client.List(ctx, bannedUserList, opts); err != nil {
				return false, err
			}

			// One last check to confirm that the e-mail addresses match also (in case of the infinitesimal chance of a hash collision)
			for _, bannedUser := range bannedUserList.Items {
				if bannedUser.Spec.Email == userSignup.Spec.IdentityClaims.Email {
					banned = true
					break
				}
			}

			hashIsValid := validateEmailHash(userSignup.Spec.IdentityClaims.Email, emailHashLbl)
			if !hashIsValid {
				err := fmt.Errorf("email hash is invalid for UserSignup [%s]", userSignup.Name)
				return banned, err
			}
		} else {
			// If there isn't an email-hash label, then the state is invalid
			err := fmt.Errorf("missing email-hash label for UserSignup [%s]", userSignup.Name)
			return banned, err
		}
	} else {
		err := fmt.Errorf("could not determine email address for UserSignup [%s]", userSignup.Name)
		return banned, err
	}
	return banned, nil
}

func validateEmailHash(userEmail, userEmailHash string) bool {
	return hash.EncodeString(userEmail) == userEmailHash
}

// deleteSignupUnverifiedRetentionPeriod deletes specified Usersignup and increments metrics if applicable.
// metrics incremented - UserSignupDeletedWithInitiatingVerificationTotal and UserSignupDeletedWithoutInitiatingVerificationTotal
func (r *Reconciler) deleteSignupUnverifiedRetentionPeriod(ctx context.Context, userSignup *toolchainv1alpha1.UserSignup) error {
	// before deleting the resource, we want to "remember" if the user triggered a phone verification or not,
	// based on the presence of the `toolchain.dev.openshift.com/verification-code` annotation
	_, phoneVerificationTriggered := userSignup.Annotations[toolchainv1alpha1.UserSignupVerificationCodeAnnotationKey]

	err := r.DeleteUserSignup(ctx, userSignup)
	if err != nil {
		return err
	}

	// increment the appropriate counter, based on whether the phone verification was triggered or not
	if phoneVerificationTriggered {
		metrics.UserSignupDeletedWithInitiatingVerificationTotal.Inc()
	} else {
		metrics.UserSignupDeletedWithoutInitiatingVerificationTotal.Inc()
	}
	logger := log.FromContext(ctx)
	logger.Info("incremented counter", "name", userSignup.Name, "phone verification triggered", phoneVerificationTriggered)
	return err
}

// DeleteUserSignup deletes the specified UserSignup
func (r *Reconciler) DeleteUserSignup(ctx context.Context, userSignup *toolchainv1alpha1.UserSignup) error {
	propagationPolicy := metav1.DeletePropagationForeground
	err := r.Client.Delete(ctx, userSignup, &runtimeclient.DeleteOptions{
		PropagationPolicy: &propagationPolicy,
	})
	if err != nil {
		return err
	}

	logger := log.FromContext(ctx)
	logger.Info("Deleted UserSignup", "name", userSignup.Name)
	return nil
}
