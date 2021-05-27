package usersignupcleanup

import (
	"context"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"time"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	crtCfg "github.com/codeready-toolchain/host-operator/pkg/configuration"
	"github.com/codeready-toolchain/toolchain-common/pkg/condition"
	"github.com/codeready-toolchain/toolchain-common/pkg/states"
	"github.com/go-logr/logr"
	apiv1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

type StatusUpdater func(userAcc *toolchainv1alpha1.UserSignup, message string) error

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("usersignupcleanup-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to the secondary resource UserSignup and requeue the owner UserSignup
	if err := c.Watch(&source.Kind{Type: &toolchainv1alpha1.UserSignup{}},
		&handler.EnqueueRequestForObject{},
	); err != nil {
		return err
	}

	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr manager.Manager) error {
	return add(mgr, r)
}

// Reconciler cleans up old UserSignup resources
type Reconciler struct {
	Client client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
	Config *crtCfg.Config
}

// Reconcile reads that state of the cluster for a UserSignup object and makes changes based on the state read
// and what is in the UserSignup.Spec
// Note:
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *Reconciler) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	reqLogger := r.Log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
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

	if states.VerificationRequired(instance) && !(states.Approved(instance) || instance.Spec.Approved) {

		createdTime := instance.ObjectMeta.CreationTimestamp

		unverifiedThreshold := time.Now().Add(-time.Duration(r.Config.GetUserSignupUnverifiedRetentionDays()*24) * time.Hour)

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

	if states.Deactivated(instance) {
		// Find the UserSignupComplete condition
		cond, found := condition.FindConditionByType(instance.Status.Conditions, toolchainv1alpha1.UserSignupComplete)

		if !found || cond.Status != apiv1.ConditionTrue || cond.Reason != toolchainv1alpha1.UserSignupUserDeactivatedReason {
			// We cannot find the status condition with "deactivated" reason, simply return
			return reconcile.Result{}, nil
		}

		// If the LastTransitionTime of the deactivated status condition is older than the configured threshold,
		// then delete the UserSignup

		deactivatedThreshold := time.Now().Add(-time.Duration(r.Config.GetUserSignupDeactivatedRetentionDays()*24) * time.Hour)

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
	propagationPolicy := metav1.DeletePropagationForeground
	err := r.Client.Delete(context.TODO(), userSignup, &client.DeleteOptions{
		PropagationPolicy: &propagationPolicy,
	})
	if err != nil {
		return err
	}
	logger.Info("Deleted UserSignup", "Name", userSignup.Name)
	return nil
}
