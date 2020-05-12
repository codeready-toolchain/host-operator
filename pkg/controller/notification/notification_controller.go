package notification

import (
	"bytes"
	"context"
	"fmt"
	toolchainv1alpha1 "github.com/codeready-toolchain/api/pkg/apis/toolchain/v1alpha1"
	"github.com/codeready-toolchain/host-operator/pkg/configuration"
	"github.com/codeready-toolchain/host-operator/pkg/templates/notificationtemplates"
	commonCondition "github.com/codeready-toolchain/toolchain-common/pkg/condition"
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
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"sigs.k8s.io/controller-runtime/pkg/source"
	"text/template"
)

const (
	NotificationInvalidTemplateNameReason       = "InvalidTemplateName"
	NotificationInvalidTemplateDefinitionReason = "InvalidTemplateDefinition"
	NotificationFailedToDeliverReason           = "FailedToDeliver"
)

var log = logf.Log.WithName("controller_notification")

type StatusUpdater func(notification *toolchainv1alpha1.Notification, message string) error

type NotificationContext struct {
	UserID      string
	FirstName   string
	LastName    string
	Email       string
	CompanyName string
}

// Add creates a new Notification Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager, _ *configuration.Registry) error {
	return add(mgr, newReconciler(mgr))
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
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileNotification{client: mgr.GetClient(), scheme: mgr.GetScheme()}
}

var _ reconcile.Reconciler = &ReconcileNotification{}

// ReconcileNotification reconciles a Notification object
type ReconcileNotification struct {
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the apiserver
	client client.Client
	scheme *runtime.Scheme
}

func (r *ReconcileNotification) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	reqLogger := log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	reqLogger.Info("Reconciling Notification")

	// Fetch the Notification instance
	instance := &toolchainv1alpha1.Notification{}
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

	// Lookup the notification template
	template, found, err := notificationtemplates.GetNotificationTemplate(instance.Spec.Template)
	if err != nil {
		if errors.IsNotFound(err) {
			return reconcile.Result{}, r.wrapErrorWithStatusUpdate(reqLogger, instance, r.setStatusInvalidTemplate, err,
				"Failed to find template %s", instance.Spec.Template)
		}
		return reconcile.Result{}, err
	}

	// If the template was not found, set the status to invalid template
	if !found {
		return reconcile.Result{}, r.setStatusInvalidTemplate(instance, fmt.Sprintf("invalid template [%s]",
			instance.Spec.Template))
	}

	// Generate the content of the notification
	generated, err := r.generateNotificationContent(instance.Spec.UserID, instance.Namespace, template)
	if err != nil {
		return reconcile.Result{}, r.wrapErrorWithStatusUpdate(reqLogger, instance, r.setStatusInvalidTemplateDefinition,
			err, "failed to generate template %s", instance.Spec.Template)
	}

	// Deliver the notification
	err = r.deliverNotification(template.Subject, generated)
	if err != nil {
		return reconcile.Result{}, r.wrapErrorWithStatusUpdate(reqLogger, instance, r.setStatusFailedToDeliver,
			err, "failed to deliver notification")
	}

	return reconcile.Result{}, nil
}

func (r *ReconcileNotification) deliverNotification(subject, content string) error {

	// TODO deliver the notification

	return nil
}

func (r *ReconcileNotification) generateNotificationContent(userID, namespace string,
	templ *notificationtemplates.NotificationTemplate) (string, error) {

	context, err := r.createNotificationContext(userID, namespace)
	if err != nil {
		return "", err
	}

	tmpl, err := template.New("notification").Parse(templ.Content)
	if err != nil {
		return "", err
	}

	var buf bytes.Buffer

	err = tmpl.Execute(&buf, context)
	if err != nil {
		return "", err
	}

	return buf.String(), nil
}

func (r *ReconcileNotification) createNotificationContext(userID, namespace string) (*NotificationContext, error) {
	// Lookup the UserSignup resource with the specified userID
	instance := &toolchainv1alpha1.UserSignup{}
	err := r.client.Get(context.TODO(), types.NamespacedName{
		Namespace: namespace,
		Name:      userID,
	}, instance)

	if err != nil {
		return nil, err
	}

	notificationCtx := &NotificationContext{
		UserID:      userID,
		FirstName:   instance.Spec.GivenName,
		LastName:    instance.Spec.FamilyName,
		CompanyName: instance.Spec.Company,
	}

	if emailLbl, exists := instance.Annotations[toolchainv1alpha1.UserSignupUserEmailAnnotationKey]; exists {
		notificationCtx.Email = emailLbl
	}

	return notificationCtx, nil
}

func (r *ReconcileNotification) setStatusInvalidTemplate(notification *toolchainv1alpha1.Notification, message string) error {
	return r.updateStatusConditions(
		notification,
		toolchainv1alpha1.Condition{
			Type:    toolchainv1alpha1.NotificationDelivered,
			Status:  corev1.ConditionFalse,
			Reason:  NotificationInvalidTemplateNameReason,
			Message: message,
		})
}

func (r *ReconcileNotification) setStatusInvalidTemplateDefinition(notification *toolchainv1alpha1.Notification, message string) error {
	return r.updateStatusConditions(
		notification,
		toolchainv1alpha1.Condition{
			Type:    toolchainv1alpha1.NotificationDelivered,
			Status:  corev1.ConditionFalse,
			Reason:  NotificationInvalidTemplateDefinitionReason,
			Message: message,
		})
}

func (r *ReconcileNotification) setStatusFailedToDeliver(notification *toolchainv1alpha1.Notification, message string) error {
	return r.updateStatusConditions(
		notification,
		toolchainv1alpha1.Condition{
			Type:    toolchainv1alpha1.NotificationDelivered,
			Status:  corev1.ConditionFalse,
			Reason:  NotificationFailedToDeliverReason,
			Message: message,
		})
}

func (r *ReconcileNotification) updateStatus(logger logr.Logger, notification *toolchainv1alpha1.Notification,
	statusUpdater StatusUpdater) error {

	if err := statusUpdater(notification, ""); err != nil {
		logger.Error(err, "status update failed")
		return err
	}

	return nil
}

// wrapErrorWithStatusUpdate wraps the error and update the Notification status. If the update fails then the error is logged.
func (r *ReconcileNotification) wrapErrorWithStatusUpdate(logger logr.Logger, notification *toolchainv1alpha1.Notification,
	statusUpdater StatusUpdater, err error, format string, args ...interface{}) error {
	if err == nil {
		return nil
	}
	if err := statusUpdater(notification, err.Error()); err != nil {
		logger.Error(err, "Error updating Notification status")
	}
	return errs.Wrapf(err, format, args...)
}

func (r *ReconcileNotification) updateStatusConditions(notification *toolchainv1alpha1.Notification, newConditions ...toolchainv1alpha1.Condition) error {
	var updated bool
	notification.Status.Conditions, updated = commonCondition.AddOrUpdateStatusConditions(notification.Status.Conditions, newConditions...)
	if !updated {
		// Nothing changed
		return nil
	}
	return r.client.Status().Update(context.TODO(), notification)
}
