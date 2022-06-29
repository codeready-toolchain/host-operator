package socialevent

import (
	"context"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	commoncontrollers "github.com/codeready-toolchain/toolchain-common/controllers"

	"github.com/go-logr/logr"
	errs "github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

// Reconciler reconciles a SocialEvent object
type Reconciler struct {
	client.Client
	Namespace     string
	StatusUpdater *StatusUpdater
}

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&toolchainv1alpha1.SocialEvent{}, builder.WithPredicates(predicate.GenerationChangedPredicate{})).
		Watches(
			// watches UserSignups when their labels are *changed* (in particular, to track the approved users)
			// and they have a `toolchain.dev.openshift.com/social-event` label
			&source.Kind{Type: &toolchainv1alpha1.UserSignup{}},
			handler.EnqueueRequestsFromMapFunc(commoncontrollers.MapToOwnerByLabel(r.Namespace, toolchainv1alpha1.SocialEventUserSignupLabelKey)),
			builder.WithPredicates(predicate.LabelChangedPredicate{}),
		).Complete(r)
}

//+kubebuilder:rbac:groups=toolchain.dev.openshift.com,resources=socialevents,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=toolchain.dev.openshift.com,resources=socialevents/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=toolchain.dev.openshift.com,resources=socialevents/finalizers,verbs=update

// Reconcile takes care of:
// - checking that the target User and Space tiers specified in the SocialEvent are valid and set the status condition accordingly
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
		return reconcile.Result{}, errs.Wrap(err, "unable to get the current SocialEvent")
	}

	if err := r.checkTier(logger, event); err != nil {
		return ctrl.Result{}, err
	}

	// count the number of UserSignups with the `toolchain.dev.openshift.com/social-event` label matching the SocialEvent.Name
	// and update the SocialEvent.Status.ActivationCount accordingly
	usersignups := &toolchainv1alpha1.UserSignupList{}
	if err := r.Client.List(context.TODO(), usersignups,
		client.InNamespace(r.Namespace),
		client.MatchingLabels{
			toolchainv1alpha1.SocialEventUserSignupLabelKey: event.Name,
			toolchainv1alpha1.UserSignupStateLabelKey:       toolchainv1alpha1.UserSignupStateLabelValueApproved,
		}); err != nil {
		return reconcile.Result{}, errs.Wrapf(err, "unable to list UserSignups with '%s' label", toolchainv1alpha1.SocialEventUserSignupLabelKey)
	}
	logger.Info("approved usersignups with activation code", "count", len(usersignups.Items))
	event.Status.ActivationCount = len(usersignups.Items)
	if err := r.Client.Status().Update(context.TODO(), event); err != nil {
		return reconcile.Result{}, errs.Wrap(err, "unable to update status with activation count")
	}
	return reconcile.Result{}, nil
}

func (r *Reconciler) checkTier(logger logr.Logger, event *toolchainv1alpha1.SocialEvent) error {
	// fetch the UserTier specified in the SocialEvent
	userTier := &toolchainv1alpha1.UserTier{}
	if err := r.Client.Get(context.TODO(), types.NamespacedName{
		Namespace: event.Namespace,
		Name:      event.Spec.UserTier,
	}, userTier); err != nil {
		if errors.IsNotFound(err) {
			return r.StatusUpdater.userTierNotFound(logger, event)
		}
		// Error reading the object - requeue the request.
		return r.StatusUpdater.unableToGetUserTier(logger, event, err)
	}

	// fetch the NSTemplateTier specified in the SocialEvent
	spaceTier := &toolchainv1alpha1.NSTemplateTier{}
	if err := r.Client.Get(context.TODO(), types.NamespacedName{
		Namespace: event.Namespace,
		Name:      event.Spec.SpaceTier,
	}, spaceTier); err != nil {
		if errors.IsNotFound(err) {
			return r.StatusUpdater.spaceTierNotFound(logger, event)
		}
		// Error reading the object - requeue the request.
		return r.StatusUpdater.unableToGetSpaceTier(logger, event, err)
	}
	return r.StatusUpdater.ready(event)
}
