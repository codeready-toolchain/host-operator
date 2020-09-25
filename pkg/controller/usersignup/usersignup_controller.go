package usersignup

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/pkg/apis/toolchain/v1alpha1"
	"github.com/codeready-toolchain/host-operator/pkg/configuration"
	"github.com/codeready-toolchain/host-operator/pkg/counter"
	"github.com/codeready-toolchain/toolchain-common/pkg/cluster"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/validation"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
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
func Add(mgr manager.Manager, _ *configuration.Config) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileUserSignup{
		statusUpdater: &statusUpdater{
			client: mgr.GetClient(),
		},
		scheme:            mgr.GetScheme(),
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
	err = c.Watch(&source.Kind{Type: &toolchainv1alpha1.UserSignup{}}, &handler.EnqueueRequestForObject{},
		UserSignupChangedPredicate{})
	if err != nil {
		return err
	}

	// Watch for changes to the secondary resource MasterUserRecord and requeue the owner UserSignup
	err = c.Watch(&source.Kind{Type: &toolchainv1alpha1.MasterUserRecord{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &toolchainv1alpha1.UserSignup{},
	})
	if err != nil {
		return err
	}

	err = c.Watch(&source.Kind{Type: &toolchainv1alpha1.BannedUser{}}, &handler.EnqueueRequestsFromMapFunc{
		ToRequests: BannedUserToUserSignupMapper{client: mgr.GetClient()},
	})

	if err != nil {
		return err
	}

	return nil
}

var _ reconcile.Reconciler = &ReconcileUserSignup{}

// ReconcileUserSignup reconciles a UserSignup object
type ReconcileUserSignup struct {
	*statusUpdater
	scheme            *runtime.Scheme
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

	banned, err := r.isUserBanned(reqLogger, instance)
	if err != nil {
		return reconcile.Result{}, err
	}

	if exists, err := r.ensureMurIfAlreadyExists(reqLogger, instance, banned); exists || err != nil {
		return reconcile.Result{}, err
	}

	// If there is no MasterUserRecord created, yet the UserSignup is Banned, simply set the status
	// and return
	if banned {
		return reconcile.Result{}, r.updateStatus(reqLogger, instance, r.setStatusBanned)
	}

	// If there is no MasterUserRecord created, yet the UserSignup is marked as Deactivated, simply set the status
	// and return
	if instance.Spec.Deactivated {
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
			return true, r.DeleteMasterUserRecord(mur, userSignup, reqLogger, r.setStatusBanning, r.setStatusFailedToDeleteMUR)
		}

		// If the user has been deactivated, then we need to delete the MUR
		if userSignup.Spec.Deactivated {
			return true, r.DeleteMasterUserRecord(mur, userSignup, reqLogger, r.setStatusDeactivating, r.setStatusFailedToDeleteMUR)
		}

		// look-up the `basic` NSTemplateTier to get the NS templates
		nstemplateTier, err := getNsTemplateTier(r.client, defaultTierName, userSignup.Namespace)
		if err != nil {
			return true, r.wrapErrorWithStatusUpdate(reqLogger, userSignup, r.setStatusNoTemplateTierAvailable, err, "")
		}

		// if the UserSignup doesn't have the approved label set, then update it
		if err := r.setApprovedLabel(reqLogger, userSignup, toolchainv1alpha1.UserSignupApprovedLabelValueTrue); err != nil {
			return true, err
		}

		// check if anything in the MUR chould be migrated/fixed
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
	// Check the user approval policy.
	userApprovalPolicy, err := r.ReadUserApprovalPolicyConfig(userSignup.Namespace)
	if err != nil {
		return r.wrapErrorWithStatusUpdate(reqLogger, userSignup, r.setStatusFailedToReadUserApprovalPolicy, err, "")
	}

	// If the signup has been explicitly approved (by an admin), or the user approval policy is set to automatic,
	// then proceed with the signup
	if userSignup.Spec.Approved || userApprovalPolicy == configuration.UserApprovalPolicyAutomatic {
		if userSignup.Spec.Approved {
			if statusError := r.updateStatus(reqLogger, userSignup, r.setStatusApprovedByAdmin); statusError != nil {
				return statusError
			}
		} else {
			if statusError := r.updateStatus(reqLogger, userSignup, r.setStatusApprovedAutomatically); statusError != nil {
				return statusError
			}
		}

		// Check if the user requires phone verification, and do not proceed further if they do
		if userSignup.Spec.VerificationRequired {
			return r.updateStatus(reqLogger, userSignup, r.setStatusVerificationRequired)
		}

		var targetCluster string

		// If a target cluster hasn't been selected, select one from the members
		if userSignup.Spec.TargetCluster != "" {
			targetCluster = userSignup.Spec.TargetCluster
		} else {
			// Automatic cluster selection based on cluster readiness
			members := r.getMemberClusters(cluster.Ready, cluster.CapacityNotExhausted)
			if len(members) > 0 {
				targetCluster = members[0].Name
			} else {
				reqLogger.Error(err, "No member clusters found")
				if statusError := r.updateStatus(reqLogger, userSignup, r.setStatusNoClustersAvailable); statusError != nil {
					return statusError
				}

				return fmt.Errorf("no target clusters available")
			}
		}

		// look-up the `basic` NSTemplateTier to get the NS templates
		nstemplateTier, err := getNsTemplateTier(r.client, defaultTierName, userSignup.Namespace)
		if err != nil {
			return r.wrapErrorWithStatusUpdate(reqLogger, userSignup, r.setStatusNoTemplateTierAvailable, err, "")
		}
		if err := r.setApprovedLabel(reqLogger, userSignup, toolchainv1alpha1.UserSignupApprovedLabelValueTrue); err != nil {
			return err
		}

		// Provision the MasterUserRecord
		err = r.provisionMasterUserRecord(userSignup, targetCluster, nstemplateTier, reqLogger)
		if err != nil {
			return err
		}
	} else {
		return r.updateStatus(reqLogger, userSignup, r.setStatusPendingApproval)
	}
	return nil
}

func (r *ReconcileUserSignup) setApprovedLabel(reqLogger logr.Logger, userSignup *toolchainv1alpha1.UserSignup, value string) error {
	if userSignup.Labels[toolchainv1alpha1.UserSignupApprovedLabelKey] != value {
		userSignup.Labels[toolchainv1alpha1.UserSignupApprovedLabelKey] = value
		if err := r.client.Update(context.TODO(), userSignup); err != nil {
			return r.wrapErrorWithStatusUpdate(reqLogger, userSignup, r.setStatusFailedToUpdateApprovedLabel, err,
				"unable to update approved label at UserSignup resource")
		}
	}
	return nil
}

func getNsTemplateTier(cl client.Client, tierName, namespace string) (*toolchainv1alpha1.NSTemplateTier, error) {
	nstemplateTier := &toolchainv1alpha1.NSTemplateTier{}
	err := cl.Get(context.TODO(), types.NamespacedName{Namespace: namespace, Name: tierName}, nstemplateTier)
	return nstemplateTier, err
}

func (r *ReconcileUserSignup) generateCompliantUsername(instance *toolchainv1alpha1.UserSignup) (string, error) {
	replaced := transformUsername(instance.Spec.Username)
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
			return "", fmt.Errorf(fmt.Sprintf("could not generate compliant username as MasterUserRecord [%s] already exists", mur.Name))
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
	if err := r.setApprovedLabel(logger, userSignup, toolchainv1alpha1.UserSignupApprovedLabelValueFalse); err != nil {
		return err
	}

	err = r.client.Delete(context.TODO(), mur)
	if err != nil {
		return r.wrapErrorWithStatusUpdate(logger, userSignup, failedStatusUpdater, err,
			"Error deleting MasterUserRecord")
	}
	logger.Info("Deleted MasterUserRecord", "Name", mur.Name)
	return nil
}

// ReadUserApprovalPolicyConfig reads the ConfigMap for the toolchain configuration in the operator namespace, and returns
// the config map value for the user approval policy (which will either be "manual" or "automatic")
func (r *ReconcileUserSignup) ReadUserApprovalPolicyConfig(namespace string) (string, error) {
	cm := &corev1.ConfigMap{}
	err := r.client.Get(context.TODO(), types.NamespacedName{Namespace: namespace, Name: configuration.ToolchainConfigMapName}, cm)
	if err != nil {
		if errors.IsNotFound(err) {
			return configuration.UserApprovalPolicyManual, nil
		}
		return "", err
	}

	val, ok := cm.Data[configuration.ToolchainConfigMapUserApprovalPolicy]
	if !ok {
		return "", nil
	}
	return val, nil
}

// validateEmailHash calculates an md5 hash value for the provided userEmail string, and compares it to the provided
// userEmailHash.  If the values are the same the function returns true, otherwise it will return false
func validateEmailHash(userEmail, userEmailHash string) bool {
	md5hash := md5.New()
	// Ignore the error, as this implementation cannot return one
	_, _ = md5hash.Write([]byte(userEmail))
	return hex.EncodeToString(md5hash.Sum(nil)) == userEmailHash
}
