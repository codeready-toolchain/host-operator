package usersignup

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"

	"github.com/codeready-toolchain/toolchain-common/pkg/states"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/host-operator/controllers/usersignup/unapproved"
	crtCfg "github.com/codeready-toolchain/host-operator/pkg/configuration"
	"github.com/codeready-toolchain/host-operator/pkg/counter"
	"github.com/codeready-toolchain/host-operator/pkg/metrics"
	"github.com/codeready-toolchain/host-operator/pkg/templates/notificationtemplates"
	"github.com/codeready-toolchain/toolchain-common/pkg/cluster"
	"github.com/codeready-toolchain/toolchain-common/pkg/condition"
	"github.com/codeready-toolchain/toolchain-common/pkg/usersignup"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/validation"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

type StatusUpdaterFunc func(userAcc *toolchainv1alpha1.UserSignup, message string) error

const defaultTierName = "base"

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("usersignup-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to primary resource UserSignup
	if err := c.Watch(
		&source.Kind{Type: &toolchainv1alpha1.UserSignup{}},
		&handler.EnqueueRequestForObject{},
		UserSignupChangedPredicate{}); err != nil {
		return err
	}

	// Watch for changes to the secondary resource MasterUserRecord and requeue the owner UserSignup
	if err := c.Watch(
		&source.Kind{Type: &toolchainv1alpha1.MasterUserRecord{}},
		&handler.EnqueueRequestForOwner{
			IsController: true,
			OwnerType:    &toolchainv1alpha1.UserSignup{},
		}); err != nil {
		return err
	}

	if err := c.Watch(
		&source.Kind{Type: &toolchainv1alpha1.BannedUser{}},
		&handler.EnqueueRequestsFromMapFunc{
			ToRequests: BannedUserToUserSignupMapper{client: mgr.GetClient()},
		}); err != nil {
		return err
	}

	mapToOldestUnapproved := &handler.EnqueueRequestsFromMapFunc{
		ToRequests: unapproved.NewUserSignupMapper(mgr.GetClient()),
	}
	whenAutomaticApprovalIsEnabled := &OnlyWhenAutomaticApprovalIsEnabled{
		client: mgr.GetClient(),
	}

	// Watch for updates in ToolchainStatus CR to check if there is any member cluster with free capacity
	if err := c.Watch(
		&source.Kind{Type: &toolchainv1alpha1.ToolchainStatus{}},
		mapToOldestUnapproved,
		whenAutomaticApprovalIsEnabled); err != nil {
		return err
	}

	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr manager.Manager) error {
	return add(mgr, r)
}

// Reconciler reconciles a UserSignup object
type Reconciler struct {
	*StatusUpdater
	Log               logr.Logger
	Scheme            *runtime.Scheme
	CrtConfig         *crtCfg.Config
	GetMemberClusters cluster.GetMemberClustersFunc
}

//+kubebuilder:rbac:groups=toolchain.dev.openshift.com,resources=usersignups,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=toolchain.dev.openshift.com,resources=usersignups/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=toolchain.dev.openshift.com,resources=usersignups/finalizers,verbs=update

//+kubebuilder:rbac:groups=toolchain.dev.openshift.com,resources=bannedusers,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=toolchain.dev.openshift.com,resources=bannedusers/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=toolchain.dev.openshift.com,resources=bannedusers/finalizers,verbs=update

// Reconcile reads that state of the cluster for a UserSignup object and makes changes based on the state read
// and what is in the UserSignup.Spec
// Note:
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *Reconciler) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	logger := r.Log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	logger.Info("Reconciling UserSignup")

	// Fetch the UserSignup instance
	userSignup := &toolchainv1alpha1.UserSignup{}
	err := r.Client.Get(context.TODO(), request.NamespacedName, userSignup)
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
	logger = logger.WithValues("username", userSignup.Spec.Username)

	if userSignup.Labels[toolchainv1alpha1.UserSignupStateLabelKey] == "" {
		if err := r.setStateLabel(logger, userSignup, toolchainv1alpha1.UserSignupStateLabelValueNotReady); err != nil {
			return reconcile.Result{}, err
		}
	}

	banned, err := r.isUserBanned(logger, userSignup)
	if err != nil {
		return reconcile.Result{}, err
	}

	// If the usersignup is not banned and not deactivated then ensure the deactivated notification status is set to false.
	// This is especially important for cases when a user is deactivated and then reactivated because the status is used to
	// trigger sending of the notification. If a user is reactivated a notification should be sent to the user again.

	if !banned && !states.Deactivated(userSignup) {
		if err := r.updateStatus(logger, userSignup, r.setStatusDeactivationNotificationUserIsActive); err != nil {
			return reconcile.Result{}, err
		}
	}

	// If the usersignup is not banned and not within the pre-deactivation period then ensure the deactivation notification
	// status is set to false. This is especially important for cases when a user is deactivated and then reactivated
	// because the status is used to trigger sending of the notification. If a user is reactivated a notification should
	// be sent to the user again.
	if !banned && !states.Deactivating(userSignup) && !states.Deactivated(userSignup) {
		if err := r.updateStatus(logger, userSignup, r.setStatusDeactivatingNotificationNotInPreDeactivation); err != nil {
			return reconcile.Result{}, err
		}
	}

	if states.Deactivating(userSignup) && condition.IsNotTrue(userSignup.Status.Conditions,
		toolchainv1alpha1.UserSignupUserDeactivatingNotificationCreated) {

		if err := r.sendDeactivatingNotification(logger, userSignup); err != nil {
			logger.Error(err, "Failed to create user deactivating notification")

			// set the failed to create notification status condition
			return reconcile.Result{}, r.wrapErrorWithStatusUpdate(logger, userSignup,
				r.setStatusDeactivatingNotificationCreationFailed, err, "Failed to create user deactivating notification")
		}

		if err := r.updateStatus(logger, userSignup, r.setStatusDeactivatingNotificationCreated); err != nil {
			logger.Error(err, "Failed to update notification created status")
			return reconcile.Result{}, err
		}
	}

	if exists, err := r.checkIfMurAlreadyExists(logger, userSignup, banned); exists || err != nil {
		return reconcile.Result{}, err
	}

	// If there is no MasterUserRecord created, yet the UserSignup is Banned, simply set the status
	// and return
	if banned {
		// if the UserSignup doesn't have the state=banned label set, then update it
		if err := r.setStateLabel(logger, userSignup, toolchainv1alpha1.UserSignupStateLabelValueBanned); err != nil {
			return reconcile.Result{}, err
		}

		return reconcile.Result{}, r.updateStatus(logger, userSignup, r.setStatusBanned)
	}

	// If there is no MasterUserRecord created, yet the UserSignup is marked as Deactivated, set the status,
	// send a notification to the user, and return
	if states.Deactivated(userSignup) {
		// if the UserSignup doesn't have the state=deactivated label set, then update it
		if err := r.setStateLabel(logger, userSignup, toolchainv1alpha1.UserSignupStateLabelValueDeactivated); err != nil {
			return reconcile.Result{}, err
		}
		if condition.IsNotTrue(userSignup.Status.Conditions, toolchainv1alpha1.UserSignupUserDeactivatedNotificationCreated) {
			if err := r.sendDeactivatedNotification(logger, userSignup); err != nil {
				logger.Error(err, "Failed to create user deactivation notification")

				// set the failed to create notification status condition
				return reconcile.Result{}, r.wrapErrorWithStatusUpdate(logger, userSignup, r.setStatusDeactivationNotificationCreationFailed, err, "Failed to create user deactivation notification")
			}

			if err := r.updateStatus(logger, userSignup, r.setStatusDeactivationNotificationCreated); err != nil {
				logger.Error(err, "Failed to update notification created status")
				return reconcile.Result{}, err
			}
		}
		return reconcile.Result{}, r.updateStatus(logger, userSignup, r.setStatusDeactivated)
	}

	return reconcile.Result{}, r.ensureNewMurIfApproved(logger, userSignup)
}

// Is the user banned? To determine this we query the BannedUser resource for any matching entries.  The query
// is based on the user's emailHash value - if there is a match, and the e-mail addresses are equal, then the
// user is banned.
func (r *Reconciler) isUserBanned(reqLogger logr.Logger, userSignup *toolchainv1alpha1.UserSignup) (bool, error) {
	banned := false
	// Lookup the user email annotation
	if emailLbl, exists := userSignup.Annotations[toolchainv1alpha1.UserSignupUserEmailAnnotationKey]; exists {

		// Lookup the email hash label
		if emailHashLbl, exists := userSignup.Labels[toolchainv1alpha1.UserSignupUserEmailHashLabelKey]; exists {

			labels := map[string]string{toolchainv1alpha1.BannedUserEmailHashLabelKey: emailHashLbl}
			opts := client.MatchingLabels(labels)
			bannedUserList := &toolchainv1alpha1.BannedUserList{}

			// Query BannedUser for resources that match the same email hash
			if err := r.Client.List(context.TODO(), bannedUserList, opts); err != nil {
				return false, r.wrapErrorWithStatusUpdate(reqLogger, userSignup, r.setStatusFailedToReadBannedUsers, err, "Failed to query BannedUsers")
			}

			// One last check to confirm that the e-mail addresses match also (in case of the infinitesimal chance of a hash collision)
			for _, bannedUser := range bannedUserList.Items {
				if bannedUser.Spec.Email == emailLbl {
					banned = true
					break
				}
			}

			hashIsValid := validateEmailHash(emailLbl, emailHashLbl)
			if !hashIsValid {
				err := fmt.Errorf("hash is invalid")
				return banned, r.wrapErrorWithStatusUpdate(reqLogger, userSignup, r.setStatusInvalidEmailHash, err, "the email hash '%s' is invalid ", emailHashLbl)
			}
		} else {
			// If there isn't an email-hash label, then the state is invalid
			err := fmt.Errorf("missing label at usersignup")
			return banned, r.wrapErrorWithStatusUpdate(reqLogger, userSignup, r.setStatusMissingEmailHash, err,
				"the required label '%s' is not present", toolchainv1alpha1.UserSignupUserEmailHashLabelKey)
		}
	} else {
		err := fmt.Errorf("missing annotation at usersignup")
		return banned, r.wrapErrorWithStatusUpdate(reqLogger, userSignup, r.setStatusInvalidMissingUserEmailAnnotation, err,
			"the required annotation '%s' is not present", toolchainv1alpha1.UserSignupUserEmailAnnotationKey)
	}
	return banned, nil
}

// checkIfMurAlreadyExists checks if there is already a MUR for the given UserSignup.
// If there is already one then it returns 'true' as the first returned value, but before doing that it checks if the MUR should be deleted or not
// or if the MUR requires some migration changes or additional fixes.
// If no MUR for the given UserSignup is found, then it returns 'false' as the first returned value.
func (r *Reconciler) checkIfMurAlreadyExists(reqLogger logr.Logger, userSignup *toolchainv1alpha1.UserSignup,
	banned bool) (bool, error) {
	// List all MasterUserRecord resources that have an owner label equal to the UserSignup.Name
	murList := &toolchainv1alpha1.MasterUserRecordList{}
	labels := map[string]string{toolchainv1alpha1.MasterUserRecordOwnerLabelKey: userSignup.Name}
	opts := client.MatchingLabels(labels)
	if err := r.Client.List(context.TODO(), murList, opts); err != nil {
		return false, r.wrapErrorWithStatusUpdate(reqLogger, userSignup, r.setStatusInvalidMURState, err,
			"Failed to list MasterUserRecords by owner")
	}

	murs := murList.Items
	// If we found more than one MasterUserRecord, then die
	if len(murs) > 1 {
		err := fmt.Errorf("multiple matching MasterUserRecord resources found")
		return false, r.wrapErrorWithStatusUpdate(reqLogger, userSignup, r.setStatusInvalidMURState, err, "Multiple MasterUserRecords found")
	} else if len(murs) == 1 {
		mur := &murs[0]
		// If the user has been banned, then we need to delete the MUR
		if banned {
			// set the state label to banned
			if err := r.setStateLabel(reqLogger, userSignup, toolchainv1alpha1.UserSignupStateLabelValueBanned); err != nil {
				return true, err
			}
			reqLogger.Info("deleting MasterUserRecord since user has been banned")
			return true, r.DeleteMasterUserRecord(mur, userSignup, reqLogger, r.setStatusBanning, r.setStatusFailedToDeleteMUR)
		}

		// If the user has been deactivated, then we need to delete the MUR
		if states.Deactivated(userSignup) {
			// set the state label to deactivated
			if err := r.setStateLabel(reqLogger, userSignup, toolchainv1alpha1.UserSignupStateLabelValueDeactivated); err != nil {
				return true, err
			}
			// We set the inProgressStatusUpdater parameter here to setStatusDeactivationInProgress, as a temporary status before
			// the main reconcile function completes the deactivation process
			reqLogger.Info("deleting MasterUserRecord since user has been deactivated")
			return true, r.DeleteMasterUserRecord(mur, userSignup, reqLogger, r.setStatusDeactivationInProgress, r.setStatusFailedToDeleteMUR)
		}

		// if the UserSignup doesn't have the state=approved label set, then update it
		if err := r.setStateLabel(reqLogger, userSignup, toolchainv1alpha1.UserSignupStateLabelValueApproved); err != nil {
			return true, err
		}

		// look-up the `basic` NSTemplateTier to get the NS templates
		nstemplateTier, err := getNsTemplateTier(r.Client, defaultTierName, userSignup.Namespace)
		if err != nil {
			return true, r.wrapErrorWithStatusUpdate(reqLogger, userSignup, r.setStatusNoTemplateTierAvailable, err, "")
		}

		// check if anything in the MUR should be migrated/fixed
		if changed, err := migrateOrFixMurIfNecessary(mur, nstemplateTier, userSignup); err != nil {
			return true, r.wrapErrorWithStatusUpdate(reqLogger, userSignup, r.setStatusInvalidMURState, err, "unable to migrate or fix existing MasterUserRecord")

		} else if changed {
			reqLogger.Info("updating MasterUserRecord after it was migrated")
			if err := r.Client.Update(context.TODO(), mur); err != nil {
				return true, r.wrapErrorWithStatusUpdate(reqLogger, userSignup, r.setStatusInvalidMURState, err, "unable to migrate or fix existing MasterUserRecord")
			}
			return true, nil
		}

		// If we successfully found an existing MasterUserRecord then our work here is done, set the status
		// conditions to complete and set the compliant username and return
		reqLogger.Info("MasterUserRecord exists, setting UserSignup status to 'Complete'")
		// Use compliantUsernameUpdater to properly handle when the master user record is created or updated
		return true, r.updateStatus(reqLogger, userSignup, r.updateCompleteStatus(reqLogger, mur.Name))
	}
	return false, nil
}

func (r *Reconciler) ensureNewMurIfApproved(reqLogger logr.Logger, userSignup *toolchainv1alpha1.UserSignup) error {
	// Check if the user requires phone verification, and do not proceed further if they do
	if states.VerificationRequired(userSignup) {
		return r.updateStatus(reqLogger, userSignup, r.setStatusVerificationRequired)
	}

	approved, targetCluster, err := getClusterIfApproved(r.Client, userSignup, r.GetMemberClusters)
	// if error was returned or no available cluster found
	if err != nil || targetCluster == notFound {
		// set the state label to pending
		if err := r.setStateLabel(reqLogger, userSignup, toolchainv1alpha1.UserSignupStateLabelValuePending); err != nil {
			return err
		}
		// if user was approved manually
		if states.Approved(userSignup) {
			if err == nil {
				err = fmt.Errorf("no suitable member cluster found - capacity was reached")
			}
			return r.wrapErrorWithStatusUpdate(reqLogger, userSignup, r.set(statusApprovedByAdmin, statusNoClustersAvailable), err, "no target clusters available")
		}

		// if an error was returned, then log it, set the status and return an error
		if err != nil {
			return r.wrapErrorWithStatusUpdate(reqLogger, userSignup, r.set(statusPendingApproval, statusNoClustersAvailable), err, "getting target clusters failed")
		}
		// in case no error was returned which means that no cluster was found, then just wait for next reconcile triggered by ToolchainStatus update
		return r.updateStatus(reqLogger, userSignup, r.set(statusPendingApproval, statusNoClustersAvailable))
	}

	if !approved {
		// set the state label to pending
		if err := r.setStateLabel(reqLogger, userSignup, toolchainv1alpha1.UserSignupStateLabelValuePending); err != nil {
			return err
		}
		return r.updateStatus(reqLogger, userSignup, r.set(statusPendingApproval, statusIncompletePendingApproval))
	}

	if states.Approved(userSignup) {
		if err := r.updateStatus(reqLogger, userSignup, r.set(statusApprovedByAdmin)); err != nil {
			return err
		}
	} else {
		if err := r.updateStatus(reqLogger, userSignup, r.setStatusApprovedAutomatically); err != nil {
			return err
		}
	}
	// set the state label to approved
	if err := r.setStateLabel(reqLogger, userSignup, toolchainv1alpha1.UserSignupStateLabelValueApproved); err != nil {
		return err
	}

	// look-up the `base` NSTemplateTier to get the NS templates
	nstemplateTier, err := getNsTemplateTier(r.Client, defaultTierName, userSignup.Namespace)
	if err != nil {
		return r.wrapErrorWithStatusUpdate(reqLogger, userSignup, r.setStatusNoTemplateTierAvailable, err, "")
	}

	// Provision the MasterUserRecord
	return r.provisionMasterUserRecord(userSignup, targetCluster.getClusterName(), nstemplateTier, reqLogger)
}

func (r *Reconciler) setStateLabel(logger logr.Logger, userSignup *toolchainv1alpha1.UserSignup, state string) error {
	oldState := userSignup.Labels[toolchainv1alpha1.UserSignupStateLabelKey]
	if oldState == state {
		// skipping
		return nil
	}
	userSignup.Labels[toolchainv1alpha1.UserSignupStateLabelKey] = state
	activations := 0
	if state == toolchainv1alpha1.UserSignupStateLabelValueApproved {
		activations = r.updateActivationCounterAnnotation(logger, userSignup)
	}
	if err := r.Client.Update(context.TODO(), userSignup); err != nil {
		return r.wrapErrorWithStatusUpdate(logger, userSignup, r.setStatusFailedToUpdateStateLabel, err,
			"unable to update state label at UserSignup resource")
	}
	updateUserSignupMetricsByState(oldState, state)
	// increment the counter *only if the client update did not fail*
	domain := metrics.GetEmailDomain(userSignup)
	counter.UpdateUsersPerActivationCounters(logger, activations, domain) // will ignore if `activations == 0`

	return nil
}

func updateUserSignupMetricsByState(oldState, newState string) {
	if oldState == "" {
		metrics.UserSignupUniqueTotal.Inc()
	}
	switch newState {
	case toolchainv1alpha1.UserSignupStateLabelValueApproved:
		metrics.UserSignupApprovedTotal.Inc()
	case toolchainv1alpha1.UserSignupStateLabelValueDeactivated:
		metrics.UserSignupDeactivatedTotal.Inc()
	case toolchainv1alpha1.UserSignupStateLabelValueBanned:
		metrics.UserSignupBannedTotal.Inc()
	}
}

func getNsTemplateTier(cl client.Client, tierName, namespace string) (*toolchainv1alpha1.NSTemplateTier, error) {
	nstemplateTier := &toolchainv1alpha1.NSTemplateTier{}
	err := cl.Get(context.TODO(), types.NamespacedName{Namespace: namespace, Name: tierName}, nstemplateTier)
	return nstemplateTier, err
}

func (r *Reconciler) generateCompliantUsername(instance *toolchainv1alpha1.UserSignup) (string, error) {
	replaced := usersignup.TransformUsername(instance.Spec.Username)

	// Check for any forbidden prefixes
	for _, prefix := range r.CrtConfig.GetForbiddenUsernamePrefixes() {
		if strings.HasPrefix(replaced, prefix) {
			replaced = fmt.Sprintf("%s%s", "crt-", replaced)
			break
		}
	}

	// Check for any forbidden suffixes
	for _, suffix := range r.CrtConfig.GetForbiddenUsernameSuffixes() {
		if strings.HasSuffix(replaced, suffix) {
			replaced = fmt.Sprintf("%s%s", replaced, "-crt")
			break
		}
	}

	validationErrors := validation.IsQualifiedName(replaced)
	if len(validationErrors) > 0 {
		return "", fmt.Errorf(fmt.Sprintf("transformed username [%s] is invalid", replaced))
	}

	transformed := replaced

	for i := 2; i < 101; i++ { // No more than 100 attempts to find a vacant name
		mur := &toolchainv1alpha1.MasterUserRecord{}
		// Check if a MasterUserRecord exists with the same transformed name
		namespacedMurName := types.NamespacedName{Namespace: instance.Namespace, Name: transformed}
		err := r.Client.Get(context.TODO(), namespacedMurName, mur)
		if err != nil {
			if !errors.IsNotFound(err) {
				return "", err
			}
			// If there was a NotFound error looking up the mur, it means we found an available name
			return transformed, nil
		} else if mur.Labels[toolchainv1alpha1.MasterUserRecordOwnerLabelKey] == instance.Name {
			// If the found MUR has the same UserID as the UserSignup, then *it* is the correct MUR -
			// Return an error here and allow the reconcile() function to pick it up on the next loop
			return "", fmt.Errorf(fmt.Sprintf("INFO: could not generate compliant username as MasterUserRecord with the same name [%s] and user id [%s] already exists. The next reconcile loop will pick it up.", mur.Name, instance.Name))
		}

		transformed = fmt.Sprintf("%s-%d", replaced, i)
	}

	return "", fmt.Errorf(fmt.Sprintf("unable to transform username [%s] even after 100 attempts", instance.Spec.Username))
}

// provisionMasterUserRecord does the work of provisioning the MasterUserRecord
func (r *Reconciler) provisionMasterUserRecord(userSignup *toolchainv1alpha1.UserSignup, targetCluster string,
	nstemplateTier *toolchainv1alpha1.NSTemplateTier, logger logr.Logger) error {

	// TODO Update the MasterUserRecord with NSTemplateTier values
	// SEE https://jira.coreos.com/browse/CRT-74

	compliantUsername, err := r.generateCompliantUsername(userSignup)
	if err != nil {
		return r.wrapErrorWithStatusUpdate(logger, userSignup, r.setStatusFailedToCreateMUR, err,
			"Error generating compliant username for %s", userSignup.Spec.Username)
	}

	mur, err := newMasterUserRecord(userSignup, targetCluster, nstemplateTier, compliantUsername)
	if err != nil {
		return r.wrapErrorWithStatusUpdate(logger, userSignup, r.setStatusFailedToCreateMUR, err,
			"Error creating MasterUserRecord %s", mur.Name)
	}

	err = controllerutil.SetControllerReference(userSignup, mur, r.Scheme)
	if err != nil {
		return r.wrapErrorWithStatusUpdate(logger, userSignup, r.setStatusFailedToCreateMUR, err,
			"Error setting controller reference for MasterUserRecord %s", mur.Name)
	}

	logger.Info("Creating MasterUserRecord", "Name", mur.Name)
	err = r.Client.Create(context.TODO(), mur)
	if err != nil {
		return r.wrapErrorWithStatusUpdate(logger, userSignup, r.setStatusFailedToCreateMUR, err,
			"Error creating MasterUserRecord")
	}
	// increment the counter of MasterUserRecords
	domain := metrics.GetEmailDomain(mur)
	counter.IncrementMasterUserRecordCount(logger, domain)

	logger.Info("Created MasterUserRecord", "Name", mur.Name, "TargetCluster", targetCluster)
	return nil
}

// updateActivationCounterAnnotation increments the 'toolchain.dev.openshift.com/activation-counter' annotation value on the given UserSignup
func (r *Reconciler) updateActivationCounterAnnotation(logger logr.Logger, userSignup *toolchainv1alpha1.UserSignup) int {
	if activations, exists := userSignup.Annotations[toolchainv1alpha1.UserSignupActivationCounterAnnotationKey]; exists {
		logger.Info("updating 'toolchain.dev.openshift.com/activation-counter' on active user")
		activations, err := strconv.Atoi(activations)
		if err == nil {
			// increment the value of the annotation
			activations++
			userSignup.Annotations[toolchainv1alpha1.UserSignupActivationCounterAnnotationKey] = strconv.Itoa(activations)
			return activations
		}
		logger.Error(err, "The 'toolchain.dev.openshift.com/activation-counter' annotation value was not an integer and was reset to '1'.", "value", activations)
		// "best effort": reset number of activations to 1 for this user
		userSignup.Annotations[toolchainv1alpha1.UserSignupActivationCounterAnnotationKey] = "1"
		return 1
	}
	// annotation was missing so assume it's the first activation
	logger.Info("setting 'toolchain.dev.openshift.com/activation-counter' on new active user")
	userSignup.Annotations[toolchainv1alpha1.UserSignupActivationCounterAnnotationKey] = "1" // first activation, annotation did not exist
	return 1
}

// DeleteMasterUserRecord deletes the specified MasterUserRecord
func (r *Reconciler) DeleteMasterUserRecord(mur *toolchainv1alpha1.MasterUserRecord,
	userSignup *toolchainv1alpha1.UserSignup, logger logr.Logger,
	inProgressStatusUpdater, failedStatusUpdater StatusUpdaterFunc) error {

	err := r.updateStatus(logger, userSignup, inProgressStatusUpdater)
	if err != nil {
		return nil
	}

	err = r.Client.Delete(context.TODO(), mur)
	if err != nil {
		return r.wrapErrorWithStatusUpdate(logger, userSignup, failedStatusUpdater, err,
			"Error deleting MasterUserRecord")
	}
	logger.Info("Deleted MasterUserRecord", "Name", mur.Name)
	return nil
}

func (r *Reconciler) sendDeactivatingNotification(logger logr.Logger, userSignup *toolchainv1alpha1.UserSignup) error {
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

	if err := controllerutil.SetControllerReference(userSignup, notification, r.Scheme); err != nil {
		logger.Error(err, "Failed to set owner reference for deactivating notification resource")
		return err
	}

	if err := r.Client.Create(context.TODO(), notification); err != nil {
		logger.Error(err, "Failed to create deactivating notification resource")
		return err
	}

	logger.Info("Deactivating notification resource created")
	return nil
}

func (r *Reconciler) sendDeactivatedNotification(logger logr.Logger, userSignup *toolchainv1alpha1.UserSignup) error {
	notification := &toolchainv1alpha1.Notification{
		ObjectMeta: v1.ObjectMeta{
			GenerateName: fmt.Sprintf("%s-%s-", userSignup.Status.CompliantUsername, toolchainv1alpha1.NotificationTypeDeactivated),
			Namespace:    userSignup.Namespace,
			Labels: map[string]string{
				// NotificationUserNameLabelKey is only used for easy lookup for debugging and e2e tests
				toolchainv1alpha1.NotificationUserNameLabelKey: userSignup.Status.CompliantUsername,
				// NotificationTypeLabelKey is only used for easy lookup for debugging and e2e tests
				toolchainv1alpha1.NotificationTypeLabelKey: toolchainv1alpha1.NotificationTypeDeactivated,
			},
		},
		Spec: toolchainv1alpha1.NotificationSpec{
			UserID:   userSignup.Name,
			Template: notificationtemplates.UserDeactivated.Name,
		},
	}

	if err := controllerutil.SetControllerReference(userSignup, notification, r.Scheme); err != nil {
		logger.Error(err, "Failed to set owner reference for deactivation notification resource")
		return err
	}

	if err := r.Client.Create(context.TODO(), notification); err != nil {
		logger.Error(err, "Failed to create deactivation notification resource")
		return err
	}

	logger.Info("Deactivation notification resource created")
	return nil
}

// validateEmailHash calculates an md5 hash value for the provided userEmail string, and compares it to the provided
// userEmailHash.  If the values are the same the function returns true, otherwise it will return false
func validateEmailHash(userEmail, userEmailHash string) bool {
	md5hash := md5.New()
	// Ignore the error, as this implementation cannot return one
	_, _ = md5hash.Write([]byte(userEmail))
	return hex.EncodeToString(md5hash.Sum(nil)) == userEmailHash
}
