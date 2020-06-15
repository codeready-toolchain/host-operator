package notification

import (
	"context"
	"time"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/pkg/apis/toolchain/v1alpha1"
	"github.com/codeready-toolchain/host-operator/pkg/configuration"
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
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

var log = logf.Log.WithName("controller_notification")

// Add creates a new Notification Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager, config *configuration.Config) error {
	return add(mgr, newReconciler(mgr, config))
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("notification-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to primary resource Notification
	err = c.Watch(&source.Kind{Type: &toolchainv1alpha1.Notification{}},
		&handler.EnqueueRequestForObject{},
		predicate.GenerationChangedPredicate{})
	if err != nil {
		return err
	}

	return nil
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager, config *configuration.Config) reconcile.Reconciler {
	return &ReconcileNotification{client: mgr.GetClient(), scheme: mgr.GetScheme(), config: config}
}

var _ reconcile.Reconciler = &ReconcileNotification{}

// ReconcileNotification reconciles a Notification object
type ReconcileNotification struct {
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the apiserver
	client client.Client
	scheme *runtime.Scheme
	config *configuration.Config
}

func (r *ReconcileNotification) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	reqLogger := log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	reqLogger.Info("Reconciling Notification")

	// Fetch the Notification instance
	notification := &toolchainv1alpha1.Notification{}
	err := r.client.Get(context.TODO(), request.NamespacedName, notification)
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

	// if is delivered, then check when status was changed and delete it if the requested duration has passed
	completeCond, found := condition.FindConditionByType(notification.Status.Conditions, toolchainv1alpha1.NotificationDelivered)
	if found && completeCond.Status == corev1.ConditionTrue {
		deleted, requeueAfter, err := r.checkTransitionTimeAndDelete(reqLogger, notification, completeCond)
		if deleted || err != nil {
			return reconcile.Result{}, err
		}
		return reconcile.Result{
			Requeue:      true,
			RequeueAfter: requeueAfter,
		}, nil
	}

	reqLogger.Info("Notification has been delivered")
	return reconcile.Result{
		Requeue:      true,
		RequeueAfter: r.config.GetDurationBeforeNotificationDeletion(),
	}, r.setStatusNotificationDelivered(notification)
}

func (r *ReconcileNotification) checkTransitionTimeAndDelete(log logr.Logger, notification *toolchainv1alpha1.Notification, completeCond toolchainv1alpha1.Condition) (bool, time.Duration, error) {
	log.Info("the Notification is delivered so we can deal with its deletion")
	timeSinceCompletion := time.Since(completeCond.LastTransitionTime.Time)

	if timeSinceCompletion >= r.config.GetDurationBeforeNotificationDeletion() {
		log.Info("the Notification has been delivered for a longer time than the 'durationBeforeNotificationDeletion', so it's ready to be deleted",
			"durationBeforeNotificationDeletion", r.config.GetDurationBeforeNotificationDeletion().String())
		if err := r.client.Delete(context.TODO(), notification, &client.DeleteOptions{}); err != nil {
			return false, 0, errs.Wrapf(err, "unable to delete Notification object '%s'", notification.Name)
		}
		return true, 0, nil
	}
	diff := r.config.GetDurationBeforeNotificationDeletion() - timeSinceCompletion
	log.Info("the Notification has been completed for shorter time than 'durationBeforeNotificationDeletion', so it's going to be reconciled again",
		"durationBeforeNotificationDeletion", r.config.GetDurationBeforeNotificationDeletion().String(), "reconcileAfter", diff.String())
	return false, diff, nil
}

func (r *ReconcileNotification) updateStatusConditions(notification *toolchainv1alpha1.Notification, newConditions ...toolchainv1alpha1.Condition) error {
	var updated bool
	notification.Status.Conditions, updated = condition.AddOrUpdateStatusConditions(notification.Status.Conditions, newConditions...)
	if !updated {
		// Nothing changed
		return nil
	}
	return r.client.Status().Update(context.TODO(), notification)
}

func (r *ReconcileNotification) setStatusNotificationDelivered(notification *toolchainv1alpha1.Notification) error {
	return r.updateStatusConditions(
		notification,
		toolchainv1alpha1.Condition{
			Type:   toolchainv1alpha1.NotificationDelivered,
			Status: corev1.ConditionTrue,
			Reason: "notification has been delivered to the user",
		})
}
