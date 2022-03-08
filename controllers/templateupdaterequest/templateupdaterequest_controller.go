package templateupdaterequest

import (
	"context"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	tierutil "github.com/codeready-toolchain/host-operator/controllers/nstemplatetier/util"
	"github.com/codeready-toolchain/toolchain-common/pkg/condition"
	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/source"

	errs "github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr manager.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&toolchainv1alpha1.TemplateUpdateRequest{}, builder.WithPredicates(predicate.GenerationChangedPredicate{})).
		Watches(&source.Kind{Type: &toolchainv1alpha1.MasterUserRecord{}}, &handler.EnqueueRequestForObject{}).
		Complete(r)
}

// Reconciler reconciles a TemplateUpdateRequest object
type Reconciler struct {
	Client client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=toolchain.dev.openshift.com,resources=templateupdaterequests,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=toolchain.dev.openshift.com,resources=templateupdaterequests/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=toolchain.dev.openshift.com,resources=templateupdaterequests/finalizers,verbs=update

// Reconcile reads that state of the cluster for a TemplateUpdateRequest object and makes changes based on the state read
// and what is in the TemplateUpdateRequest.Spec
// Note:
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *Reconciler) Reconcile(ctx context.Context, request ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("Reconciling TemplateUpdateRequest")

	// Fetch the TemplateUpdateRequest tur
	tur := &toolchainv1alpha1.TemplateUpdateRequest{}
	err := r.Client.Get(context.TODO(), request.NamespacedName, tur)
	if err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		logger.Error(err, "unable to get the current TemplateUpdateRequest")
		return reconcile.Result{}, errs.Wrap(err, "unable to get the current TemplateUpdateRequest")
	}

	return r.handleSpaceUpdate(logger, request, tur)
}

func (r *Reconciler) handleSpaceUpdate(logger logr.Logger, request ctrl.Request, tur *toolchainv1alpha1.TemplateUpdateRequest) (ctrl.Result, error) {
	// lookup the Space with the same name as the TemplateUpdateRequest tur
	space := &toolchainv1alpha1.Space{}
	if err := r.Client.Get(context.TODO(), request.NamespacedName, space); err != nil {
		if errors.IsNotFound(err) {
			// Space object not found, could have been deleted after reconcile request.
			// Marking this TemplateUpdateRequest as failed
			return reconcile.Result{}, r.addFailureStatusCondition(tur, err)
		}
		// Error reading the object - requeue the request.
		logger.Error(err, "unable to get the Space associated with the TemplateUpdateRequest")
		return reconcile.Result{}, errs.Wrap(err, "unable to get the Space associated with the TemplateUpdateRequest")
	}

	labelKey := tierutil.TemplateTierHashLabelKey(space.Spec.TierName)
	// if the tier hash has changed and the Space is in ready state then the update is complete
	if tur.Spec.CurrentTierHash != space.Labels[labelKey] && condition.IsTrue(space.Status.Conditions, toolchainv1alpha1.ConditionReady) {
		// once the Space is up-to-date, we can delete this TemplateUpdateRequest
		logger.Info("Space is up-to-date. Marking the TemplateUpdateRequest as complete")
		return reconcile.Result{}, r.setCompleteStatusCondition(tur)
	}

	// otherwise, we need to wait
	logger.Info("Space still being updated...")
	if err := r.addUpdatingStatusCondition(tur, map[string]string{}); err != nil {
		logger.Error(err, "Unable to update the TemplateUpdateRequest status")
		return reconcile.Result{}, errs.Wrap(err, "unable to update the TemplateUpdateRequest status")
	}
	// no explicit requeue: expect new reconcile loop when Space changes
	return reconcile.Result{}, nil
}

// --------------------------------------------------
// status updates
// --------------------------------------------------

// ToFailure condition when an error occurred
func ToFailure(err error) toolchainv1alpha1.Condition {
	return toolchainv1alpha1.Condition{
		Type:    toolchainv1alpha1.TemplateUpdateRequestComplete,
		Status:  corev1.ConditionFalse,
		Reason:  toolchainv1alpha1.TemplateUpdateRequestUnableToUpdateReason,
		Message: err.Error(),
	}
}

// ToBeUpdating condition when the update is in progress
func ToBeUpdating() toolchainv1alpha1.Condition {
	return toolchainv1alpha1.Condition{
		Type:               toolchainv1alpha1.TemplateUpdateRequestComplete,
		Status:             corev1.ConditionFalse,
		Reason:             toolchainv1alpha1.TemplateUpdateRequestUpdatingReason,
		LastTransitionTime: metav1.Now(),
	}
}

// ToBeComplete condition when the update completed with success
func ToBeComplete() toolchainv1alpha1.Condition {
	return toolchainv1alpha1.Condition{
		Type:               toolchainv1alpha1.TemplateUpdateRequestComplete,
		Status:             corev1.ConditionTrue,
		Reason:             toolchainv1alpha1.TemplateUpdateRequestUpdatedReason,
		LastTransitionTime: metav1.Now(),
	}
}

// addUpdatingStatusCondition sets the TemplateUpdateRequest status condition to `complete=false/reason=updating` and retains the sync indexes
func (r *Reconciler) addUpdatingStatusCondition(tur *toolchainv1alpha1.TemplateUpdateRequest, syncIndexes map[string]string) error {
	tur.Status.SyncIndexes = syncIndexes
	tur.Status.Conditions = []toolchainv1alpha1.Condition{ToBeUpdating()}
	return r.Client.Status().Update(context.TODO(), tur)
}

// addFailureStatusCondition appends a new TemplateUpdateRequest status condition to `complete=false/reason=updating`
func (r *Reconciler) addFailureStatusCondition(tur *toolchainv1alpha1.TemplateUpdateRequest, err error) error {
	tur.Status.Conditions = condition.AddStatusConditions(tur.Status.Conditions, ToFailure(err))
	return r.Client.Status().Update(context.TODO(), tur)
}

// setCompleteStatusCondition sets the TemplateUpdateRequest status condition to `complete=true/reason=updated` and clears all previous conditions of the same type
func (r *Reconciler) setCompleteStatusCondition(tur *toolchainv1alpha1.TemplateUpdateRequest) error {
	tur.Status.Conditions = []toolchainv1alpha1.Condition{ToBeComplete()}
	return r.Client.Status().Update(context.TODO(), tur)
}
