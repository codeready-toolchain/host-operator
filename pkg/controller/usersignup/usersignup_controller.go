package usersignup

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/pkg/apis/toolchain/v1alpha1"
	crtCfg "github.com/codeready-toolchain/host-operator/pkg/configuration"
	"github.com/codeready-toolchain/host-operator/pkg/controller/usersignup/unapproved"
	"github.com/codeready-toolchain/host-operator/pkg/counter"
	"github.com/codeready-toolchain/host-operator/pkg/metrics"
	"github.com/codeready-toolchain/host-operator/pkg/templates/notificationtemplates"
	"github.com/codeready-toolchain/toolchain-common/pkg/cluster"
	"github.com/codeready-toolchain/toolchain-common/pkg/condition"
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
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

var log = logf.Log.WithName("controller_usersignup")

var (
	specialCharRegexp = regexp.MustCompile("[^A-Za-z0-9]")
	onlyNumbers       = regexp.MustCompile("^[0-9]*$")
)

type StatusUpdater func(userAcc *toolchainv1alpha1.UserSignup, message string) error

const defaultTierName = "basic"

// Add creates a new UserSignup Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager, crtConfig *crtCfg.Config) error {
	return add(mgr, newReconciler(mgr, crtConfig))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager, crtConfig *crtCfg.Config) reconcile.Reconciler {
	return &ReconcileUserSignup{
		statusUpdater: &statusUpdater{
			client: mgr.GetClient(),
		},
		scheme:            mgr.GetScheme(),
		crtConfig:         crtConfig,
		getMemberClusters: cluster.GetMemberClusters,
	}
}

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

var _ reconcile.Reconciler = &ReconcileUserSignup{}

// ReconcileUserSignup reconciles a UserSignup object
type ReconcileUserSignup struct {
	*statusUpdater
	scheme            *runtime.Scheme
	crtConfig         *crtCfg.Config
	getMemberClusters cluster.GetMemberClustersFunc
}

// Reconcile reads that state of the cluster for a UserSignup object and makes changes based on the state read
// and what is in the UserSignup.Spec
// Note:
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcileUserSignup) Reconcile(request reconcile.Request) (reconcile.Result, error) {
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
	if instance.Labels[toolchainv1alpha1.UserSignupStateLabelKey] == "" {
		if err := r.setStateLabel(reqLogger, instance, toolchainv1alpha1.UserSignupStateLabelValueNotReady); err != nil {
			return reconcile.Result{}, err
		}
	}

	banned, err := r.isUserBanned(reqLogger, instance)
	if err != nil {
		return reconcile.Result{}, err
	}

	// If the usersignup is not banned and not deactivated then ensure the deactivated notification status is set to false.
	// This is especially important for cases when a user is deactivated and then reactivated because the status is used to
	// trigger sending of the notification. If a user is reactivated a notification should be sent to the user again.
	if !banned && !instance.Spec.Deactivated {
		if err := r.updateStatus(reqLogger, instance, r.setStatusDeactivationNotificationUserIsActive); err != nil {
			return reconcile.Result{}, err
		}
	}

	if exists, err := r.ensureMurIfAlreadyExists(reqLogger, instance, banned); exists || err != nil {
		return reconcile.Result{}, err
	}

	// If there is no MasterUserRecord created, yet the UserSignup is Banned, simply set the status
	// and return
	if banned {
		// if the UserSignup doesn't have the state=banned label set, then update it
		if err := r.setStateLabel(reqLogger, instance, toolchainv1alpha1.UserSignupStateLabelValueBanned); err != nil {
			return reconcile.Result{}, err
		}
		return reconcile.Result{}, r.updateStatus(reqLogger, instance, r.setStatusBanned)
	}

	// If there is no MasterUserRecord created, yet the UserSignup is marked as Deactivated, set the status,
	// send a notification to the user, and return
	if instance.Spec.Deactivated {
		// if the UserSignup doesn't have the state=deactivated label set, then update it
		if err := r.setStateLabel(reqLogger, instance, toolchainv1alpha1.UserSignupStateLabelValueDeactivated); err != nil {
			return reconcile.Result{}, err
		}
		if condition.IsNotTrue(instance.Status.Conditions, toolchainv1alpha1.UserSignupUserDeactivatedNotificationCreated) {
			if err := r.sendDeactivatedNotification(reqLogger, instance); err != nil {
				reqLogger.Error(err, "Failed to create user deactivation notification")

				// set the failed to create notification status condition
				return reconcile.Result{}, r.wrapErrorWithStatusUpdate(reqLogger, instance, r.setStatusDeactivationNotificationCreationFailed, err, "Failed to create user deactivation notification")
			}

			if err := r.updateStatus(reqLogger, instance, r.setStatusDeactivationNotificationCreated); err != nil {
				reqLogger.Error(err, "Failed to update notification created status")
				return reconcile.Result{}, err
			}
		}
		return reconcile.Result{}, r.updateStatus(reqLogger, instance, r.setStatusDeactivated)
	}

	return reconcile.Result{}, r.ensureNewMurIfApproved(reqLogger, instance)
}

// Is the user banned? To determine this we query the BannedUser resource for any matching entries.  The query
// is based on the user's emailHash value - if there is a match, and the e-mail addresses are equal, then the
// user is banned.
func (r *ReconcileUserSignup) isUserBanned(reqLogger logr.Logger, userSignup *toolchainv1alpha1.UserSignup) (bool, error) {
	banned := false
	// Lookup the user email annotation
	if emailLbl, exists := userSignup.Annotations[toolchainv1alpha1.UserSignupUserEmailAnnotationKey]; exists {

		// Lookup the email hash label
		if emailHashLbl, exists := userSignup.Labels[toolchainv1alpha1.UserSignupUserEmailHashLabelKey]; exists {

			labels := map[string]string{toolchainv1alpha1.BannedUserEmailHashLabelKey: emailHashLbl}
			opts := client.MatchingLabels(labels)
			bannedUserList := &toolchainv1alpha1.BannedUserList{}

			// Query BannedUser for resources that match the same email hash
			if err := r.client.List(context.TODO(), bannedUserList, opts); err != nil {
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

// ensureMurIfAlreadyExists checks if there is already a MUR for the given UserSignup.
// If there is already one then it returns 'true' as the first returned value, but before doing that it checks if the MUR should be deleted or not
// or if the MUR requires some migration changes or additional fixes.
// If no MUR for the given UserSignup is found, then it returns 'false' as the first returned value.
func (r *ReconcileUserSignup) ensureMurIfAlreadyExists(reqLogger logr.Logger, userSignup *toolchainv1alpha1.UserSignup, banned bool) (bool, error) {
	// List all MasterUserRecord resources that have a UserID label equal to the UserSignup.Name
	labels := map[string]string{toolchainv1alpha1.MasterUserRecordUserIDLabelKey: userSignup.Name}
	opts := client.MatchingLabels(labels)
	murList := &toolchainv1alpha1.MasterUserRecordList{}
	if err := r.client.List(context.TODO(), murList, opts); err != nil {
		return false, r.wrapErrorWithStatusUpdate(reqLogger, userSignup, r.setStatusInvalidMURState, err, "Failed to list MasterUserRecords")
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
			return true, r.DeleteMasterUserRecord(mur, userSignup, reqLogger, r.setStatusBanning, r.setStatusFailedToDeleteMUR)
		}

		// If the user has been deactivated, then we need to delete the MUR
		if userSignup.Spec.Deactivated {
			// set the state label to deactivated
			if err := r.setStateLabel(reqLogger, userSignup, toolchainv1alpha1.UserSignupStateLabelValueDeactivated); err != nil {
				return true, err
			}
			return true, r.DeleteMasterUserRecord(mur, userSignup, reqLogger, r.setStatusDeactivating, r.setStatusFailedToDeleteMUR)
		}

		// if the UserSignup doesn't have the state=approved label set, then update it
		if err := r.setStateLabel(reqLogger, userSignup, toolchainv1alpha1.UserSignupStateLabelValueApproved); err != nil {
			return true, err
		}

		// look-up the `basic` NSTemplateTier to get the NS templates
		nstemplateTier, err := getNsTemplateTier(r.client, defaultTierName, userSignup.Namespace)
		if err != nil {
			return true, r.wrapErrorWithStatusUpdate(reqLogger, userSignup, r.setStatusNoTemplateTierAvailable, err, "")
		}

		// check if anything in the MUR should be migrated/fixed
		if changed, err := migrateOrFixMurIfNecessary(mur, nstemplateTier); err != nil {
			return true, r.wrapErrorWithStatusUpdate(reqLogger, userSignup, r.setStatusInvalidMURState, err, "unable to migrate or fix existing MasterUserRecord")

		} else if changed {
			if err := r.client.Update(context.TODO(), mur); err != nil {
				return true, r.wrapErrorWithStatusUpdate(reqLogger, userSignup, r.setStatusInvalidMURState, err, "unable to migrate or fix existing MasterUserRecord")
			}
			return true, nil
		}

		// If we successfully found an existing MasterUserRecord then our work here is done, set the status
		// conditions to complete and set the compliant username and return
		reqLogger.Info("MasterUserRecord exists, setting UserSignup status to 'Complete'")
		// Use compliantUsernameUpdater to properly handle when the master user record is created or updated
		return true, r.updateStatus(reqLogger, userSignup, r.updateCompleteStatus(mur.Name))
	}
	return false, nil
}

func (r *ReconcileUserSignup) ensureNewMurIfApproved(reqLogger logr.Logger, userSignup *toolchainv1alpha1.UserSignup) error {
	// Check if the user requires phone verification, and do not proceed further if they do
	if userSignup.Spec.VerificationRequired {
		return r.updateStatus(reqLogger, userSignup, r.setStatusVerificationRequired)
	}

	approved, targetCluster, err := getClusterIfApproved(r.client, r.crtConfig, userSignup, r.getMemberClusters)
	// if error was returned or no available cluster found
	if err != nil || targetCluster == notFound {
		// set the state label to pending
		if err := r.setStateLabel(reqLogger, userSignup, toolchainv1alpha1.UserSignupStateLabelValuePending); err != nil {
			return err
		}
		// if user was approved manually
		if userSignup.Spec.Approved {
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

	if userSignup.Spec.Approved {
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

	// look-up the `basic` NSTemplateTier to get the NS templates
	nstemplateTier, err := getNsTemplateTier(r.client, defaultTierName, userSignup.Namespace)
	if err != nil {
		return r.wrapErrorWithStatusUpdate(reqLogger, userSignup, r.setStatusNoTemplateTierAvailable, err, "")
	}

	// Provision the MasterUserRecord
	return r.provisionMasterUserRecord(userSignup, targetCluster.getClusterName(), nstemplateTier, reqLogger)
}

func (r *ReconcileUserSignup) setStateLabel(reqLogger logr.Logger, userSignup *toolchainv1alpha1.UserSignup, value string) error {
	oldValue := userSignup.Labels[toolchainv1alpha1.UserSignupStateLabelKey]
	if oldValue != value {
		userSignup.Labels[toolchainv1alpha1.UserSignupStateLabelKey] = value
		if err := r.client.Update(context.TODO(), userSignup); err != nil {
			return r.wrapErrorWithStatusUpdate(reqLogger, userSignup, r.setStatusFailedToUpdateStateLabel, err,
				"unable to update state label at UserSignup resource")
		}
		updateMetricsByState(oldValue, value)
		return nil
	}
	return nil
}

func updateMetricsByState(oldState, newState string) {
	metrics.UserSignupUniqueTotal.ConditionalIncrement(oldState == "")
	metrics.UserSignupApprovedTotal.ConditionalIncrement(newState == toolchainv1alpha1.UserSignupStateLabelValueApproved)
	metrics.UserSignupDeactivatedTotal.ConditionalIncrement(newState == toolchainv1alpha1.UserSignupStateLabelValueDeactivated)
	metrics.UserSignupBannedTotal.ConditionalIncrement(newState == toolchainv1alpha1.UserSignupStateLabelValueBanned)
}

func getNsTemplateTier(cl client.Client, tierName, namespace string) (*toolchainv1alpha1.NSTemplateTier, error) {
	nstemplateTier := &toolchainv1alpha1.NSTemplateTier{}
	err := cl.Get(context.TODO(), types.NamespacedName{Namespace: namespace, Name: tierName}, nstemplateTier)
	return nstemplateTier, err
}

func (r *ReconcileUserSignup) generateCompliantUsername(instance *toolchainv1alpha1.UserSignup) (string, error) {
	replaced := transformUsername(instance.Spec.Username)

	// Check for any forbidden prefixes
	for _, prefix := range r.crtConfig.GetForbiddenUsernamePrefixes() {
		if strings.HasPrefix(replaced, prefix) {
			replaced = fmt.Sprintf("%s%s", "crt-", replaced)
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
		err := r.client.Get(context.TODO(), namespacedMurName, mur)
		if err != nil {
			if !errors.IsNotFound(err) {
				return "", err
			}
			// If there was a NotFound error looking up the mur, it means we found an available name
			return transformed, nil
		} else if mur.Labels[toolchainv1alpha1.MasterUserRecordUserIDLabelKey] == instance.Name {
			// If the found MUR has the same UserID as the UserSignup, then *it* is the correct MUR -
			// Return an error here and allow the reconcile() function to pick it up on the next loop
			return "", fmt.Errorf(fmt.Sprintf("INFO: could not generate compliant username as MasterUserRecord with the same name [%s] and user id [%s] already exists. The next reconcile loop will pick it up.", mur.Name, instance.Name))
		}

		transformed = fmt.Sprintf("%s-%d", replaced, i)
	}

	return "", fmt.Errorf(fmt.Sprintf("unable to transform username [%s] even after 100 attempts", instance.Spec.Username))
}

func transformUsername(username string) string {
	newUsername := specialCharRegexp.ReplaceAllString(strings.Split(username, "@")[0], "-")
	if len(newUsername) == 0 {
		newUsername = strings.ReplaceAll(username, "@", "at-")
	}
	newUsername = specialCharRegexp.ReplaceAllString(newUsername, "-")

	matched := onlyNumbers.MatchString(newUsername)
	if matched {
		newUsername = "crt-" + newUsername
	}
	for strings.Contains(newUsername, "--") {
		newUsername = strings.ReplaceAll(newUsername, "--", "-")
	}
	if strings.HasPrefix(newUsername, "-") {
		newUsername = "crt" + newUsername
	}
	if strings.HasSuffix(newUsername, "-") {
		newUsername = newUsername + "crt"
	}
	return newUsername
}

// provisionMasterUserRecord does the work of provisioning the MasterUserRecord
func (r *ReconcileUserSignup) provisionMasterUserRecord(userSignup *toolchainv1alpha1.UserSignup, targetCluster string,
	nstemplateTier *toolchainv1alpha1.NSTemplateTier, logger logr.Logger) error {

	// TODO Update the MasterUserRecord with NSTemplateTier values
	// SEE https://jira.coreos.com/browse/CRT-74

	compliantUsername, err := r.generateCompliantUsername(userSignup)
	if err != nil {
		return r.wrapErrorWithStatusUpdate(logger, userSignup, r.setStatusFailedToCreateMUR, err,
			"Error generating compliant username for %s", userSignup.Spec.Username)
	}

	mur, err := newMasterUserRecord(nstemplateTier, compliantUsername, userSignup.Namespace, targetCluster, userSignup.Name)
	if err != nil {
		return r.wrapErrorWithStatusUpdate(logger, userSignup, r.setStatusFailedToCreateMUR, err,
			"Error creating MasterUserRecord %s", mur.Name)
	}

	err = controllerutil.SetControllerReference(userSignup, mur, r.scheme)
	if err != nil {
		return r.wrapErrorWithStatusUpdate(logger, userSignup, r.setStatusFailedToCreateMUR, err,
			"Error setting controller reference for MasterUserRecord %s", mur.Name)
	}

	logger.Info("Creating MasterUserRecord", "Name", mur.Name)
	err = r.client.Create(context.TODO(), mur)
	if err != nil {
		return r.wrapErrorWithStatusUpdate(logger, userSignup, r.setStatusFailedToCreateMUR, err,
			"Error creating MasterUserRecord")
	}
	counter.IncrementMasterUserRecordCount()

	logger.Info("Created MasterUserRecord", "Name", mur.Name, "TargetCluster", targetCluster)
	return nil
}

// DeleteMasterUserRecord deletes the specified MasterUserRecord
func (r *ReconcileUserSignup) DeleteMasterUserRecord(mur *toolchainv1alpha1.MasterUserRecord,
	userSignup *toolchainv1alpha1.UserSignup, logger logr.Logger,
	inProgressStatusUpdater, failedStatusUpdater StatusUpdater) error {

	err := r.updateStatus(logger, userSignup, inProgressStatusUpdater)
	if err != nil {
		return nil
	}

	err = r.client.Delete(context.TODO(), mur)
	if err != nil {
		return r.wrapErrorWithStatusUpdate(logger, userSignup, failedStatusUpdater, err,
			"Error deleting MasterUserRecord")
	}
	logger.Info("Deleted MasterUserRecord", "Name", mur.Name)
	return nil
}

func (r *ReconcileUserSignup) sendDeactivatedNotification(logger logr.Logger, userSignup *toolchainv1alpha1.UserSignup) error {
	notification := &toolchainv1alpha1.Notification{
		ObjectMeta: v1.ObjectMeta{
			Name:      userSignup.Status.CompliantUsername + "-deactivated",
			Namespace: userSignup.Namespace,
		},
		Spec: toolchainv1alpha1.NotificationSpec{
			UserID:   userSignup.Name,
			Template: notificationtemplates.UserDeactivated.Name,
		},
	}

	if err := controllerutil.SetControllerReference(userSignup, notification, r.scheme); err != nil {
		logger.Error(err, "Failed to set owner reference for deactivation notification resource")
		return err
	}

	if err := r.client.Create(context.TODO(), notification); err != nil {
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
