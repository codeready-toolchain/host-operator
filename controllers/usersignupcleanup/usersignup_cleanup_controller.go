package usersignupcleanup

import (
	"context"
	"time"

	errs "github.com/pkg/errors"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/host-operator/pkg/metrics"
	"github.com/codeready-toolchain/toolchain-common/pkg/condition"
	commonconfig "github.com/codeready-toolchain/toolchain-common/pkg/configuration"
	"github.com/codeready-toolchain/toolchain-common/pkg/states"
	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type StatusUpdater func(userAcc *toolchainv1alpha1.UserSignup, message string) error

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr manager.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&toolchainv1alpha1.UserSignup{}).
		Complete(r)
}

// Reconciler cleans up old UserSignup resources
type Reconciler struct {
	Client client.Client
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
	err := r.Client.Get(context.TODO(), request.NamespacedName, instance)
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
	reqLogger = reqLogger.WithValues("username", instance.Spec.Username)

	config, err := commonconfig.GetToolchainConfig(r.Client)
	if err != nil {
		return reconcile.Result{}, errs.Wrapf(err, "unable to get ToolchainConfig")
	}
	activations, activationsAnnotationPresent := instance.Annotations[toolchainv1alpha1.UserSignupActivationCounterAnnotationKey]

	// Check if the UserSignup is waiting for phone verification to be finished
	// Check also if the UserSignup doesn't have at least one activation - if it has, then it means that it's reactivated UserSignup and we should skip it here
	if states.VerificationRequired(instance) && (!activationsAnnotationPresent || activations == "0") {

		createdTime := instance.ObjectMeta.CreationTimestamp

		unverifiedThreshold := time.Now().Add(-time.Duration(config.Deactivation().UserSignupUnverifiedRetentionDays()*24) * time.Hour)

		if createdTime.Time.Before(unverifiedThreshold) {
			reqLogger.Info("Deleting UserSignup due to exceeding unverified retention period")
			return reconcile.Result{}, r.DeleteUserSignup(instance, reqLogger)
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

	// Check if the UserSignup is:
	// * either deactivated
	// * or if it is waiting for phone verification to be finished and at the same it has at least one activation stored in the annotation
	//   (in other words the UserSignup is reactivated but the phone verification hasn't been finished)
	if states.Deactivated(instance) || (states.VerificationRequired(instance) && activationsAnnotationPresent && activations != "0") {
		// Find the UserSignupComplete condition
		cond, found := condition.FindConditionByType(instance.Status.Conditions, toolchainv1alpha1.UserSignupComplete)
		if !found {
			// We cannot find the status Complete condition
			return reconcile.Result{}, nil
		}

		// If the LastTransitionTime of the deactivated status condition is older than the configured threshold,
		// then delete the UserSignup
		deactivatedThreshold := time.Now().Add(-time.Duration(config.Deactivation().UserSignupDeactivatedRetentionDays()*24) * time.Hour)

		if cond.LastTransitionTime.Time.Before(deactivatedThreshold) {
			reqLogger.Info("Deleting UserSignup due to exceeding deactivated retention period")
			return reconcile.Result{}, r.DeleteUserSignup(instance, reqLogger)
		}

		// Requeue this for reconciliation after the time has passed between the last transition time
		// and the current deletion expiry threshold
		requeueAfter := cond.LastTransitionTime.Time.Sub(deactivatedThreshold)

		// Requeue the reconciler to process this resource again after the threshold for deletion
		return reconcile.Result{
			Requeue:      true,
			RequeueAfter: requeueAfter,
		}, nil
	}

	return reconcile.Result{}, nil
}

// DeleteUserSignup deletes the specified UserSignup
func (r *Reconciler) DeleteUserSignup(userSignup *toolchainv1alpha1.UserSignup, logger logr.Logger) error {
	// before deleting the resource, we want to "remember" if the user triggered a phone verification or not,
	// based on the presence of the `toolchain.dev.openshift.com/verification-code` annotation
	_, phoneVerificationTriggered := userSignup.Annotations[toolchainv1alpha1.UserSignupVerificationCodeAnnotationKey]

	propagationPolicy := metav1.DeletePropagationForeground
	err := r.Client.Delete(context.TODO(), userSignup, &client.DeleteOptions{
		PropagationPolicy: &propagationPolicy,
	})
	if err != nil {
		return err
	}
	logger.Info("Deleted UserSignup", "name", userSignup.Name)
	// increment the appropriate counter, based whether the phone verification was triggered or not
	if phoneVerificationTriggered {
		metrics.UserSignupDeletedWithInitiatingVerificationTotal.Inc()
	} else {
		metrics.UserSignupDeletedWithoutInitiatingVerificationTotal.Inc()
	}
	logger.Info("incremented counter", "name", userSignup.Name, "phone verification triggered", phoneVerificationTriggered)
	return nil
}
