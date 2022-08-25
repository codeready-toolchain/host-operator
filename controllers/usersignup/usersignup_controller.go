package usersignup

import (
	"context"
	"crypto/md5" //nolint:gosec
	"encoding/hex"
	"fmt"
	"hash/crc32"
	"regexp"
	"strconv"
	"strings"
	"time"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/host-operator/controllers/toolchainconfig"
	"github.com/codeready-toolchain/host-operator/pkg/counter"
	"github.com/codeready-toolchain/host-operator/pkg/metrics"
	"github.com/codeready-toolchain/host-operator/pkg/pending"
	"github.com/codeready-toolchain/host-operator/pkg/segment"
	"github.com/codeready-toolchain/host-operator/pkg/templates/notificationtemplates"
	commoncontrollers "github.com/codeready-toolchain/toolchain-common/controllers"
	"github.com/codeready-toolchain/toolchain-common/pkg/cluster"
	"github.com/codeready-toolchain/toolchain-common/pkg/condition"
	notify "github.com/codeready-toolchain/toolchain-common/pkg/notification"
	"github.com/codeready-toolchain/toolchain-common/pkg/states"
	"github.com/codeready-toolchain/toolchain-common/pkg/usersignup"
	"github.com/redhat-cop/operator-utils/pkg/util"

	"github.com/go-logr/logr"
	errs "github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/validation"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

type StatusUpdaterFunc func(userAcc *toolchainv1alpha1.UserSignup, message string) error

const (
	migrationReplacesAnnotationName   = toolchainv1alpha1.LabelKeyPrefix + "migration-replaces"
	migratedAnnotationName            = toolchainv1alpha1.LabelKeyPrefix + "migrated"
	migrationInProgressAnnotationName = toolchainv1alpha1.LabelKeyPrefix + "migration-in-progress"
)

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr manager.Manager) error {
	unapprovedMapper := pending.NewUserSignupMapper(mgr.GetClient())
	return ctrl.NewControllerManagedBy(mgr).
		For(&toolchainv1alpha1.UserSignup{}, builder.WithPredicates(UserSignupChangedPredicate{})).
		Owns(&toolchainv1alpha1.MasterUserRecord{}).
		Watches(
			&source.Kind{Type: &toolchainv1alpha1.BannedUser{}},
			handler.EnqueueRequestsFromMapFunc(MapBannedUserToUserSignup(mgr.GetClient()))).
		Watches(
			&source.Kind{Type: &toolchainv1alpha1.Space{}},
			handler.EnqueueRequestsFromMapFunc(commoncontrollers.MapToOwnerByLabel(r.Namespace, toolchainv1alpha1.SpaceCreatorLabelKey))).
		Watches(
			&source.Kind{Type: &toolchainv1alpha1.SpaceBinding{}},
			handler.EnqueueRequestsFromMapFunc(commoncontrollers.MapToOwnerByLabel(r.Namespace, toolchainv1alpha1.SpaceCreatorLabelKey))).
		Watches(
			&source.Kind{Type: &toolchainv1alpha1.ToolchainStatus{}},
			handler.EnqueueRequestsFromMapFunc(unapprovedMapper.MapToOldestPending),
			builder.WithPredicates(&OnlyWhenAutomaticApprovalIsEnabled{
				client: mgr.GetClient(),
			})).
		Complete(r)
}

// Reconciler reconciles a UserSignup object
type Reconciler struct {
	*StatusUpdater
	Namespace         string
	Scheme            *runtime.Scheme
	GetMemberClusters cluster.GetMemberClustersFunc
	SegmentClient     *segment.Client
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
func (r *Reconciler) Reconcile(ctx context.Context, request ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
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

	// Don't reconcile while migration is in progress
	// TODO delete this after migration is complete
	if _, found := userSignup.Annotations[migrationInProgressAnnotationName]; found {
		return reconcile.Result{}, nil
	}

	if util.IsBeingDeleted(userSignup) {
		logger.Info("The UserSignup is being deleted")
		return reconcile.Result{}, nil
	}

	// TODO REMOVE THIS BLOCK AFTER MIGRATION - See additional comments further below in the migrateUserIfNecessary function
	if originalUserSignupName, found := userSignup.Annotations[migrationReplacesAnnotationName]; found {
		return r.cleanupMigration(userSignup, originalUserSignupName, request, logger)
	}
	if userSignup.GetLabels() == nil {
		userSignup.Labels = make(map[string]string)
	}
	if userSignup.Labels[toolchainv1alpha1.UserSignupStateLabelKey] == "" {
		if err := r.setStateLabel(logger, userSignup, toolchainv1alpha1.UserSignupStateLabelValueNotReady); err != nil {
			return reconcile.Result{}, err
		}
	}

	config, err := toolchainconfig.GetToolchainConfig(r.Client)
	if err != nil {
		return reconcile.Result{}, err
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

		if err := r.sendDeactivatingNotification(logger, config, userSignup); err != nil {
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

	if exists, err := r.checkIfMurAlreadyExists(logger, config, userSignup, banned); err != nil || exists {
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

	// Check if the user has been deactivated
	if states.Deactivated(userSignup) {
		return r.handleDeactivatedUserSignup(logger, config, request, userSignup)
	}

	return reconcile.Result{}, r.ensureNewMurIfApproved(logger, config, userSignup)
}

// handleDeactivatedUserSignup defines the workflow for deactivated users
//
// If there is no MasterUserRecord created, yet the UserSignup is marked as Deactivated, set the status,
// send a notification to the user, and return
func (r *Reconciler) handleDeactivatedUserSignup(logger logr.Logger, config toolchainconfig.ToolchainConfig,
	request ctrl.Request, userSignup *toolchainv1alpha1.UserSignup) (ctrl.Result, error) {

	// Only send the deactivated notification if the previous state was "approved", i.e. we will only send the
	// deactivated notification to the user if the account is currently active and is being deactivated
	if userSignup.Labels[toolchainv1alpha1.UserSignupStateLabelKey] == toolchainv1alpha1.UserSignupStateLabelValueApproved &&
		condition.IsNotTrue(userSignup.Status.Conditions, toolchainv1alpha1.UserSignupUserDeactivatedNotificationCreated) {
		if err := r.sendDeactivatedNotification(logger, config, userSignup); err != nil {
			logger.Error(err, "Failed to create user deactivation notification")

			// set the failed to create notification status condition
			return reconcile.Result{}, r.wrapErrorWithStatusUpdate(logger, userSignup, r.setStatusDeactivationNotificationCreationFailed, err, "Failed to create user deactivation notification")
		}

		if err := r.updateStatus(logger, userSignup, r.setStatusDeactivationNotificationCreated); err != nil {
			logger.Error(err, "Failed to update notification created status")
			return reconcile.Result{}, err
		}
	}

	// if the UserSignup doesn't have the state=deactivated label set, then update it
	if err := r.setStateLabel(logger, userSignup, toolchainv1alpha1.UserSignupStateLabelValueDeactivated); err != nil {
		return reconcile.Result{}, err
	}

	err := r.updateStatus(logger, userSignup, r.setStatusDeactivated)
	if err != nil {
		return reconcile.Result{}, err
	}

	// Migrate the user if necessary
	return reconcile.Result{}, r.migrateUserIfNecessary(userSignup, request, logger)
}

// migrateUserIfNecessary migrates the UserSignup if necessary - REMOVE THIS FUNCTION AFTER MIGRATION COMPLETE
// TODO remove this function once all basic tier users have been migrated
// The migration process is as follows:
//
// 1) Check if the UserSignup has been migrated.  This is done by checking if a UserSignup with a name equal
//    to the encoded username exists (while also checking that the currently reconciling UserSignup isn't actually it)
//
// 2) If the UserSignup hasn't been migrated, then migrate it by creating a new UserSignup with the name set to
//    the encoded username, and the "migration-replaces" annotation set to the name of this (the old) UserSignup
//
// 3)
//
// Other things to remove after migration is completed:
//
// 1) The setStatusMigrationFailedLookup function in status.go
// 2) The setStatusMigrationFailedCreate function in status.go
// 3) The setStatusMigrationFailedCleanup function in status.go
// 4) The setStatusMigrationSuccessful function in status.go
// 5) The UserSignupUserMigrationFailed constant in status.go
// 6) The EncodeUserIdentifier function from this file
// 7) The above block of code in this function that deletes the original UserSignup if the "migration-replaces" annotation is set
// 8) The cleanupMigration function from this file
// 9) The TestUserSignupMigration() test in usersignup_controller_test.go
//
func (r *Reconciler) migrateUserIfNecessary(userSignup *toolchainv1alpha1.UserSignup, request ctrl.Request, logger logr.Logger) error {
	encodedUsername := EncodeUserIdentifier(userSignup.Spec.Username)
	if userSignup.Name == encodedUsername {
		// current usersignup has been migrated
		return nil
	}
	migratedUserSignup := &toolchainv1alpha1.UserSignup{}
	if err := r.Client.Get(context.TODO(), types.NamespacedName{
		Namespace: request.Namespace,
		Name:      encodedUsername,
	}, migratedUserSignup); err != nil {
		if !errors.IsNotFound(err) {
			// Error reading the object - requeue the request.
			return r.wrapErrorWithStatusUpdate(logger, userSignup,
				r.setStatusMigrationFailedLookup, err, "Failed to lookup migrated UserSignup")
		}

		// Create the migrated UserSignup here.
		//
		// We need to:
		//
		// 1) Copy the existing UserSignup
		// 2) Override the new UserSignup resource name with the encoded username
		// 3) Set the starting status to Deactivated (technically we could let the reconciler function do this
		//    when it reconciles the migrated UserSignup, but it shouldn't hurt to set this status up front)
		migratedUserSignup = &toolchainv1alpha1.UserSignup{
			ObjectMeta: metav1.ObjectMeta{
				Name:        encodedUsername,
				Namespace:   userSignup.Namespace,
				Labels:      map[string]string{},
				Annotations: map[string]string{},
			},
			Spec: userSignup.Spec,
		}

		// Copy the labels and annotations
		for key, value := range userSignup.Labels {
			migratedUserSignup.Labels[key] = value
		}

		for key, value := range userSignup.Annotations {
			migratedUserSignup.Annotations[key] = value
		}

		migratedUserSignup.Annotations[migrationReplacesAnnotationName] = userSignup.Name
		migratedUserSignup.Annotations[migrationInProgressAnnotationName] = "true"

		if err := r.Client.Create(context.TODO(), migratedUserSignup); err != nil {
			// If there was an error creating the migrated UserSignup, then set the status and requeue
			return r.wrapErrorWithStatusUpdate(logger, userSignup,
				r.setStatusMigrationFailedCreate, err, "Failed to create migrated UserSignup")
		}

		// Reload the UserSignup in order to populate the status (which happens outside of this condition block)
		if err := r.Client.Get(context.TODO(), types.NamespacedName{
			Namespace: migratedUserSignup.Namespace,
			Name:      migratedUserSignup.Name,
		}, migratedUserSignup); err != nil {
			// If there was an error reloading the migrated UserSignup, then set the status and requeue
			return r.wrapErrorWithStatusUpdate(logger, userSignup,
				r.setStatusMigrationFailedCreate, err, "Failed to reload migrated UserSignup")
		}
	}

	// Copy the status and conditions
	migratedUserSignup.Status.CompliantUsername = userSignup.Status.CompliantUsername
	migratedUserSignup.Status.Conditions = []toolchainv1alpha1.Condition{}
	migratedUserSignup.Status.Conditions = append(migratedUserSignup.Status.Conditions, userSignup.Status.Conditions...)

	// We want to force these particular conditions no matter what
	migratedUserSignup.Status.Conditions, _ = condition.AddOrUpdateStatusConditions(migratedUserSignup.Status.Conditions,
		toolchainv1alpha1.Condition{
			Type:    UserMigrated,
			Status:  corev1.ConditionFalse,
			Reason:  "MigrationStarted",
			Message: "",
		})

	if err := r.Client.Status().Update(context.TODO(), migratedUserSignup); err != nil {
		// If there was an error updating the status for the migrated UserSignup, then set the status and requeue
		return r.wrapErrorWithStatusUpdate(logger, userSignup,
			r.setStatusMigrationFailedCreate, err, "Failed to update status for migrated UserSignup")
	}

	// Finally, we need to remove the migration in progress annotation
	delete(migratedUserSignup.Annotations, migrationInProgressAnnotationName)

	if err := r.Client.Update(context.TODO(), migratedUserSignup); err != nil {
		// If there was an error updating the annotations for the migrated UserSignup, then set the status and requeue
		return r.wrapErrorWithStatusUpdate(logger, userSignup,
			r.setStatusMigrationFailedCreate, err, "Failed to remove migration in progress annotation for migrated UserSignup")
	}

	return r.setStatusMigrationSuccessful(userSignup)
}

// cleanupMigration performs migration cleanup if the "migration-replaces" annotation has been set on the UserSignup
//
// If the annotation has been set, then it indicates that this UserSignup has been migrated, and that the
// *original* UserSignup should be deleted.  The value of the annotation is the name of the original UserSignup
// resource that should now be deleted.
//
// This function should:
//
// 1) Delete the original UserSignup, then
// 2) Remove the migration-replaces annotation
func (r *Reconciler) cleanupMigration(userSignup *toolchainv1alpha1.UserSignup, originalUserSignupName string,
	request ctrl.Request, logger logr.Logger) (ctrl.Result, error) {

	userSignupToDelete := &toolchainv1alpha1.UserSignup{}
	if err := r.Client.Get(context.TODO(), types.NamespacedName{
		Namespace: request.Namespace,
		Name:      originalUserSignupName,
	}, userSignupToDelete); err != nil {
		// If the annotation exists however the original UserSignup isn't found, then remove the annotation and the
		// migration condition from the status
		if errors.IsNotFound(err) {

			// Set the migrated annotation and delete the migration from annotation
			userSignup.Annotations[migratedAnnotationName] = fmt.Sprintf("migrated from [%s]", originalUserSignupName)
			delete(userSignup.Annotations, migrationReplacesAnnotationName)

			// Update the UserSignup
			err := r.Client.Update(context.TODO(), userSignup)
			if err != nil {
				return reconcile.Result{}, r.wrapErrorWithStatusUpdate(logger, userSignup,
					r.setStatusMigrationFailedCleanup, err, "Failed to update migrated UserSignup")
			}
			// If everything was cleaned up successfully, requeue one more time to invoke the standard reconciliation workflow
			return reconcile.Result{Requeue: true}, nil
		}

		return reconcile.Result{}, r.wrapErrorWithStatusUpdate(logger, userSignup,
			r.setStatusMigrationFailedLookup, err, fmt.Sprintf("Failed to lookup original UserSignup [%s]",
				userSignup.Annotations[migrationReplacesAnnotationName]))
	}

	// Delete the original UserSignup if it has finished migration
	if !condition.IsTrueWithReason(userSignupToDelete.Status.Conditions, toolchainv1alpha1.UserSignupComplete,
		toolchainv1alpha1.UserSignupUserDeactivatedReason) {
		// The UserSignup to delete isn't finished migrating yet, return an error
		return reconcile.Result{}, r.wrapErrorWithStatusUpdate(logger, userSignup, r.setStatusMigrationFailedCleanup,
			errs.New("Original UserSignup not finished migration"),
			fmt.Sprintf("Original UserSignup [%s] not yet finished migration", userSignupToDelete.Name))

	}

	if err := r.Client.Delete(context.TODO(), userSignupToDelete); err != nil {
		return reconcile.Result{}, r.wrapErrorWithStatusUpdate(logger, userSignup, r.setStatusMigrationFailedCleanup,
			err, fmt.Sprintf("Failed to remove original UserSignup [%s]",
				userSignup.Annotations[migrationReplacesAnnotationName]))
	}
	// Requeue so that the annotation will now be removed also
	return reconcile.Result{Requeue: true, RequeueAfter: time.Second}, nil
}

// EncodeUserIdentifier is a temporary function used by the migration procedure
// REMOVE AFTER MIGRATION IS COMPLETE
func EncodeUserIdentifier(subject string) string {
	DNS1123NameMaximumLength := 63
	DNS1123NotAllowedCharacters := "[^-a-z0-9]"
	DNS1123NotAllowedStartCharacters := "^[^a-z0-9]+"
	DNS1123NotAllowedEndCharacters := "[^a-z0-9]+$"

	// Convert to lower case
	encoded := strings.ToLower(subject)

	// Remove all invalid characters
	nameNotAllowedChars := regexp.MustCompile(DNS1123NotAllowedCharacters)
	encoded = nameNotAllowedChars.ReplaceAllString(encoded, "")

	// Remove invalid start characters
	nameNotAllowedStartChars := regexp.MustCompile(DNS1123NotAllowedStartCharacters)
	encoded = nameNotAllowedStartChars.ReplaceAllString(encoded, "")

	// Remove invalid end characters
	nameNotAllowedEndChars := regexp.MustCompile(DNS1123NotAllowedEndCharacters)
	encoded = nameNotAllowedEndChars.ReplaceAllString(encoded, "")

	// Add a checksum prefix if the encoded value is different to the original subject value
	if encoded != subject {
		encoded = fmt.Sprintf("%x-%s", crc32.Checksum([]byte(subject), crc32.IEEETable), encoded)
	}

	// Trim if the length exceeds the maximum
	if len(encoded) > DNS1123NameMaximumLength {
		encoded = encoded[0:DNS1123NameMaximumLength]
	}

	return encoded
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
func (r *Reconciler) checkIfMurAlreadyExists(reqLogger logr.Logger, config toolchainconfig.ToolchainConfig, userSignup *toolchainv1alpha1.UserSignup,
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

			reqLogger.Info("Deleting MasterUserRecord since user has been banned")
			return true, r.deleteMasterUserRecord(mur, userSignup, reqLogger, r.setStatusBanning)
		}

		// If the user has been deactivated, then we need to delete the MUR
		if states.Deactivated(userSignup) {
			// We set the inProgressStatusUpdater parameter here to setStatusDeactivationInProgress, as a temporary status before
			// the main reconcile function completes the deactivation process
			reqLogger.Info("Deleting MasterUserRecord since user has been deactivated")
			return true, r.deleteMasterUserRecord(mur, userSignup, reqLogger, r.setStatusDeactivationInProgress)
		}

		// if the UserSignup doesn't have the state=approved label set, then update it
		if err := r.setStateLabel(reqLogger, userSignup, toolchainv1alpha1.UserSignupStateLabelValueApproved); err != nil {
			return true, err
		}

		// look-up the default UserTier to set it on the MUR if not set
		defaultUserTier, err := getUserTier(r.Client, config.Tiers().DefaultUserTier(), userSignup.Namespace)
		if err != nil {
			return true, r.wrapErrorWithStatusUpdate(reqLogger, userSignup, r.setStatusNoUserTierAvailable, err, "")
		}

		// look-up the default NSTemplateTier to set it on the MUR if not set
		defaultSpaceTier, err := getNsTemplateTier(r.Client, config.Tiers().DefaultSpaceTier(), userSignup.Namespace)
		if err != nil {
			return true, r.wrapErrorWithStatusUpdate(reqLogger, userSignup, r.setStatusNoTemplateTierAvailable, err, "")
		}

		// check if anything in the MUR should be migrated/fixed
		if changed := migrateOrFixMurIfNecessary(mur, defaultUserTier, userSignup); changed {
			reqLogger.Info("Updating MasterUserRecord after it was migrated")
			if err := r.Client.Update(context.TODO(), mur); err != nil {
				return true, r.wrapErrorWithStatusUpdate(reqLogger, userSignup, r.setStatusInvalidMURState, err, "unable update MasterUserRecord to complete migration")
			}
			return true, nil
		}
		reqLogger.Info("MasterUserRecord exists", "Name", mur.Name)

		if shouldManageSpace(userSignup) {
			space, created, err := r.ensureSpace(reqLogger, userSignup, mur, defaultSpaceTier)
			// if there was an error or the space was created then return to complete the reconcile, another reconcile will occur when space is created since this controller watches spaces
			if err != nil || created {
				return true, err
			}

			if err = r.ensureSpaceBinding(reqLogger, userSignup, mur, space); err != nil {
				return true, err
			}

			ready, err := r.ensureSpaceReady(reqLogger, space)
			// if there was an error or the Space is not Ready, another reconcile will occur when space status is updated since this controller watches space (without a predicate)
			if !ready || err != nil {
				return true, err
			}
		}

		reqLogger.Info("Setting UserSignup status to 'Complete'")
		return true, r.updateStatus(reqLogger, userSignup, r.updateCompleteStatus(reqLogger, mur.Name))
	}
	return false, nil
}

func (r *Reconciler) ensureNewMurIfApproved(reqLogger logr.Logger, config toolchainconfig.ToolchainConfig, userSignup *toolchainv1alpha1.UserSignup) error {
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

	// look-up the default UserTier
	userTier, err := getUserTier(r.Client, config.Tiers().DefaultUserTier(), userSignup.Namespace)
	if err != nil {
		return r.wrapErrorWithStatusUpdate(reqLogger, userSignup, r.setStatusNoUserTierAvailable, err, "")
	}

	// Provision the MasterUserRecord
	return r.provisionMasterUserRecord(reqLogger, config, userSignup, targetCluster, userTier)
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
	r.updateUserSignupMetricsByState(logger, userSignup, oldState, state)
	// increment the counter *only if the client update did not fail*
	domain := metrics.GetEmailDomain(userSignup)
	counter.UpdateUsersPerActivationCounters(logger, activations, domain) // will ignore if `activations == 0`
	return nil
}

func (r *Reconciler) updateUserSignupMetricsByState(logger logr.Logger, userSignup *toolchainv1alpha1.UserSignup, oldState string, newState string) {
	if oldState == "" {
		metrics.UserSignupUniqueTotal.Inc()
	}
	switch newState {
	case toolchainv1alpha1.UserSignupStateLabelValueApproved:
		metrics.UserSignupApprovedTotal.Inc()
		// track activation in Segment
		if r.SegmentClient != nil {
			r.SegmentClient.TrackAccountActivation(userSignup.Spec.Username)
		} else {
			logger.Info("segment client not configure to track account activations")
		}
	case toolchainv1alpha1.UserSignupStateLabelValueDeactivated:
		if oldState == toolchainv1alpha1.UserSignupStateLabelValueApproved {
			metrics.UserSignupDeactivatedTotal.Inc()
		}
	case toolchainv1alpha1.UserSignupStateLabelValueBanned:
		metrics.UserSignupBannedTotal.Inc()
	}
}

func getNsTemplateTier(cl client.Client, tierName, namespace string) (*toolchainv1alpha1.NSTemplateTier, error) {
	nstemplateTier := &toolchainv1alpha1.NSTemplateTier{}
	err := cl.Get(context.TODO(), types.NamespacedName{Namespace: namespace, Name: tierName}, nstemplateTier)
	return nstemplateTier, err
}

func getUserTier(cl client.Client, tierName, namespace string) (*toolchainv1alpha1.UserTier, error) {
	userTier := &toolchainv1alpha1.UserTier{}
	err := cl.Get(context.TODO(), types.NamespacedName{Namespace: namespace, Name: tierName}, userTier)
	return userTier, err
}

func (r *Reconciler) generateCompliantUsername(config toolchainconfig.ToolchainConfig, instance *toolchainv1alpha1.UserSignup) (string, error) {
	replaced := usersignup.TransformUsername(instance.Spec.Username)

	// Check for any forbidden prefixes
	for _, prefix := range config.Users().ForbiddenUsernamePrefixes() {
		if strings.HasPrefix(replaced, prefix) {
			replaced = fmt.Sprintf("%s%s", "crt-", replaced)
			break
		}
	}

	// Check for any forbidden suffixes
	for _, suffix := range config.Users().ForbiddenUsernameSuffixes() {
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
func (r *Reconciler) provisionMasterUserRecord(logger logr.Logger, config toolchainconfig.ToolchainConfig, userSignup *toolchainv1alpha1.UserSignup, targetCluster targetCluster,
	userTier *toolchainv1alpha1.UserTier) error {

	// Set the last-target-cluster annotation so that if the user signs up again later on, they can be provisioned to the same cluster
	userSignup.Annotations[toolchainv1alpha1.UserSignupLastTargetClusterAnnotationKey] = targetCluster.getClusterName()
	if err := r.Client.Update(context.TODO(), userSignup); err != nil {
		return r.wrapErrorWithStatusUpdate(logger, userSignup, r.setStatusFailedToUpdateAnnotation, err,
			"unable to update last target cluster annotation on UserSignup resource")
	}

	compliantUsername, err := r.generateCompliantUsername(config, userSignup)
	if err != nil {
		return r.wrapErrorWithStatusUpdate(logger, userSignup, r.setStatusFailedToCreateMUR, err,
			"Error generating compliant username for %s", userSignup.Spec.Username)
	}

	mur := newMasterUserRecord(userSignup, targetCluster.getClusterName(), userTier.Name, compliantUsername)

	if err := controllerutil.SetControllerReference(userSignup, mur, r.Scheme); err != nil {
		return r.wrapErrorWithStatusUpdate(logger, userSignup, r.setStatusFailedToCreateMUR, err,
			"Error setting controller reference for MasterUserRecord %s", mur.Name)
	}

	logger.Info("Creating MasterUserRecord", "Name", mur.Name)
	err = r.Client.Create(context.TODO(), mur)
	if err != nil {
		return r.wrapErrorWithStatusUpdate(logger, userSignup, r.setStatusFailedToCreateMUR, err,
			"error creating MasterUserRecord")
	}
	// increment the counter of MasterUserRecords
	domain := metrics.GetEmailDomain(mur)
	counter.IncrementMasterUserRecordCount(logger, domain)

	logger.Info("Created MasterUserRecord", "Name", mur.Name, "TargetCluster", targetCluster)
	return nil
}

// ensureSpace does the work of provisioning the Space
func (r *Reconciler) ensureSpace(logger logr.Logger, userSignup *toolchainv1alpha1.UserSignup, mur *toolchainv1alpha1.MasterUserRecord, spaceTier *toolchainv1alpha1.NSTemplateTier) (*toolchainv1alpha1.Space, bool, error) {
	logger.Info("Ensuring Space", "UserSignup", userSignup.Name, "MUR", mur.Name)

	space := &toolchainv1alpha1.Space{}
	err := r.Client.Get(context.TODO(), types.NamespacedName{
		Namespace: userSignup.Namespace,
		Name:      mur.Name,
	}, space)
	if err == nil {
		if util.IsBeingDeleted(space) {
			return nil, false, fmt.Errorf("cannot create space because it is currently being deleted")
		}
		logger.Info("Space exists")
		return space, false, nil
	}

	if !errors.IsNotFound(err) {
		return nil, false, errs.Wrap(err, fmt.Sprintf(`failed to get Space associated with mur "%s"`, mur.Name))
	}

	if len(mur.Spec.UserAccounts) == 0 || mur.Spec.UserAccounts[0].TargetCluster == "" {
		return nil, false, fmt.Errorf("unable to get target cluster from masteruserrecord for space creation")
	}
	tCluster := targetCluster(mur.Spec.UserAccounts[0].TargetCluster)

	space = newSpace(userSignup, tCluster, mur.Name, spaceTier.Name)

	err = r.Client.Create(context.TODO(), space)
	if err != nil {
		return nil, false, r.wrapErrorWithStatusUpdate(logger, userSignup, r.setStatusFailedToCreateSpace, err,
			"error creating Space")
	}

	logger.Info("Created Space", "Name", space.Name, "TargetCluster", tCluster, "Tier", mur.Spec.TierName)
	return space, true, nil
}

func (r *Reconciler) ensureSpaceReady(logger logr.Logger, space *toolchainv1alpha1.Space) (bool, error) {
	logger.Info("Ensuring Space is Ready", "Space", space.Name)
	readyCond, ok := condition.FindConditionByType(space.Status.Conditions, toolchainv1alpha1.ConditionReady)
	if ok {
		if readyCond.Status == corev1.ConditionTrue && readyCond.Reason == toolchainv1alpha1.SpaceProvisionedReason {
			return true, nil
		}
		return false, nil
	}
	return false, fmt.Errorf("ready condition not found on space %s", space.Name)
}

// ensureSpaceBinding creates a SpaceBinding for the provided MUR and Space if one does not exist
func (r *Reconciler) ensureSpaceBinding(logger logr.Logger, userSignup *toolchainv1alpha1.UserSignup, mur *toolchainv1alpha1.MasterUserRecord, space *toolchainv1alpha1.Space) error {
	logger.Info("Ensuring SpaceBinding", "MUR", mur.Name, "Space", space.Name)

	spaceBindings := &toolchainv1alpha1.SpaceBindingList{}
	labels := map[string]string{
		toolchainv1alpha1.SpaceBindingMasterUserRecordLabelKey: mur.Name,
		toolchainv1alpha1.SpaceBindingSpaceLabelKey:            space.Name,
	}
	opts := client.MatchingLabels(labels)
	if err := r.Client.List(context.TODO(), spaceBindings, opts); err != nil {
		return errs.Wrap(err, fmt.Sprintf(`attempt to list SpaceBinding associated with mur '%s' and space '%s' failed`, mur.Name, space.Name))
	}

	if len(spaceBindings.Items) == 1 {
		spaceBinding := spaceBindings.Items[0]
		if util.IsBeingDeleted(&spaceBinding) {
			return fmt.Errorf("cannot create SpaceBinding because it is currently being deleted")
		}
		logger.Info("SpaceBinding already exists")
		return nil
	} else if len(spaceBindings.Items) > 1 {
		return fmt.Errorf(`unable to proceed because there are multiple SpaceBindings associated with MasterUserRecord '%s' and Space '%s'`, mur.Name, space.Name)
	}

	spaceBinding := newSpaceBinding(mur, space, userSignup.Name)

	if err := r.Client.Create(context.TODO(), spaceBinding); err != nil {
		return r.wrapErrorWithStatusUpdate(logger, userSignup, r.setStatusFailedToCreateSpaceBinding, err,
			"error creating SpaceBinding")
	}

	logger.Info("Created SpaceBinding", "MUR", mur.Name, "Space", space.Name)
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

// deleteMasterUserRecord deletes the specified MasterUserRecord
func (r *Reconciler) deleteMasterUserRecord(mur *toolchainv1alpha1.MasterUserRecord,
	userSignup *toolchainv1alpha1.UserSignup, logger logr.Logger,
	inProgressStatusUpdater StatusUpdaterFunc) error {

	logger.Info("Deleting MasterUserRecord", "MUR", mur.Name)
	err := r.updateStatus(logger, userSignup, inProgressStatusUpdater)
	if err != nil {
		return err
	}

	err = r.Client.Delete(context.TODO(), mur)
	if err != nil {
		return r.wrapErrorWithStatusUpdate(logger, userSignup, r.setStatusFailedToDeleteMUR, err,
			"error deleting MasterUserRecord")
	}
	logger.Info("Deleted MasterUserRecord", "MUR", mur.Name)
	return nil
}

func (r *Reconciler) sendDeactivatingNotification(logger logr.Logger, config toolchainconfig.ToolchainConfig, userSignup *toolchainv1alpha1.UserSignup) error {
	labels := map[string]string{
		toolchainv1alpha1.NotificationUserNameLabelKey: userSignup.Status.CompliantUsername,
		toolchainv1alpha1.NotificationTypeLabelKey:     toolchainv1alpha1.NotificationTypeDeactivating,
	}
	opts := client.MatchingLabels(labels)
	notificationList := &toolchainv1alpha1.NotificationList{}
	if err := r.Client.List(context.TODO(), notificationList, opts); err != nil {
		return err
	}

	// if there is no existing notification with these labels
	if len(notificationList.Items) == 0 {

		keysAndVals := map[string]string{
			toolchainconfig.NotificationContextRegistrationURLKey: config.RegistrationService().RegistrationServiceURL(),
		}

		notification, err := notify.NewNotificationBuilder(r.Client, userSignup.Namespace).
			WithTemplate(notificationtemplates.UserDeactivating.Name).
			WithNotificationType(toolchainv1alpha1.NotificationTypeDeactivating).
			WithControllerReference(userSignup, r.Scheme).
			WithUserContext(userSignup).
			WithKeysAndValues(keysAndVals).
			Create(userSignup.Annotations[toolchainv1alpha1.UserSignupUserEmailAnnotationKey])

		if err != nil {
			logger.Error(err, "Failed to create deactivating notification resource")
			return err
		}

		logger.Info(fmt.Sprintf("Deactivating notification resource [%s] created", notification.Name))
	}
	return nil
}

func (r *Reconciler) sendDeactivatedNotification(logger logr.Logger, config toolchainconfig.ToolchainConfig, userSignup *toolchainv1alpha1.UserSignup) error {
	labels := map[string]string{
		toolchainv1alpha1.NotificationUserNameLabelKey: userSignup.Status.CompliantUsername,
		toolchainv1alpha1.NotificationTypeLabelKey:     toolchainv1alpha1.NotificationTypeDeactivated,
	}
	opts := client.MatchingLabels(labels)
	notificationList := &toolchainv1alpha1.NotificationList{}
	if err := r.Client.List(context.TODO(), notificationList, opts); err != nil {
		return err
	}

	// if there is no existing notification with these labels
	if len(notificationList.Items) == 0 {
		keysAndVals := map[string]string{
			toolchainconfig.NotificationContextRegistrationURLKey: config.RegistrationService().RegistrationServiceURL(),
		}

		notification, err := notify.NewNotificationBuilder(r.Client, userSignup.Namespace).
			WithTemplate(notificationtemplates.UserDeactivated.Name).
			WithNotificationType(toolchainv1alpha1.NotificationTypeDeactivated).
			WithControllerReference(userSignup, r.Scheme).
			WithUserContext(userSignup).
			WithKeysAndValues(keysAndVals).
			Create(userSignup.Annotations[toolchainv1alpha1.UserSignupUserEmailAnnotationKey])

		if err != nil {
			logger.Error(err, "Failed to create deactivated notification resource")
			return err
		}

		logger.Info(fmt.Sprintf("Deactivated notification resource [%s] created", notification.Name))
	}
	return nil
}

// validateEmailHash calculates an md5 hash value for the provided userEmail string, and compares it to the provided
// userEmailHash.  If the values are the same the function returns true, otherwise it will return false
func validateEmailHash(userEmail, userEmailHash string) bool {
	md5hash := md5.New() //nolint:gosec
	// Ignore the error, as this implementation cannot return one
	_, _ = md5hash.Write([]byte(userEmail))
	return hex.EncodeToString(md5hash.Sum(nil)) == userEmailHash
}

func shouldManageSpace(userSignup *toolchainv1alpha1.UserSignup) bool {
	return userSignup.Annotations[toolchainv1alpha1.SkipAutoCreateSpaceAnnotationKey] != "true"
}
