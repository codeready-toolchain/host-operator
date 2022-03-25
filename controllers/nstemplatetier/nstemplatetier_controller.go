package nstemplatetier

import (
	"context"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	tierutil "github.com/codeready-toolchain/host-operator/controllers/nstemplatetier/util"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/go-logr/logr"
	errs "github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// ----------------------------------------------------------------------------------------------------------------------------
// NSTemplateTier Controller Reconciler:
// . in case of a new NSTemplateTier update to process:
// .. inserts a new record in the `status.updates` history
// ----------------------------------------------------------------------------------------------------------------------------

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr manager.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&toolchainv1alpha1.NSTemplateTier{}, builder.WithPredicates(predicate.GenerationChangedPredicate{})).
		Owns(&toolchainv1alpha1.TemplateUpdateRequest{}).
		Complete(r)
}

// Reconciler reconciles a NSTemplateTier object (only when this latter's specs were updated)
type Reconciler struct {
	Client client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=toolchain.dev.openshift.com,resources=nstemplatetiers,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=toolchain.dev.openshift.com,resources=spaces,verbs=get;list;watch
//+kubebuilder:rbac:groups=toolchain.dev.openshift.com,resources=nstemplatetiers/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=toolchain.dev.openshift.com,resources=nstemplatetiers/finalizers,verbs=update

// Reconcile takes care of:
// - inserting a new entry in the `status.updates`
func (r *Reconciler) Reconcile(ctx context.Context, request ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// fetch the NSTemplateTier tier
	tier := &toolchainv1alpha1.NSTemplateTier{}
	if err := r.Client.Get(context.TODO(), request.NamespacedName, tier); err != nil {
		if errors.IsNotFound(err) {
			logger.Info("NSTemplateTier not found")
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		logger.Error(err, "unable to get the current NSTemplateTier")
		return reconcile.Result{}, errs.Wrap(err, "unable to get the current NSTemplateTier")
	}

	// create a new entry in the `status.history` if needed
	if added, err := r.ensureStatusUpdateRecord(logger, tier); err != nil {
		logger.Error(err, "unable to insert a new entry in status.updates after NSTemplateTier changed")
		return reconcile.Result{}, errs.Wrap(err, "unable to insert a new entry in status.updates after NSTemplateTier changed")
	} else if added {
		logger.Info("Requeing after adding a new entry in tier.status.updates")
		return reconcile.Result{Requeue: true}, nil
	}
	return reconcile.Result{}, nil
}

// ensureStatusUpdateRecord adds a new entry in the `status.updates` with the current date/time
// if needed and the hash of the NSTemplateTier
// returns `true` if an entry was added, `err` if something wrong happened
func (r *Reconciler) ensureStatusUpdateRecord(logger logr.Logger, tier *toolchainv1alpha1.NSTemplateTier) (bool, error) {
	hash, err := tierutil.ComputeHashForNSTemplateTier(tier)
	if err != nil {
		return false, errs.Wrapf(err, "unable to append an entry in the `status.updates` for NSTemplateTier '%s'", tier.Name)
	}
	// if there was no previous status:
	if len(tier.Status.Updates) == 0 {
		tier.Status.Updates = append(tier.Status.Updates, toolchainv1alpha1.NSTemplateTierHistory{
			UpdateTime: metav1.Now(),
			Hash:       hash,
		})
		return true, r.Client.Status().Update(context.TODO(), tier)
	}
	// check whether the entry was already added
	if tier.Status.Updates[len(tier.Status.Updates)-1].Hash == hash {
		logger.Info("current tier template already exists in Updates")
		return false, nil
	}
	logger.Info("Adding a new entry in tier.status.updates")
	tier.Status.Updates = append(tier.Status.Updates, toolchainv1alpha1.NSTemplateTierHistory{
		StartTime: metav1.Now(),
		Hash:      hash,
	})
	return true, r.Client.Status().Update(context.TODO(), tier)
}
