package deactivation

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/codeready-toolchain/host-operator/pkg/controller/usersignup"

	"github.com/codeready-toolchain/host-operator/pkg/templates/notificationtemplates"
	"github.com/codeready-toolchain/toolchain-common/pkg/condition"
	"github.com/go-logr/logr"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	coputil "github.com/redhat-cop/operator-utils/pkg/util"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/pkg/apis/toolchain/v1alpha1"
	"github.com/codeready-toolchain/host-operator/pkg/configuration"
	"github.com/codeready-toolchain/host-operator/pkg/metrics"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

var log = logf.Log.WithName("controller_deactivation")

// Add creates a new Deactivation Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager, config *configuration.Config) error {
	return add(mgr, newReconciler(mgr, config))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager, cfg *configuration.Config) reconcile.Reconciler {
	return &ReconcileDeactivation{client: mgr.GetClient(), scheme: mgr.GetScheme(), config: cfg}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("deactivation-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to primary resource MasterUserRecord
	err = c.Watch(&source.Kind{Type: &toolchainv1alpha1.MasterUserRecord{}}, &handler.EnqueueRequestForObject{}, CreateAndUpdateOnlyPredicate{})
	if err != nil {
		return err
	}

	return nil
}

var _ reconcile.Reconciler = &ReconcileDeactivation{}

// ReconcileDeactivation reconciles a Deactivation object
type ReconcileDeactivation struct {
	*usersignup.StatusUpdater
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the apiserver
	client client.Client
	scheme *runtime.Scheme
	config *configuration.Config
}

// Reconcile reads the state of the cluster for a MUR object and determines whether to trigger deactivation or requeue based on its current status
// Note:
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcileDeactivation) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	logger := log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	logger.Info("Reconciling Deactivation")

	// Fetch the MasterUserRecord instance
	mur := &toolchainv1alpha1.MasterUserRecord{}
	err := r.client.Get(context.TODO(), request.NamespacedName, mur)
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
	if err := r.client.Get(context.TODO(), types.NamespacedName{
		Namespace: mur.Namespace,
		Name:      mur.Labels[toolchainv1alpha1.MasterUserRecordOwnerLabelKey],
	}, usersignup); err != nil {
		// Error getting usersignup - requeue the request.
		return reconcile.Result{}, err
	}

	// Check the domain exclusion list, if the user's email matches then they cannot be automatically deactivated
	if emailLbl, exists := usersignup.Annotations[toolchainv1alpha1.UserSignupUserEmailAnnotationKey]; exists {
		for _, domain := range r.config.GetDeactivationDomainsExcludedList() {
			if strings.HasSuffix(emailLbl, domain) {
				logger.Info("user cannot be automatically deactivated because they belong to the exclusion list", "domain", domain)
				return reconcile.Result{}, nil
			}
		}
	}

	if len(mur.Spec.UserAccounts) == 0 {
		err = fmt.Errorf("cannot determine deactivation timeout period because the mur has no associated user accounts")
		logger.Error(err, "failed to process deactivation")
		return reconcile.Result{}, err
	}

	// The tier is defined as part of its user account, since murs can in theory have multiple user accounts we will only consider the first one
	account := mur.Spec.UserAccounts[0]

	// Get the tier associated with the user account, we'll observe the deactivation timeout period from the tier spec
	nsTemplateTier := &toolchainv1alpha1.NSTemplateTier{}
	tierName := types.NamespacedName{Namespace: request.Namespace, Name: account.Spec.NSTemplateSet.TierName}
	if err := r.client.Get(context.TODO(), tierName, nsTemplateTier); err != nil {
		logger.Error(err, "unable to get NSTemplateTier", "name", account.Spec.NSTemplateSet.TierName)
		return reconcile.Result{}, err
	}

	// If the deactivation timeout is 0 then users that belong to this tier should not be automatically deactivated
	deactivationTimeoutDays := nsTemplateTier.Spec.DeactivationTimeoutDays
	if deactivationTimeoutDays == 0 {
		logger.Info("User belongs to a tier that does not have a deactivation timeout. The user will not be automatically deactivated")
		// Users belonging to this tier will not be auto deactivated, no need to requeue.
		return reconcile.Result{}, nil
	}

	deactivationTimeout := time.Duration(deactivationTimeoutDays*24) * time.Hour

	logger.Info("user account time values", "deactivation timeout duration", deactivationTimeout, "provisionedTimestamp", provisionedTimestamp)

	timeSinceProvisioned := time.Since(provisionedTimestamp.Time)

	deactivatingNotificationTimeout := time.Duration((deactivationTimeoutDays - r.config.GetUserSignupDeactivatingNotificationDays()) * 24)

	if timeSinceProvisioned < deactivatingNotificationTimeout {
		// It is not yet time to send the deactivating notification so requeue until it will be time to send it
		requeueAfterTimeToNotify := deactivatingNotificationTimeout - timeSinceProvisioned
		logger.Info("requeueing request", "RequeueAfter", requeueAfterTimeToNotify, "Expected deactivating notification date/time", time.Now().Add(requeueAfterTimeToNotify).String())
		return reconcile.Result{RequeueAfter: requeueAfterTimeToNotify}, nil
	}

	if condition.IsNotTrue(usersignup.Status.Conditions, toolchainv1alpha1.UserSignupUserDeactivatingNotificationCreated) {
		if err := r.sendDeactivatingNotification(logger, usersignup); err != nil {
			logger.Error(err, "Failed to create user deactivating notification")

			// set the failed to create notification status condition
			return reconcile.Result{}, r.WrapErrorWithStatusUpdate(logger, usersignup, r.SetStatusDeactivatingNotificationCreationFailed, err, "Failed to create user deactivating notification")
		}

		if err := r.UpdateStatus(logger, usersignup, r.SetStatusDeactivatingNotificationCreated); err != nil {
			logger.Error(err, "Failed to update notification created status")
			return reconcile.Result{}, err
		}
	}

	if timeSinceProvisioned < deactivationTimeout {
		// It is not yet time to deactivate so requeue when it will be
		requeueAfterExpired := deactivationTimeout - timeSinceProvisioned
		logger.Info("requeueing request", "RequeueAfter", requeueAfterExpired, "Expected deactivation date/time", time.Now().Add(requeueAfterExpired).String())
		return reconcile.Result{RequeueAfter: requeueAfterExpired}, nil
	}

	// Deactivate the user
	if usersignup.Spec.Deactivated {
		// The UserSignup is already set for deactivation, nothing left to do
		return reconcile.Result{}, nil
	}
	usersignup.Spec.Deactivated = true

	logger.Info("deactivating the user")
	if err := r.client.Update(context.TODO(), usersignup); err != nil {
		logger.Error(err, "failed to update usersignup")
		return reconcile.Result{}, err
	}

	metrics.UserSignupAutoDeactivatedTotal.Inc()

	return reconcile.Result{}, nil
}

func (r *ReconcileDeactivation) sendDeactivatingNotification(logger logr.Logger, userSignup *toolchainv1alpha1.UserSignup) error {
	notification := &toolchainv1alpha1.Notification{
		ObjectMeta: v1.ObjectMeta{
			GenerateName: fmt.Sprintf("%s-%s-", userSignup.Status.CompliantUsername, toolchainv1alpha1.NotificationTypeDeactivating),
			Namespace:    userSignup.Namespace,
			Labels: map[string]string{
				// NotificationUserNameLabelKey is only used for easy lookup for debugging and e2e tests
				toolchainv1alpha1.NotificationUserNameLabelKey: userSignup.Status.CompliantUsername,
				// NotificationTypeLabelKey is only used for easy lookup for debugging and e2e tests
				toolchainv1alpha1.NotificationTypeLabelKey: toolchainv1alpha1.NotificationTypeDeactivating,
			},
		},
		Spec: toolchainv1alpha1.NotificationSpec{
			UserID:   userSignup.Name,
			Template: notificationtemplates.UserDeactivating.Name,
		},
	}

	if err := controllerutil.SetControllerReference(userSignup, notification, r.scheme); err != nil {
		logger.Error(err, "Failed to set owner reference for deactivating notification resource")
		return err
	}

	if err := r.client.Create(context.TODO(), notification); err != nil {
		logger.Error(err, "Failed to create deactivating notification resource")
		return err
	}

	logger.Info("Deactivating notification resource created")
	return nil
}
