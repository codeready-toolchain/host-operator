package usersignup

import (
	"context"
	toolchainv1alpha1 "github.com/codeready-toolchain/api/pkg/apis/toolchain/v1alpha1"
	commonCondition "github.com/codeready-toolchain/toolchain-common/pkg/condition"
	"github.com/codeready-toolchain/host-operator/pkg/condition"
	"github.com/codeready-toolchain/host-operator/pkg/config"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"sigs.k8s.io/controller-runtime/pkg/source"
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
	err = c.Watch(&source.Kind{Type: &toolchainv1alpha1.UserSignup{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	// Watch for changes to secondary resource Pods and requeue the owner UserSignup
	err = c.Watch(&source.Kind{Type: &corev1.Pod{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &toolchainv1alpha1.UserSignup{},
	})
	if err != nil {
		return err
	}

	return nil
}

var _ reconcile.Reconciler = &ReconcileUserSignup{}

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

	// Read the status conditions and assign some variables
	provisioning := condition.IsConditionTrue(instance.Status.Conditions, toolchainv1alpha1.UserSignupProvisioning)
	completed := condition.IsConditionTrue(instance.Status.Conditions, toolchainv1alpha1.UserSignupComplete)

	// If the status is not yet provisioning or completed, check for automatic or manual approval
	if !provisioning && !completed {
		approved := instance.Spec.Approved

		if !approved {
			autoApprovalPolicy, err := r.ReadAutoApproveConfig()
			if err != nil {
				return reconcile.Result{}, err
			}

			approved = autoApprovalPolicy == config.UserApprovalPolicyAutomatic
		}

		if approved {
			// If a target cluster hasn't been selected, select one from the members

			var targetCluster string

			if instance.Spec.TargetCluster != "" {
				targetCluster = instance.Spec.TargetCluster
			} else {
				// TODO implement cluster selection
				targetCluster = ""
			}

			// Provision the MasterUserRecord
			userAccounts := []toolchainv1alpha1.UserAccountEmbedded{
				{
					TargetCluster: targetCluster,
				},
			}

			mur := &toolchainv1alpha1.MasterUserRecord{
				Spec: toolchainv1alpha1.MasterUserRecordSpec{
					UserID: instance.Spec.UserID,
					UserAccounts: userAccounts,
				},
			}

			err := r.client.Create(context.TODO(), mur)
			if err != nil {
				// Error creating the MasterUserRecord - requeue the request.
				reqLogger.Error(err, "Error creating MasterUserRecord", "UserID", instance.Spec.UserID, "TargetCluster", targetCluster)
				return reconcile.Result{}, err
			}
			reqLogger.Info("Created MasterUserRecord", "UserID", instance.Spec.UserID, "TargetCluster", targetCluster)

			// Update the UserSignup status conditions: set PendingApproval to false, Provisioning to true
			updatedConditions, updated := commonCondition.AddOrUpdateStatusConditions(instance.Status.Conditions, toolchainv1alpha1.Condition{
				Type: toolchainv1alpha1.UserSignupPendingApproval,
				Message: "Approved",
				Status: corev1.ConditionFalse,
				Reason: "",
			},
				toolchainv1alpha1.Condition{
					Type: toolchainv1alpha1.UserSignupProvisioning,
					Message: "Provisioning",
					Status: corev1.ConditionTrue,
					Reason: "",
				})

			// If the condition values were updated, post the changes
			if updated {
				instance.Status.Conditions = updatedConditions
				err = r.client.Update(context.TODO(), instance)
				if err != nil {
					reqLogger.Error(err, "Error updating UserSignup Status conditions", "UserID", instance.Spec.UserID)
					// Error updating the status - requeue the request.
					return reconcile.Result{}, err
				}
			}
		}
	}

	return reconcile.Result{}, nil
}

// ReadAutoApproveConfig reads the ConfigMap for the toolchain configuration in the operator namespace, and returns
// the config map value for the user approval policy (which will either be "manual" or "automatic")
func (r *ReconcileUserSignup) ReadAutoApproveConfig() (string, error) {


