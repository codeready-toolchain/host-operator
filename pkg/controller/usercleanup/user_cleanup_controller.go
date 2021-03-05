package usercleanup

import (
	"context"
	toolchainv1alpha1 "github.com/codeready-toolchain/api/pkg/apis/toolchain/v1alpha1"
	crtCfg "github.com/codeready-toolchain/host-operator/pkg/configuration"
	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
	"time"
)

var log = logf.Log.WithName("controller_usercleanup")

type StatusUpdater func(userAcc *toolchainv1alpha1.UserSignup, message string) error

// Add creates a new UserCleanup Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager, crtConfig *crtCfg.Config) error {
	return add(mgr, newReconciler(mgr, crtConfig))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager, crtConfig *crtCfg.Config) reconcile.Reconciler {
	return &ReconcileUserCleanup{
		client:    mgr.GetClient(),
		scheme:    mgr.GetScheme(),
		crtConfig: crtConfig,
	}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("usercleanup-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to the secondary resource UserSignup and requeue the owner UserSignup
	if err := c.Watch(
		&source.Kind{Type: &toolchainv1alpha1.UserSignup{}},
		&handler.EnqueueRequestForOwner{
			IsController: true,
			OwnerType:    &toolchainv1alpha1.UserSignup{},
		}); err != nil {
		return err
	}

	return nil
}

var _ reconcile.Reconciler = &ReconcileUserCleanup{}

// ReconcileUserCleanup cleans up old UserSignup resources
type ReconcileUserCleanup struct {
	client    client.Client
	scheme    *runtime.Scheme
	crtConfig *crtCfg.Config
}

// Reconcile reads that state of the cluster for a UserSignup object and makes changes based on the state read
// and what is in the UserSignup.Spec
// Note:
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcileUserCleanup) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	reqLogger := log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	reqLogger.Info("Reconciling UserSignup")

	// Fetch the UserSignup instance
	instance := &toolchainv1alpha1.UserSignup{}
	err := r.client.Get(context.TODO(), request.NamespacedName, instance)
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

	if instance.Spec.Deactivated {
		// Find the UserSignupComplete condition
		var deactivatedStatusCondition *toolchainv1alpha1.Condition
		for _, cond := range instance.Status.Conditions {
			if cond.Type == toolchainv1alpha1.UserSignupComplete &&
				cond.Reason == toolchainv1alpha1.UserSignupUserDeactivatedReason {
				deactivatedStatusCondition = &cond
				break
			}
		}

		if deactivatedStatusCondition == nil {
			// We cannot find the status condition
			// TODO how should we handle this if we can't find the condition?
		}

		// If the LastTransitionTime of the deactivated status condition is older than the configured threshold,
		// then delete the UserSignup

		// TODO read from configuration
		days := 14

		threshold := v1.Time{time.Now().Add(time.Duration(days*24) * time.Hour)}

		if deactivatedStatusCondition.LastTransitionTime.Before(&threshold) {
			return reconcile.Result{}, r.DeleteUserSignup(instance, reqLogger)
		}

		// Requeue the reconciler to process this resource again after the threshold for deletion
		return reconcile.Result{
			Requeue:      true,
			RequeueAfter: 0,
		}, nil
	}

	return reconcile.Result{}, nil
}

// DeleteUserSignup deletes the specified UserSignup
func (r *ReconcileUserCleanup) DeleteUserSignup(userSignup *toolchainv1alpha1.UserSignup, logger logr.Logger) error {

	err := r.client.Delete(context.TODO(), userSignup)
	if err != nil {
		return err
	}
	logger.Info("Deleted UserSignup", "Name", userSignup.Name)
	return nil
}
