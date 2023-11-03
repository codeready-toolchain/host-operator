package notification

import (
	"context"
	"time"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/host-operator/controllers/toolchainconfig"
	"github.com/codeready-toolchain/toolchain-common/pkg/condition"

	errs "github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr manager.Manager, config toolchainconfig.ToolchainConfig) error {
	factory := NewNotificationDeliveryServiceFactory(mgr.GetClient(), toolchainconfig.DeliveryServiceFactoryConfig{ToolchainConfig: config})
	svc, err := factory.CreateNotificationDeliveryService()
	if err != nil {
		return err
	}
	r.deliveryService = svc

	return ctrl.NewControllerManagedBy(mgr).
		For(&toolchainv1alpha1.Notification{}, builder.WithPredicates(predicate.GenerationChangedPredicate{})).
		Complete(r)
}

// Reconciler reconciles a Notification object
type Reconciler struct {
	Client          runtimeclient.Client
	Scheme          *runtime.Scheme
	deliveryService DeliveryService
}

//+kubebuilder:rbac:groups=toolchain.dev.openshift.com,resources=notifications,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=toolchain.dev.openshift.com,resources=notifications/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=toolchain.dev.openshift.com,resources=notifications/finalizers,verbs=update

func (r *Reconciler) Reconcile(ctx context.Context, request ctrl.Request) (ctrl.Result, error) {
	reqLogger := log.FromContext(ctx)
	reqLogger.Info("Reconciling Notification")

	// Fetch the Notification instance
	notification := &toolchainv1alpha1.Notification{}
	err := r.Client.Get(ctx, request.NamespacedName, notification)
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

	config, err := toolchainconfig.GetToolchainConfig(r.Client)
	if err != nil {
		return reconcile.Result{}, errs.Wrapf(err, "unable to get ToolchainConfig")
	}

	// if is sent, then check when status was changed and delete it if the requested duration has passed
	completeCond, found := condition.FindConditionByType(notification.Status.Conditions, toolchainv1alpha1.NotificationSent)
	if found && completeCond.Status == corev1.ConditionTrue {
		deleted, requeueAfter, err := r.checkTransitionTimeAndDelete(ctx, config.Notifications().DurationBeforeNotificationDeletion(), notification, completeCond)
		if deleted {
			return reconcile.Result{}, err
		}
		if err != nil {
			return reconcile.Result{}, r.wrapErrorWithStatusUpdate(ctx, notification, r.setStatusNotificationDeletionFailed, err, "failed to delete notification")
		}
		return reconcile.Result{
			Requeue:      true,
			RequeueAfter: requeueAfter,
		}, nil
	}

	// if the environment is set to e2e do not attempt sending via mailgun
	if config.Environment() != "e2e-tests" {
		// get the notification environment
		templateSetName := config.Notifications().TemplateSetName()
		// Send the notification via the configured delivery service
		err = r.deliveryService.Send(notification, templateSetName)
		if err != nil {
			reqLogger.Error(err, "delivery service failed to send notification",
				"notification spec", notification.Spec,
			)

			return reconcile.Result{}, r.wrapErrorWithStatusUpdate(ctx, notification,
				r.setStatusNotificationDeliveryError, err, "failed to send notification")
		}
		reqLogger.Info("Notification has been sent")
	} else {
		reqLogger.Info("Notification has been skipped")
	}

	return reconcile.Result{
		Requeue:      true,
		RequeueAfter: config.Notifications().DurationBeforeNotificationDeletion(),
	}, r.updateStatus(ctx, notification, r.setStatusNotificationSent)
}

// checkTransitionTimeAndDelete checks if the last transition time has surpassed
// the duration before the notification should be deleted. If so, the notification is deleted.
// Returns bool indicating if the notification was deleted, the time before the notification
// can be deleted and error
func (r *Reconciler) checkTransitionTimeAndDelete(ctx context.Context, durationBeforeNotificationDeletion time.Duration, notification *toolchainv1alpha1.Notification,
	completeCond toolchainv1alpha1.Condition) (bool, time.Duration, error) {
	logger := log.FromContext(ctx)

	logger.Info("the Notification is sent so we can deal with its deletion")
	timeSinceCompletion := time.Since(completeCond.LastTransitionTime.Time)

	if timeSinceCompletion >= durationBeforeNotificationDeletion {
		logger.Info("the Notification has been sent for a longer time than the 'durationBeforeNotificationDeletion', so it's ready to be deleted",
			"durationBeforeNotificationDeletion", durationBeforeNotificationDeletion.String())
		if err := r.Client.Delete(ctx, notification, &runtimeclient.DeleteOptions{}); err != nil {
			return false, 0, errs.Wrapf(err, "unable to delete Notification object '%s'", notification.Name)
		}
		return true, 0, nil
	}
	diff := durationBeforeNotificationDeletion - timeSinceCompletion
	logger.Info("the Notification has been completed for shorter time than 'durationBeforeNotificationDeletion', so it's going to be reconciled again",
		"durationBeforeNotificationDeletion", durationBeforeNotificationDeletion.String(), "reconcileAfter", diff.String())
	return false, diff, nil
}

type statusUpdater func(ctx context.Context, notification *toolchainv1alpha1.Notification, message string) error

func (r *Reconciler) wrapErrorWithStatusUpdate(ctx context.Context, notification *toolchainv1alpha1.Notification,
	statusUpdater statusUpdater, err error, format string, args ...interface{}) error {
	if err == nil {
		return nil
	}
	if err := statusUpdater(ctx, notification, err.Error()); err != nil {
		log.FromContext(ctx).Error(err, "status update failed")
	}
	return errs.Wrapf(err, format, args...)
}

func (r *Reconciler) updateStatusConditions(ctx context.Context, notification *toolchainv1alpha1.Notification, newConditions ...toolchainv1alpha1.Condition) error {
	var updated bool
	notification.Status.Conditions, updated = condition.AddOrUpdateStatusConditions(notification.Status.Conditions, newConditions...)
	if !updated {
		// Nothing changed
		return nil
	}
	return r.Client.Status().Update(ctx, notification)
}

func (r *Reconciler) setStatusNotificationDeletionFailed(ctx context.Context, notification *toolchainv1alpha1.Notification, msg string) error {
	return r.updateStatusConditions(
		ctx,
		notification,
		toolchainv1alpha1.Condition{
			Type:    toolchainv1alpha1.NotificationDeletionError,
			Status:  corev1.ConditionTrue,
			Reason:  toolchainv1alpha1.NotificationDeletionErrorReason,
			Message: msg,
		})
}

func (r *Reconciler) setStatusNotificationDeliveryError(ctx context.Context, notification *toolchainv1alpha1.Notification, msg string) error {
	return r.updateStatusConditions(
		ctx,
		notification,
		toolchainv1alpha1.Condition{
			Type:    toolchainv1alpha1.NotificationSent,
			Status:  corev1.ConditionFalse,
			Reason:  toolchainv1alpha1.NotificationDeliveryErrorReason,
			Message: msg,
		})
}

func (r *Reconciler) setStatusNotificationSent(ctx context.Context, notification *toolchainv1alpha1.Notification, msg string) error {
	return r.updateStatusConditions(
		ctx,
		notification,
		toolchainv1alpha1.Condition{
			Type:    toolchainv1alpha1.NotificationSent,
			Status:  corev1.ConditionTrue,
			Reason:  toolchainv1alpha1.NotificationSentReason,
			Message: msg,
		})
}

func (r *Reconciler) updateStatus(ctx context.Context, notification *toolchainv1alpha1.Notification,
	statusUpdater statusUpdater) error {

	if err := statusUpdater(ctx, notification, ""); err != nil {
		log.FromContext(ctx).Error(err, "status update failed")
		return err
	}

	return nil
}
