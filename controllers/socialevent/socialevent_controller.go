package socialevent

import (
	"context"
	"fmt"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/go-logr/logr"

	errs "github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// Reconciler reconciles a SocialEvent object
type Reconciler struct {
	client.Client
	StatusUpdater *StatusUpdater
}

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&toolchainv1alpha1.SocialEvent{}, builder.WithPredicates(predicate.GenerationChangedPredicate{})).
		Complete(r)
}

//+kubebuilder:rbac:groups=toolchain.dev.openshift.com,resources=socialevents,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=toolchain.dev.openshift.com,resources=socialevents/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=toolchain.dev.openshift.com,resources=socialevents/finalizers,verbs=update

// Reconcile takes care of:
// - checking that the target tier specifiec in the SocialEvent is valid and set the status condition accordingly
func (r *Reconciler) Reconcile(ctx context.Context, request ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// fetch the SocialEvent resource
	event := &toolchainv1alpha1.SocialEvent{}
	if err := r.Client.Get(context.TODO(), request.NamespacedName, event); err != nil {
		if errors.IsNotFound(err) {
			logger.Info("SocialEvent not found")
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		logger.Error(err, "unable to get the current SocialEvent")
		return reconcile.Result{}, errs.Wrap(err, "unable to get the current NSTeSocialEventplateTier")
	}

	if err := r.checkTier(logger, event); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *Reconciler) checkTier(logger logr.Logger, event *toolchainv1alpha1.SocialEvent) error {
	// fetch the NSTemplateTier tier specified in the SocialEvent
	tier := &toolchainv1alpha1.NSTemplateTier{}
	if err := r.Client.Get(context.TODO(), types.NamespacedName{
		Namespace: event.Namespace,
		Name:      event.Spec.Tier,
	}, tier); err != nil {
		if errors.IsNotFound(err) {
			logger.Info("NSTemplateTier not found", "nstemplatetier_name", event.Spec.Tier)
			return r.StatusUpdater.updateStatusConditions(event, toolchainv1alpha1.Condition{
				Type:    toolchainv1alpha1.ConditionReady,
				Status:  corev1.ConditionFalse,
				Reason:  toolchainv1alpha1.SocialEventInvalidTierReason,
				Message: fmt.Sprintf("NSTemplateTier '%s' not found", event.Spec.Tier),
			})
		}
		// Error reading the object - requeue the request.
		logger.Error(err, "unable to get the NSTemplateTier", "nstemplatetier_name", event.Spec.Tier)
		return errs.Wrapf(err, "unable to get the '%s' NSTemplateTier", event.Spec.Tier)
	}
	return r.StatusUpdater.updateStatusConditions(event, toolchainv1alpha1.Condition{
		Type:   toolchainv1alpha1.ConditionReady,
		Status: corev1.ConditionTrue,
	})
}
