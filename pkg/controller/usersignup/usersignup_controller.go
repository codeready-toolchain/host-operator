package usersignup

import (
	"context"
	toolchainv1alpha1 "github.com/codeready-toolchain/api/pkg/apis/toolchain/v1alpha1"
	"github.com/codeready-toolchain/host-operator/pkg/config"
	"github.com/codeready-toolchain/toolchain-common/pkg/cluster"
	commonCondition "github.com/codeready-toolchain/toolchain-common/pkg/condition"
	"github.com/go-logr/logr"
	"github.com/operator-framework/operator-sdk/pkg/predicate"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

const (
	// Status condition reasons
	noClustersAvailableReason = "NoClustersAvailable"
	failedToReadUserApprovalPolicyReason = "FailedToReadUserApprovalPolicy"
	unableToCreateMURReason = "UnableToCreateMUR"
	approvedAutomaticallyReason = "ApprovedAutomatically"
	approvedByAdminReason = "ApprovedByAdmin"
	pendingApprovalReason = "PendingApproval"
)

var log = logf.Log.WithName("controller_usersignup")

// Add creates a new UserSignup Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileUserSignup{client: mgr.GetClient(), scheme: mgr.GetScheme(), clientset: kubernetes.NewForConfigOrDie(mgr.GetConfig())}
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
		predicate.GenerationChangedPredicate{})
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

	return nil
}

var _ reconcile.Reconciler = &ReconcileUserSignup{}

func NewSignupError(msg string) SignupError {
	return SignupError{message: msg}
}

type SignupError struct {
	message string
}

func (err SignupError) Error() string {
	return err.message
}

// ReconcileUserSignup reconciles a UserSignup object
type ReconcileUserSignup struct {
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the apiserver
	client client.Client
	scheme *runtime.Scheme
	clientset kubernetes.Interface
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

	// Check if the MasterUserRecord already exists
	mur := &toolchainv1alpha1.MasterUserRecord{}
	// We can use the same NamespacedName from the request as the MasterUserRecord will have the same name as the
	// UserSignup (i.e. the user's username)
	err = r.client.Get(context.TODO(), request.NamespacedName, mur)
	if err != nil {
		// We generally EXPECT the MasterUserRecord to not be found here, so we only deal with other error types
		if !errors.IsNotFound(err) {
			// Error reading the MasterUserRecord, requeue the request
			return reconcile.Result{}, err
		}
	} else {
		// If we successfully found an existing MasterUserRecord then our work here is done, set the status
		// to Complete and return
		statusError := r.setStatusComplete(instance, "")
		if statusError != nil {
			reqLogger.Error(statusError, "Error updating UserSignup Status to Complete", "Name", instance.Name)
			return reconcile.Result{}, statusError
		}
		return reconcile.Result{}, nil
	}

	// Check the user approval policy.
	userApprovalPolicy, err := r.ReadUserApprovalPolicyConfig()
	if err != nil {
		reqLogger.Error(err, "Error reading user approval policy")
		statusError := r.setStatusFailedToReadUserApprovalPolicy(instance, err.Error())
		if statusError != nil {
			reqLogger.Error(statusError, "Error updating UserSignup Status", "Name", instance.Name)
			return reconcile.Result{}, statusError
		}

		return reconcile.Result{}, err
	}

	// If the signup has been explicitly approved (by an admin), or the user approval policy is set to automatic,
	// then proceed with the signup
	if instance.Spec.Approved || userApprovalPolicy == config.UserApprovalPolicyAutomatic {
		if instance.Spec.Approved {
			statusError := r.setStatusApprovedByAdmin(instance, "")
			if statusError != nil {
				reqLogger.Error(statusError, "Error updating UserSignup Status", "Name", instance.Name)
				return reconcile.Result{}, statusError
			}
		} else {
			statusError := r.setStatusApprovedAutomatically(instance, "")
			if statusError != nil {
				reqLogger.Error(statusError, "Error updating UserSignup Status", "Name", instance.Name)
				return reconcile.Result{}, statusError
			}
		}

		var targetCluster string

		// If a target cluster hasn't been selected, select one from the members
		if instance.Spec.TargetCluster != "" {
			targetCluster = instance.Spec.TargetCluster
		} else {
			// Automatic cluster selection
			members := cluster.GetMemberClusters()
			if len(members) > 0 {
				targetCluster = members[0].Name
			} else {
				reqLogger.Error(err, "No member clusters found")
				statusError := r.setStatusNoClustersAvailable(instance, "No member clusters found")
				if statusError != nil {
					reqLogger.Error(statusError, "Error updating UserSignup Status", "UserID", instance.Spec.UserID)
					return reconcile.Result{}, statusError
				}

				err = NewSignupError("No target clusters available")
				return reconcile.Result{}, err
			}
		}

		// Provision the MasterUserRecord
		err = r.provisionMasterUserRecord(instance, targetCluster, reqLogger)
		if err != nil {
			return reconcile.Result{}, err
		}
	} else {
		statusError := r.setStatusPendingApproval(instance, "")
		if statusError != nil {
			reqLogger.Error(statusError, "Error updating UserSignup Status", "UserID", instance.Spec.UserID)
			return reconcile.Result{}, statusError
		}
	}

	return reconcile.Result{}, nil
}

// provisionMasterUserRecord does the work of provisioning the MasterUserRecord
func (r *ReconcileUserSignup) provisionMasterUserRecord(userSignup *toolchainv1alpha1.UserSignup, targetCluster string, logger logr.Logger) error {
	userAccounts := []toolchainv1alpha1.UserAccountEmbedded{
		{
			TargetCluster: targetCluster,
			Spec: toolchainv1alpha1.UserAccountSpec{
				NSTemplateSet: toolchainv1alpha1.NSTemplateSetSpec{
					Namespaces: []toolchainv1alpha1.Namespace{},
				},
			},
		},
	}

	// TODO Update the MasterUserRecord with NSTemplateTier values
	// SEE https://jira.coreos.com/browse/CRT-74

	mur := &toolchainv1alpha1.MasterUserRecord{
		ObjectMeta: metav1.ObjectMeta{
			Name: userSignup.Spec.UserID,
			Namespace: userSignup.Namespace,
		},
		Spec: toolchainv1alpha1.MasterUserRecordSpec{
			UserID: userSignup.Spec.UserID,
			UserAccounts: userAccounts,
		},
	}

	err := controllerutil.SetControllerReference(mur, userSignup, r.scheme)
	if err != nil {
		logger.Error(err, "Error setting controller reference for MasterUserRecord %s", mur.Name)

		statusError := r.setStatusFailedToCreateMUR(userSignup, "failed to set controller reference: " + err.Error())
		if statusError != nil {
			logger.Error(statusError, "Error setting MUR failed status")
			return statusError
		}
		return err
	}

	err = r.client.Create(context.TODO(), mur)
	if err != nil {
		// If the MasterUserRecord already exists (for whatever strange reason), we simply set the status to complete
		if errors.IsAlreadyExists(err) {
			logger.Info("MasterUserRecord already exists, setting status to Complete")
			statusError := r.setStatusComplete(userSignup, "")
			if statusError != nil {
				logger.Error(statusError, "Error setting MUR failed status")
				return statusError
			}

			return nil

		} else {
			logger.Error(err, "Error creating MasterUserRecord", "Name", userSignup.Name)
			statusError := r.setStatusFailedToCreateMUR(userSignup, "Failed to create MasterUserRecord: " + err.Error())
			if statusError != nil {
				logger.Error(statusError, "Error setting MUR failed status")
				return statusError
			}

			return err
		}
	}

	logger.Info("Created MasterUserRecord", "Name", userSignup.Name, "TargetCluster", targetCluster)
	return nil
}

// ReadUserApprovalPolicyConfig reads the ConfigMap for the toolchain configuration in the operator namespace, and returns
// the config map value for the user approval policy (which will either be "manual" or "automatic")
func (r *ReconcileUserSignup) ReadUserApprovalPolicyConfig() (string, error) {
	cm, err := r.clientset.CoreV1().ConfigMaps(config.GetOperatorNamespace()).Get(config.ToolchainConfigMapName, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			return config.UserApprovalPolicyManual, nil
		}
		return "", err
	}

	return cm.Data[config.ToolchainConfigMapUserApprovalPolicy], nil
}

func (r *ReconcileUserSignup) setStatusApprovedAutomatically(userSignup *toolchainv1alpha1.UserSignup, message string) error {
	return r.updateStatusConditions(
		userSignup,
		toolchainv1alpha1.Condition{
			Type: toolchainv1alpha1.UserSignupApproved,
			Status: corev1.ConditionTrue,
			Reason: approvedAutomaticallyReason,
			Message: message,
		})
}

func (r *ReconcileUserSignup) setStatusApprovedByAdmin(userSignup *toolchainv1alpha1.UserSignup, message string) error {
	return r.updateStatusConditions(
		userSignup,
		toolchainv1alpha1.Condition{
			Type: toolchainv1alpha1.UserSignupApproved,
			Status: corev1.ConditionTrue,
			Reason: approvedByAdminReason,
			Message: message,
		})
}

func (r *ReconcileUserSignup) setStatusPendingApproval(userSignup *toolchainv1alpha1.UserSignup, message string) error {
	return r.updateStatusConditions(
		userSignup,
		toolchainv1alpha1.Condition{
			Type: toolchainv1alpha1.UserSignupApproved,
			Status: corev1.ConditionFalse,
			Reason: pendingApprovalReason,
			Message: message,
		},
		toolchainv1alpha1.Condition{
			Type: toolchainv1alpha1.UserSignupComplete,
			Status: corev1.ConditionFalse,
			Reason: pendingApprovalReason,
			Message: message,
		})

}

func (r *ReconcileUserSignup) setStatusFailedToReadUserApprovalPolicy(userSignup *toolchainv1alpha1.UserSignup, message string) error {
	return r.updateStatusConditions(
		userSignup,
		toolchainv1alpha1.Condition{
			Type: toolchainv1alpha1.UserSignupComplete,
			Status: corev1.ConditionFalse,
			Reason: failedToReadUserApprovalPolicyReason,
			Message: message,
		})
}

func (r *ReconcileUserSignup) setStatusFailedToCreateMUR(userSignup *toolchainv1alpha1.UserSignup, message string) error {
	return r.updateStatusConditions(
		userSignup,
		toolchainv1alpha1.Condition{
			Type: toolchainv1alpha1.UserSignupComplete,
			Status: corev1.ConditionFalse,
			Reason: unableToCreateMURReason,
			Message: message,
		})
}

func (r *ReconcileUserSignup) setStatusNoClustersAvailable(userSignup *toolchainv1alpha1.UserSignup, message string) error {
	return r.updateStatusConditions(
		userSignup,
		toolchainv1alpha1.Condition{
			Type: toolchainv1alpha1.UserSignupComplete,
			Status: corev1.ConditionFalse,
			Reason: noClustersAvailableReason,
			Message: message,
		})
}

func (r *ReconcileUserSignup) setStatusComplete(userSignup *toolchainv1alpha1.UserSignup, message string) error {
	return r.updateStatusConditions(
		userSignup,
		toolchainv1alpha1.Condition{
			Type: toolchainv1alpha1.UserSignupComplete,
			Status: corev1.ConditionTrue,
			Reason: "",
			Message: message,
		})
}

func (r *ReconcileUserSignup) updateStatusConditions(userSignup *toolchainv1alpha1.UserSignup, newConditions ...toolchainv1alpha1.Condition) error {
	var updated bool
	userSignup.Status.Conditions, updated = commonCondition.AddOrUpdateStatusConditions(userSignup.Status.Conditions, newConditions...)
	if !updated {
		// Nothing changed
		return nil
	}
	return r.client.Status().Update(context.TODO(), userSignup)
}
